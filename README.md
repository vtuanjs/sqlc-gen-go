# sqlc-gen-go

A sqlc plugin that generates type-safe Go database access code from SQL. Runs as a WASM plugin (recommended) or standalone binary.

## Usage

```yaml
version: '2'
plugins:
- name: golang
  wasm:
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v1.7.0/sqlc-gen-go.wasm
    sha256: 996233111f91edd925266b9c52645b581fe8d16e751c412d9a59179b70f3cb90
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
    url: https://github.com/vtuanjs/sqlc-gen-go/releases/download/v1.7.0/sqlc-gen-go.wasm
    sha256: 996233111f91edd925266b9c52645b581fe8d16e751c412d9a59179b70f3cb90
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

### `go_generate`

Adds a `//go:generate` directive to each `*.sql.go` file. `$GOFILE` expands to the current filename at generate time.

```yaml
options:
  go_generate: "mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock"
```

Running `go generate ./...` produces a mock per SQL file:

| Source | Mock |
|---|---|
| `users.sql.go` | `mock/users.sql.go` |
| `orders.sql.go` | `mock/orders.sql.go` |
