# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
# Run all tests (default: modernc.org/sqlite driver)
go test ./...

# Run tests with mattn/go-sqlite3 driver (requires CGO)
go test -tags mattn ./...

# Run a single test
go test -run TestOpen_Memory ./...

# Build with mattn driver
go build -tags mattn ./...
```

## Architecture

This is a Go library for SQLite database initialization and migration management. Key design principles:

- **In-memory default**: Databases are in-memory by default (`:memory:`), safe for agents/tests/CI
- **Explicit persistence**: Persistent databases require absolute paths with `.db` extension
- **Package-owned infrastructure**: The package owns `schema.sql` which creates `schema_migrations` and `config` tables
- **User-provided migrations**: Application migrations go in an embedded `fs.FS`, named `YYYYMMDDHHMMSS_description.sql`

### Driver Support via Build Tags

Two SQLite drivers are supported through build tags and separate pragma files:
- `pragma_modernc.go` (default): Pure Go driver `modernc.org/sqlite`
- `pragma_mattn.go` (build tag `mattn`): CGO driver `github.com/mattn/go-sqlite3`

Each file defines driver-specific DSN construction and pragma syntax.

### Core Files

- `sqliteinit.go`: Main API (`Open`, `Create`, `Delete`, `Status`), `Config` struct, path validation
- `migrate.go`: Migration execution logic, schema initialization, migration file parsing
- `schema.sql`: Embedded infrastructure schema (owned by package, not user-modifiable)

### Migration Flow

1. `Open`/`Create` calls `openAndMigrate`
2. If uninitialized, `applySchemaInit` runs `schema.sql` and records it as migration ID 0
3. User migrations from `Config.Migrations` are applied in lexicographic order
4. Each migration runs in a transaction, updates `schema_migrations` and `schema.version` in `config`
