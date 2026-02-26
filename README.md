# sqlc-gen-go

> [!IMPORTANT]  
> This repository is read-only. It contains a working Go codegen plugin extracted from https://github.com/sqlc-dev/sqlc which you can fork and modify to meet your needs.

See [Building from source](#building-from-source) and [Migrating from sqlc's built-in Go codegen](#migrating-from-sqlcs-built-in-go-codegen) if you want to use a modified fork in your project.

## Usage

```yaml
version: '2'
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v1.6.0/sqlc-gen-go.wasm
    sha256: 3d401e627cc0a59ca38de09b287dd6e74b073a961e40891ebfb7047f3b987881
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

Assuming you have the Go toolchain set up, from the project root you can simply `make all`.

```sh
make all
```

This will produce a standalone binary and a WASM blob in the `bin` directory.
They don't depend on each other, they're just two different plugin styles. You can
use either with sqlc, but we recommend WASM and all of the configuration examples
here assume you're using a WASM plugin.

To use a local WASM build with sqlc, just update your configuration with a `file://`
URL pointing at the WASM blob in your `bin` directory:

```yaml
plugins:
- name: golang
  wasm:
    url: file:///path/to/bin/sqlc-gen-go.wasm
    sha256: ""
```

As-of sqlc v1.24.0 the `sha256` is optional, but without it sqlc won't cache your
module internally which will impact performance.

## Migrating from sqlc's built-in Go codegen

We’ve worked hard to make switching to sqlc-gen-go as seamless as possible. Let’s say you’re generating Go code today using a sqlc.yaml configuration that looks something like this:

```yaml
version: 2
sql:
- schema: "query.sql"
  queries: "query.sql"
  engine: "postgresql"
  gen:
    go:
      package: "db"
      out: "db"
      emit_json_tags: true
      emit_pointers_for_null_types: true
      query_parameter_limit: 5
      overrides:
      - column: "authors.id"
        go_type: "your/package.SomeType"
      rename:
        foo: "bar"
```

To use the sqlc-gen-go WASM plugin for Go codegen, your config will instead look something like this:

```yaml
version: 2
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v1.6.0/sqlc-gen-go.wasm
    sha256: 3d401e627cc0a59ca38de09b287dd6e74b073a961e40891ebfb7047f3b987881
sql:
- schema: "query.sql"
  queries: "query.sql"
  engine: "postgresql"
  codegen:
  - plugin: golang
    out: "db"
    options:
      package: "db"
      emit_json_tags: true
      emit_pointers_for_null_types: true
      query_parameter_limit: 5
      overrides:
      - column: "authors.id"
        go_type: "your/package.SomeType"
      rename:
        foo: "bar"
```

The differences are:
* An additional top-level `plugins` list with an entry for the Go codegen WASM plugin. If you’ve built the plugin from source you’ll want to use a `file://` URL. The `sha256` field is required, but will be optional in the upcoming sqlc v1.24.0 release.
* Within the `sql` block, rather than `gen` with `go` nested beneath you’ll have a `codegen` list with an entry referencing the plugin name from the top-level `plugins` list. All options from the current `go` configuration block move as-is into the `options` block within `codegen`. The only special case is `out`, which moves up a level into the `codegen` configuration itself.

## Options

### `emit_per_file_queries`

When set to `true`, each SQL source file gets its own named struct and interface derived from the file name, instead of sharing a single `Queries` struct and `Querier` interface across all files.

| SQL file | Struct | Interface |
|---|---|---|
| `users.sql` | `UsersQueries` | `UsersQuerier` |
| `user_orders.sql` | `UserOrdersQueries` | `UserOrdersQuerier` |

**Example configuration:**

```yaml
options:
  emit_interface: true
  emit_per_file_queries: true
```

**Effect on generated files:**

- `users.sql.go` — contains `UsersQueries` struct, `NewUsersQueries()` constructor, query methods, and (if `emit_interface: true`) the `UsersQuerier` interface
- `orders.sql.go` — contains `OrdersQueries`, `NewOrdersQueries()`, and `OrdersQuerier`
- `db.go` — only the shared `DBTX` interface; the `Queries` struct and `New()` are **not** generated
- `querier.go` — **not** generated (interfaces are embedded in each SQL file instead)

**Constraints:** `emit_per_file_queries` and `emit_prepared_queries` are mutually exclusive.

---

### `emit_err_nil_if_no_rows`

When set to `true`, `:one` SELECT queries return `nil, nil` instead of `nil, pgx.ErrNoRows` (or `sql.ErrNoRows`) when no row is found.

**Example configuration:**

```yaml
options:
  emit_err_nil_if_no_rows: true
```

**Effect on generated code:**

```go
func (q *Queries) GetUser(ctx context.Context, arg GetUserParams) (*User, error) {
    row := q.db.QueryRowContext(ctx, getUser, arg.ID)
    var i User
    err := row.Scan(&i.ID, &i.Name)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    return &i, err
}
```

---

### `emit_tracing`

Injects custom tracing code at the start of every generated query method. Supports `{{.MethodName}}` and `{{.StructName}}` template variables.

| Field | Description |
|---|---|
| `import` | Import path of the tracing package |
| `package` | Package alias (if different from the last segment of `import`) |
| `code` | List of lines to inject; each line is a Go template |

**Example configuration:**

```yaml
options:
  emit_tracing:
    import: "go.opentelemetry.io/otel"
    package: "otel"
    code:
      - "ctx, span := otel.Tracer(\"{{.StructName}}\").Start(ctx, \"{{.MethodName}}\")"
      - "defer span.End()"
```

**Effect on generated code:**

```go
func (q *UsersQueries) GetUser(ctx context.Context, db DBTX, arg GetUserParams) (*User, error) {
    ctx, span := otel.Tracer("UsersQueries").Start(ctx, "GetUser")
    defer span.End()
    row := db.QueryRow(ctx, getUser, arg.ID)
    ...
}
```

---

### Global overrides and renames

If you have global overrides or renames configured, you’ll need to move those to the new top-level `options` field. Replace the existing `go` field name with the name you gave your plugin in the `plugins` list. We’ve used `"golang"` in this example.

If your existing configuration looks like this:

```yaml
version: "2"
overrides:
  go:
    rename:
      id: "Identifier"
    overrides:
    - db_type: "timestamptz"
      nullable: true
      engine: "postgresql"
      go_type:
        import: "gopkg.in/guregu/null.v4"
        package: "null"
        type: "Time"
...
```

Then your updated configuration would look something like this:

```yaml
version: "2"
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v1.6.0/sqlc-gen-go.wasm
    sha256: 3d401e627cc0a59ca38de09b287dd6e74b073a961e40891ebfb7047f3b987881
options:
  golang:
    rename:
      id: "Identifier"
    overrides:
    - db_type: "timestamptz"
      nullable: true
      engine: "postgresql"
      go_type:
        import: "gopkg.in/guregu/null.v4"
        package: "null"
        type: "Time"
...
```
