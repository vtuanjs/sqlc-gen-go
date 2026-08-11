# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make all          # Build both binary and WASM plugin (bin/sqlc-gen-go + bin/sqlc-gen-go.wasm)
make build        # go build ./...
make test         # Build WASM, then run go test ./internal/... and example/test/...

# Run a single test
go test ./internal/... -run TestName

# Run fuzz tests
go test ./internal/opts/... -fuzz FuzzOverride
```

The WASM build requires `GOOS=wasip1 GOARCH=wasm`. The `make test` target builds the WASM first because integration tests depend on it. E2E tests against real databases live in `example/e2e-postgres/`, `example/e2e-mysql/`, and `example/e2e-sqlite/` and are run separately (`make example-e2e`, or per-engine `example-e2e-{postgres,mysql,sqlite}` targets).

## Architecture

This is a **sqlc plugin** that generates type-safe Go database access code from SQL queries and schemas. It can run as a standalone binary or as a WASM plugin (recommended) loaded by sqlc.

### Entry point

`plugin/main.go` → calls `codegen.Run(golang.Generate)` from the plugin SDK. The SDK handles protobuf I/O; `Generate()` in `internal/gen.go` is where all logic lives.

### Code generation pipeline (`internal/gen.go`)

```
Generate(req) → parse options → buildEnums() → buildStructs() → buildQueries() → validate() → render templates
```

Output files generated:
- `db.go` — DBTX interface and Queries struct
- `models.go` — table structs and enums
- `*.sql.go` — one file per SQL source file with query methods
- `querier.go` — interface (if `emit_interface: true`; skipped when `emit_per_file_queries: true`)
- `copyfrom.go` — bulk copy support (pgx/MySQL)
- `batch.go` — batch operations (pgx only)
- `dynfilter.go` — dynamic filter helpers (if `emit_dynamic_filter: true`)

### Key internal packages

| Package | Purpose |
|---|---|
| `internal/` | Core generation: `gen.go`, `query.go`, `result.go`, `struct.go`, `enum.go`, `field.go` |
| `internal/dynfilter.go` | Parses `-- :if @param` annotations for dynamic WHERE/ORDER BY |
| `internal/opts/` | Config parsing (`Options` struct), type override resolution |
| `internal/templates/` | Go text/templates for each driver (`pgx/`, `stdlib/`, `go-sql-driver-mysql/`) |
| `internal/inflection/` | Table name singularization for struct names |

### Type system

- `internal/postgresql_type.go`, `mysql_type.go`, `sqlite_type.go` map DB types → Go types
- `internal/opts/override.go` handles user-configured type overrides (`go_type` in sqlc.yaml)
- `internal/go_type.go` resolves final Go type strings including imports

### SQL command support

`:one`, `:many`, `:exec`, `:execrows`, `:execlastid`, `:execresult`, `:copyfrom`, `:batchexec`, `:batchmany`, `:batchone`

### Notable features

**`emit_per_file_queries`** — each SQL source file gets its own named struct/interface:
- `users.sql` → `UsersQueries` struct + `UsersQuerier` interface
- `sourceNameToPrefix()` in `gen.go` converts filenames to Go identifiers
- Incompatible with `emit_prepared_queries`
- No `querier.go` generated; interface embedded in each `*.sql.go`

**`emit_dynamic_filter`** — conditional WHERE/ORDER BY via `-- :if @param` annotations:
- Params annotated with `:if` become pointer types (`*T`); `nil` skips the condition
- Flag-only params (not SQL params) are added as `bool` fields
- `dynCompile()` and `DynamicSQL()` helpers emitted into `dynfilter.go`; generated queries use `dynCompile` (pre-compiled) by default

**`emit_tracing`** — injects custom tracing code into every query method via a Go template:
```yaml
emit_tracing:
  import: "go.opentelemetry.io/otel"
  package: "otel"
  code:
    - 'ctx, span := {{.Package}}.Tracer("").Start(ctx, "{{.MethodName}}")'
    - "defer span.End()"
```

### Template notes

- Inside `{{range .GoQueries}}`, use `{{$.FieldName}}` (not `{{.FieldName}}`) to access `tmplCtx` fields
- `OutputInterfaceMethod .SourceName` filters interface methods by source file in per-file mode

### Module name

`github.com/vtuanjs/sqlc-gen-go` (forked from `github.com/sqlc-dev/sqlc-gen-go`)
