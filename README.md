# sqliteinit

SQLite database initialization and migration management with intentional persistence semantics.

## Features

- **In-memory default**: Safe for agents, tests, and CI pipelines
- **Explicit persistence**: Requires absolute paths and opt-in
- **Embedded migrations**: SQL scripts compiled into your binary
- **Package-owned schema**: Infrastructure tables managed automatically
- **Fail-fast execution**: Migrations run in transactions, abort on error
- **Driver flexibility**: Supports modernc/sqlite (pure Go) and mattn/go-sqlite3 (CGO)

## Installation

```bash
go get github.com/mdhender/sqliteinit
```

Import a SQLite driver in your application:

```go
import _ "modernc.org/sqlite"           // default (pure Go, no CGO)
// OR
import _ "github.com/mattn/go-sqlite3"  // requires CGO, use -tags mattn
```

## Quick Start

```go
package main

import (
    "context"
    "embed"
    "io/fs"
    "log"

    "github.com/mdhender/sqliteinit"
    _ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
    ctx := context.Background()

    // Get sub-filesystem rooted at migrations/
    migrations, _ := fs.Sub(migrationsFS, "migrations")

    // Open in-memory database with migrations
    db, err := sqliteinit.Open(ctx, sqliteinit.Config{
        Path:       ":memory:",
        Migrations: migrations,
        AppVersion: "1.0.0",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Use db...
}
```

## Migration Files

Name your migration files with the pattern `YYYYMMDDHHMMSS_description.sql`:

```
migrations/
├── 20260101000001_create_users.sql
├── 20260101000002_create_posts.sql
└── 20260102000001_add_user_roles.sql
```

Migrations are applied in lexicographic order by filename.

## Persistent Databases

```go
// Create a new database
err := sqliteinit.Create(ctx, sqliteinit.Config{
    Path:       "/data/myapp/app.db",  // must be absolute with .db extension
    Migrations: migrations,
})

// Open an existing database
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:       "/data/myapp/app.db",
    Migrations: migrations,
})

// Delete a database (including WAL files)
err := sqliteinit.Delete(ctx, "/data/myapp/app.db")
```

## Configuration

| Field | Default | Description |
|-------|---------|-------------|
| `Path` | required | `:memory:` or absolute path with `.db` extension |
| `Migrations` | nil | `fs.FS` containing your SQL migration files |
| `SkipMigrations` | false | Set to true to open without running migrations |
| `AppVersion` | "" | Written to config table after initialization |
| `RequiredSchemaVersion` | 0 | If non-zero, verify schema version matches exactly |
| `ProductionEnvVar` | "ENV" | Env var checked for production mode |
| `AllowMemoryInProduction` | false | Allow `:memory:` when env var is "production" |
| `MigrationTimeout` | 90s | Maximum time for migration execution |
| `Logger` | slog.Default() | Logger for operational messages |

## Production Safety

By default, in-memory databases are rejected when `$ENV=production`:

```go
// This will fail if ENV=production
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path: ":memory:",
})

// Override the env var name
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:             ":memory:",
    ProductionEnvVar: "MYAPP_ENV",
})

// Allow memory in production (for testing)
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:                    ":memory:",
    AllowMemoryInProduction: true,
})
```

## Schema Tracking

The package automatically creates and manages:

- `schema_migrations` - Records all applied migrations
- `config` - Key-value store with `schema.version`, `app.version`, `db.created_at`

## Build Tags

For mattn/go-sqlite3 driver:

```bash
go build -tags mattn ./...
go test -tags mattn ./...
```

## Authors

- Michael D Henderson ([@mdhender](https://github.com/mdhender))
- Claude (AI assistant by [Anthropic](https://anthropic.com))

## License

MIT - See [LICENSE](LICENSE)
