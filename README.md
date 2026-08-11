# sqlc-gen-go

A sqlc plugin that generates type-safe Go database access code from SQL. Runs as a WASM plugin (recommended) or standalone binary.

## Usage

```yaml
version: '2'
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v3.0.1/sqlc-gen-go.wasm
    sha256: f6a02c8cb363c56bb7a7de1d1012a65e800897696de9d28b938563af02e3ce81
sql:
- schema: schema.sql
  queries: query.sql
  engine: postgresql
  codegen:
  - plugin: golang
    out: db
    options:
      package: db
      sql_package: pgx/v5
```

## Building from source

```sh
make all  # produces bin/sqlc-gen-go and bin/sqlc-gen-go.wasm
```

To use a local build:

```yaml
plugins:
- name: golang
  wasm:
    url: file:///path/to/bin/sqlc-gen-go.wasm
    sha256: ""  # optional since sqlc v1.24.0
```

## Migrating from sqlc's built-in Go codegen

Two changes are required:

1. Add a top-level `plugins` entry pointing to the WASM plugin.
2. Replace `gen.go` with `codegen`, referencing the plugin by name. Move all options into the `options` block; `out` moves up one level.

**Before:**
```yaml
sql:
- engine: postgresql
  gen:
    go:
      package: db
      out: db
      emit_json_tags: true
```

**After:**
```yaml
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v3.0.1/sqlc-gen-go.wasm
    sha256: f6a02c8cb363c56bb7a7de1d1012a65e800897696de9d28b938563af02e3ce81
sql:
- engine: postgresql
  codegen:
  - plugin: golang
    out: db
    options:
      package: db
      emit_json_tags: true
```

Global `overrides`/`go` move to `options`/`<plugin-name>`:

```yaml
options:
  golang:
    rename:
      id: "Identifier"
    overrides:
    - db_type: "timestamptz"
      nullable: true
      engine: postgresql
      go_type:
        import: "gopkg.in/guregu/null.v4"
        package: "null"
        type: "Time"
```

## Advanced Options

### `emit_per_file_queries`

Each SQL source file gets its own struct and interface instead of a shared `Queries`/`Querier`.

| SQL file | Struct | Interface |
|---|---|---|
| `users.sql` | `UsersQueries` | `UsersQuerier` |
| `user_orders.sql` | `UserOrdersQueries` | `UserOrdersQuerier` |

```yaml
options:
  emit_interface: true
  emit_per_file_queries: true
```

- Each `*.sql.go` contains its own struct, constructor, methods, and interface.
- `db.go` only keeps `DBTX`; `querier.go` is not generated.
- Incompatible with `emit_prepared_queries`.

---

### `emit_err_nil_if_no_rows`

`:one` queries return `nil, nil` instead of `nil, sql.ErrNoRows` when no row is found.

```yaml
options:
  emit_err_nil_if_no_rows: true
```

---

### `emit_tracing`

Injects custom code at the start of every query method. Supports `{{.MethodName}}` and `{{.StructName}}` template variables.

```yaml
options:
  emit_tracing:
    import: "go.opentelemetry.io/otel"
    package: "otel"
    code:
      - "ctx, span := otel.Tracer(\"{{.StructName}}\").Start(ctx, \"{{.MethodName}}\")"
      - "defer span.End()"
```

| Field | Description |
|---|---|
| `import` | Import path of the tracing package |
| `package` | Package alias (if different from the last path segment) |
| `code` | Lines to inject; each is a Go template |

---

### `emit_dynamic_filter`

Enables optional WHERE/ORDER BY clauses controlled at runtime via `-- :if @param` annotations in SQL.

When a parameter is marked with `:if`, the generated code:
- Makes the parameter a pointer (`*T`) in the params struct — `nil` means "skip this clause"
- Adds a `bool` field for flag-only parameters (e.g. ORDER BY toggles that appear only in `:if` annotations, not as `col = $N` predicate values)
- Calls the generated `DynamicSQL()` helper at runtime to strip inactive lines and renumber placeholders

A `:if`-gated parameter is *inactive* (clause skipped) when it is a nil pointer, a `false` bool, or a **nil** slice. An empty non-nil slice keeps the clause and renders `NULL` (matching zero rows): nil means "no filter requested", empty means "filter by the empty set" — the fail-closed default for computed lists such as permission scopes. When empty should mean "don't filter", wrap the argument in the generated `NilableSlice` helper, which converts empty to nil:

```go
users, err := q.SearchUsersByIDs(ctx, db, db.SearchUsersByIDsParams{
    Ids: db.NilableSlice(ids), // empty → nil → clause skipped
})
```

```yaml
options:
  emit_dynamic_filter: true
```

**Engine support** — PostgreSQL, SQLite, and MySQL. The emitted `dynfilter.go` runtime is keyed to the configured `engine`:

| Engine | Input placeholders | Output placeholders |
|---|---|---|
| `postgresql` | `$N` (`?` is always operator text, e.g. jsonb `?`) | `$N` |
| `sqlite` | numbered `?N` or `$N` | `$N` (bound positionally by SQLite) |
| `mysql` | bare `?`, numbered by appearance | `?` (selected by the engine alone; `sql_driver` is not required) |

**`sqlc.slice()` in dynamic queries** — `/*SLICE:name*/` markers are numbered at generation time and expanded at `Build` time into one placeholder per element (the expansion is reused when the same slice parameter appears more than once on numbered-placeholder engines). A nil or empty slice renders `NULL`, matching sqlc's non-dynamic expansion — and when the slice is itself the `:if` condition, nil skips the clause entirely while empty keeps it (see above).

**SQL annotations**

```sql
-- name: SearchUsers :many
SELECT * FROM users
WHERE
  1 = 1
  AND email = @email -- :if @email
  -- :if @phone
  AND phone = @phone
  AND EXISTS ( -- :if @has_orders
    SELECT 1 FROM orders
    WHERE orders.user_id = users.id
      AND orders.created_at >= @orders_since -- :if @orders_since
  )
ORDER BY id ASC;
```

The first `:if` is the inline style (omit this line when `email` is nil); the standalone `-- :if @phone` is the top-level style (omit the next line); `@has_orders` is a flag-only boolean that omits the whole `EXISTS (…)` block when false. The `:if` annotation must be the last thing on its line — text after it is not parsed.

```sql

-- name: SearchUsersOrdered :many
SELECT * FROM users
WHERE
  TRUE
  AND email = @email -- :if @email
ORDER BY
  id ASC,  -- :if @id_asc
  id DESC,  -- :if @id_desc
  TRUE
```

Note: We use TRUE to prevent SQL errors when a line is omitted.

**Generated Go**

For SearchUsers
```go
var _searchUsersDynQ = dynCompile(SearchUsers)

type SearchUsersParams struct {
    Email       *string    // nil → clause skipped
    Phone       *string    // nil → clause skipped
    OrdersSince *time.Time // nil → clause skipped
    HasOrders   bool       // false → EXISTS block skipped
}

func (q *SearchQueries) SearchUsers(ctx context.Context, db DBTX, arg SearchUsersParams) ([]*User, error) {
...
  dynQuery, dynArgs := _searchUsersDynQ.Build([]any{arg.Email, arg.Phone, arg.OrdersSince, arg.HasOrders})
	rows, err := db.Query(ctx, dynQuery, dynArgs...)
...
}
```

**Annotation rules**

| Style | Syntax | Behaviour |
|---|---|---|
| Inline | `AND col = @param -- :if @param` | Skip this line if param is nil/false |
| Inline (multi-param) | `AND col = @param -- :if @a @b` | Skip this line if **any** listed param is nil/false |
| Top-level | `-- :if @param` on its own line | Skip the **next** line if param is nil/false |
| Top-level Block `( )` | `-- :if @flag` then `AND EXISTS (` on the next line | Skip the **next** line if param is nil/false; if that line opens a paren block, skip the **entire block** (until matching `)`) |
| Inline Block `( )` | `AND EXISTS ( -- :if @flag` | Skip the entire parenthesized block (until matching `)`) if flag is false/nil |

Two helpers are emitted into `dynfilter.go` in the output package:

- **`dynCompile(query)`** — default behavior; pre-compiles the annotated SQL once at package init into a `dynCompiledQuery`. Each generated query uses this via a package-level `var _..DynQ = dynCompile(...)`, then calls `.Build(args)` per request with no per-call scanning.
- **`DynamicSQL(query, args)`** — one-shot helper; parses and filters on every call. Available for ad-hoc use.

After filtering, remaining placeholders are renumbered sequentially (`$N` output for PostgreSQL/SQLite, `?` for MySQL) and the args slice is trimmed to match, preventing "expected N arguments, got M" errors.

**Lexical context** — annotations and bind markers are only recognized in SQL code. String literals (including PostgreSQL dollar-quoted `$$…$$` / `$tag$…$tag$` and `E'…'` escape strings, and MySQL backslash-escaped quotes), quoted identifiers (`"…"`, MySQL backticks, SQLite `[…]`), and comments (with PostgreSQL comment nesting) are opaque: `-- :if` or `$N`/`?N` text inside them is never rewritten or counted. String literals may span lines; a line that *begins* inside an open string or comment is treated as continuation text and is never scanned for annotations, so a real `-- :if` annotation must start on a line that begins in SQL code.

---

### `disable_result_slice_pointers`

When `emit_result_struct_pointers: true` is set, `:many` queries return `[]*T` by default. Setting `disable_result_slice_pointers: true` keeps `:one` results as `*T` while changing `:many` results back to `[]T`.

Requires `emit_result_struct_pointers: true`.

```yaml
options:
  emit_result_struct_pointers: true
  disable_result_slice_pointers: true
```

| Query command | `emit_result_struct_pointers` only | + `disable_result_slice_pointers` |
|---|---|---|
| `:one` | `*MyRow` | `*MyRow` |
| `:many` | `[]*MyRow` | `[]MyRow` |

---

### `go_generate_mock`

Adds a `//go:generate` directive for mock generation. `$GOFILE` expands to the current filename at generate time.

- When `emit_per_file_queries` is enabled: the directive is added to each `*.sql.go` file.
- Otherwise: the directive is added only to the `querier.go` file.

```yaml
options:
  go_generate_mock: "mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock"
```

Running `go generate ./...` produces a mock per SQL file (with `emit_per_file_queries`):

| Source | Mock |
|---|---|
| `users.sql.go` | `mock/users.sql.go` |
| `orders.sql.go` | `mock/orders.sql.go` |

---

## Test Coverage

### Plugin internals (`internal/`)

Run with:

```sh
go test ./internal/... -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out
```

| Package | Coverage |
|---|---|
| `internal` | 88.0% |
| `internal/opts` | 89.4% |
| `internal/inflection` | 100.0% |
| **Total** | **88.2%** |

Key areas at 100%: `enum.go`, `field.go` (all case-style helpers), `inflection/singular.go`, `opts/enum.go` (driver/package validation), `opts/options.go ValidateOpts`, `reserved.go`, `struct.go`, `imports.go` (merge/sort/interface/copyfrom/batch).

### Generated code (`example/test/`)

Unit tests for the generated Go code — no database required. Covers `DynamicSQL` SQL-building logic, generated query SQL strings, and dynamic filter / ORDER BY combinations. Each generated package accepts its engine's input placeholders (PostgreSQL `$N`; SQLite numbered `?N`; MySQL bare `?`) and normalizes active ones to its engine's output placeholders.

```sh
cd example
go test ./test/... -v
```

**82 passing test cases** across:

| Test | Sub-tests | What is covered |
|---|---|---|
| `TestDynamicSQL` | 30 | Placeholder remapping, gap handling, ORDER BY clauses, orphaned WHERE/GROUP BY/HAVING cleanup, EXISTS blocks, empty-slice gating, `NilableSlice` |
| `TestDynamicSQL_LexicalContext` | 16 | Markers inside string literals (incl. dollar-quoted, `E'…'`, backslash-escaped, multi-line), quoted identifiers, nested comments |
| `TestDynamicSQLSlices` | 7 | `sqlc.slice()` expansion, repeated and empty slices, slice-marker edge cases across all three engines |
| `TestSearchUsers` | 9 | Optional email/phone/date filter combinations on generated search query |
| `TestSearchUsersOrdered` | 4 | ORDER BY flag combinations |
| `TestSearchUsersByContact` | 4 | Multi-param optional filter |
| `TestSearchUsersWithSameNameAndEmail` | 2 | Nil vs non-nil shared-column filter |
| `TestSearchUsersWithBlock` | 2 | EXISTS block conditional inclusion |
| `TestSearchUsersWithTopStyle` | 2 | Top-level `:if` annotation style |
| `TestSearchUsersOrderedByID` | 4 | ASC/DESC flag combinations with optional filters |
| `TestGetUserWithLock` | 2 | `FOR UPDATE` / `FOR SHARE` SQL generation |

### End-to-end

Each engine has its own e2e package (`example/e2e-postgres/`, `example/e2e-mysql/`, `example/e2e-sqlite/`) testing its generated package (`dbpostgres`, `dbmysql`, `dbsqlite`). `make example-e2e` runs all three; per-engine targets exist too:

```sh
make example-e2e            # all engines (postgres + mysql via docker compose)
make example-e2e-postgres
make example-e2e-mysql
make example-e2e-sqlite     # in-memory, no docker
```

### PostgreSQL end-to-end (`example/e2e-postgres/`)

Integration tests that run queries against a real PostgreSQL database (`postgres://postgres:postgres@localhost:6432/sqlc-test`). Covers the same scenarios as the unit tests but validates actual query execution and result mapping.

| Test | What is covered |
|---|---|
| `TestSearchUsers` | Optional email/phone/date filters against real rows |
| `TestSearchUsersOrdered` | ORDER BY flag combinations |
| `TestSearchUsersByContact` | Multi-param optional filter |
| `TestSearchUsersWithSameNameAndEmail` | Nil vs non-nil shared-column filter |
| `TestSearchUsersOrderedByID` | ASC/DESC flag combinations with optional filters |
| `TestGetUserWithLock` | `FOR UPDATE` / `FOR SHARE` locking |

### MySQL end-to-end (`example/e2e-mysql/`)

Runs the generated dynamic-filter query against a real MySQL 8 (`root:mysql@tcp(localhost:6603)/sqlc-test`), covering nil, populated, and empty `sqlc.slice()` values.

### SQLite end-to-end (`example/e2e-sqlite/`)

Runs SQLite dynamic filters through `database/sql` without Docker, including optional middle-argument removal and placeholder remapping.

---

## Benchmarks

Benchmarks compare three approaches for dynamic SQL construction (`emit_dynamic_filter`). Source: [`example/bench/`](example/bench/).

```sh
cd example
go test ./bench -bench=. -benchmem -count=3 -run='^$'
```

Three strategies under test:

| Strategy | Description |
|---|---|
| **DynamicSQL** | One-shot helper; parses the annotated SQL on every call |
| **PreCompiled** | SQL parsed once at package init into a `dynCompiledQuery`; `Build()` called per request — no per-call scanning |
| **Manual** | Hand-written `strings.Builder` with `fmt.Fprintf` per condition |

### Small query (5 params, 4 optional)

| Benchmark | ns/op | req/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| DynamicSQL — no optional | 2,285 | ~438 K | 3,688 | 46 |
| **PreCompiled — no optional** | **180** | **~5.56 M** | **304** | **3** |
| Manual — no optional | 138 | ~7.25 M | 368 | 5 |
| DynamicSQL — all optional | 2,478 | ~403 K | 4,168 | 49 |
| **PreCompiled — all optional** | **409** | **~2.44 M** | **784** | **6** |
| Manual — all optional | 442 | ~2.26 M | 688 | 6 |

### Large query (21 params, 20 optional)

| Benchmark | ns/op | req/s | B/op | allocs/op |
|---|---:|---:|---:|---:|
| DynamicSQL — no optional | 5,375 | ~186 K | 7,472 | 119 |
| **PreCompiled — no optional** | **226** | **~4.43 M** | **304** | **3** |
| Manual — no optional | 198 | ~5.05 M | 640 | 5 |
| DynamicSQL — all optional | 6,843 | ~146 K | 9,552 | 126 |
| **PreCompiled — all optional** | **944** | **~1.06 M** | **2,384** | **10** |
| Manual — all optional | 2,473 | ~404 K | 1,921 | 27 |

Results on Intel Core i7-11800H @ 2.30GHz.

### Takeaways

- **PreCompiled is ~14–25× faster than `DynamicSQL`** and matches manual for the no-optional case.
- **For the all-optional large query, PreCompiled is 2.3× faster than manual** — `Build` writes pre-split string literals directly vs `fmt.Fprintf` per condition.
- **Allocations drop from 46–126 down to 3–10**, matching or beating manual.
- A typical DB round-trip is ~1 ms; PreCompiled overhead is ~150–800 ns — effectively free in practice.
