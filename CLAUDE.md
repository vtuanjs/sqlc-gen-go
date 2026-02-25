# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
make all          # Build both binary and WASM plugin (bin/sqlc-gen-go + bin/sqlc-gen-go.wasm)
make build        # go build ./...
make test         # Build WASM then run go test ./...

# Run a single test
go test ./internal/... -run TestName

# Run fuzz tests
go test ./internal/opts/... -fuzz FuzzOverride
```

The WASM build requires `GOOS=wasip1 GOARCH=wasm`. The `make test` target builds the WASM first because end-to-end tests depend on it.

## Architecture

This is a **sqlc plugin** that generates type-safe Go database access code from SQL queries and schemas. It can run as a standalone binary or as a WASM plugin (recommended) loaded by sqlc.

### Entry point

`plugin/main.go` → calls `codegen.Run(golang.Generate)` from the plugin SDK. The SDK handles protobuf I/O; `Generate()` in `internal/gen.go` is where all logic lives.

### Code generation pipeline (`internal/gen.go`)

```
Generate(req) → parse options → buildEnums() → buildStructs() → buildQueries() → validate() → render templates
```

Output files generated per SQL file:
- `models.go` — table structs and enums
- `*.sql.go` — one file per SQL source file with query methods
- `db.go` — DB wrapper (DBTX interface)
- `querier.go` — interface (if `emit_interface: true`)
- `copyfrom.go` — bulk copy support (pgx/MySQL)
- `batch.go` — batch operations (pgx only)

### Key internal packages

| Package | Purpose |
|---|---|
| `internal/` | Core generation: `gen.go`, `query.go`, `result.go`, `struct.go`, `enum.go`, `field.go` |
| `internal/opts/` | Config parsing (`Options` struct), type override resolution |
| `internal/templates/` | Go text/templates for each driver (`pgx/`, `stdlib/`, `go-sql-driver-mysql/`) |
| `internal/inflection/` | Table name singularization for struct names |
| `internal/endtoend/` | E2E test data and runner |

### Type system

- `internal/postgresql_type.go`, `mysql_type.go`, `sqlite_type.go` map DB types → Go types
- `internal/opts/override.go` handles user-configured type overrides (`go_type` in sqlc.yaml)
- `internal/go_type.go` resolves final Go type strings including imports

### SQL command support

`:one`, `:many`, `:exec`, `:execrows`, `:execlastid`, `:copyfrom`, `:batchexec`, `:batchmany`, `:batchone`

### Module name

`github.com/vtuanjs/sqlc-gen-go` (forked from `github.com/sqlc-dev/sqlc-gen-go`)