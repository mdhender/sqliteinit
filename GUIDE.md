# Getting Started with sqliteinit

A walkthrough for integrating sqliteinit into your application.

## Project Setup

Create a migrations directory in your project:

```
myapp/
├── main.go
├── migrations/
│   └── (your .sql files go here)
└── go.mod
```

Import the package and a SQLite driver:

```go
import (
    "github.com/mdhender/sqliteinit"
    _ "modernc.org/sqlite"
)
```

## Writing Migrations

Create SQL files with timestamp prefixes. The timestamp becomes the schema version.

```
migrations/
├── 20260115100000_create_users.sql
└── 20260115100001_create_posts.sql
```

Each file contains plain SQL:

```sql
-- 20260115100000_create_users.sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);
```

```sql
-- 20260115100001_create_posts.sql
CREATE TABLE posts (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
```

## Embedding Migrations

Use Go's embed directive to compile migrations into your binary:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS
```

Create a sub-filesystem rooted at the migrations directory:

```go
migrations, err := fs.Sub(migrationsFS, "migrations")
if err != nil {
    log.Fatal(err)
}
```

## Happy Path: Development

For local development and testing, use in-memory databases:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:       ":memory:",
    Migrations: migrations,
})
```

The database initializes, migrations run, and you have a fresh schema every time. No cleanup needed.

## Happy Path: Production

For production, create a persistent database once during deployment or first run. Persistent paths must be absolute and use the `.db` extension:

```go
err := sqliteinit.Create(ctx, sqliteinit.Config{
    Path:       "/var/lib/myapp/data.db",
    Migrations: migrations,
    AppVersion: version,
})
```

On subsequent runs, open the existing database:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:                  "/var/lib/myapp/data.db",
    Migrations:            migrations,
    RequiredSchemaVersion: 20260115100001,
})
```

New migrations are applied automatically. `RequiredSchemaVersion` catches deployment mistakes where the code expects a schema version that doesn't match.

## Happy Path: Checking Before Applying

Before deploying, check what migrations would run:

```go
status, err := sqliteinit.Status(ctx, sqliteinit.Config{
    Path:       "/var/lib/myapp/data.db",
    Migrations: migrations,
})

fmt.Printf("Current version: %d\n", status.SchemaVersion)
fmt.Printf("Pending migrations: %v\n", status.Pending)
```

## Sad Path: Invalid Path

Persistent paths must be absolute with a `.db` extension:

```go
// Relative path
err := sqliteinit.Create(ctx, sqliteinit.Config{
    Path: "data/app.db",
})
// err: "data/app.db: persistent database path must be absolute"

// Wrong extension
err := sqliteinit.Create(ctx, sqliteinit.Config{
    Path: "/var/lib/myapp/data.sqlite",
})
// err: "/var/lib/myapp/data.sqlite: expected .db extension"
```

## Sad Path: Database Doesn't Exist

If you call `Open` on a persistent path that doesn't exist:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path: "/var/lib/myapp/data.db",
})
// err: "/var/lib/myapp/data.db: database file not found (use Create to make a new database)"
```

Fix: Use `Create` first, or check with `Status` (which returns `IsInitialized: false` for missing files).

## Sad Path: Database Already Exists

If you call `Create` when the file already exists:

```go
err := sqliteinit.Create(ctx, sqliteinit.Config{
    Path: "/var/lib/myapp/data.db",
})
// err: "/var/lib/myapp/data.db: file already exists"
```

Fix: Use `Open` for existing databases.

## Sad Path: Schema Version Mismatch

If the database schema doesn't match what your code expects:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:                  "/var/lib/myapp/data.db",
    RequiredSchemaVersion: 20260115100001,
})
// err: "schema version mismatch: required 20260115100001, found 20260115100000"
```

This usually means:
- You deployed code without running migrations
- You're running old code against a newer database
- The migrations filesystem is missing files

Fix: Verify migrations are embedded correctly and the database has been migrated.

## Sad Path: Migration Fails

If a migration contains invalid SQL:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path:       ":memory:",
    Migrations: migrations,
})
// err: "migrate: apply 20260115100001_create_posts.sql: exec: ..."
```

Migrations run in transactions. A failed migration rolls back and leaves the database at the previous version. Fix the SQL and retry.

## Sad Path: In-Memory in Production

If `$ENV=production` and you try to use an in-memory database:

```go
db, err := sqliteinit.Open(ctx, sqliteinit.Config{
    Path: ":memory:",
})
// err: "in-memory database not allowed in production (ENV=production)"
```

This is intentional. In-memory databases lose data on restart. Either use a persistent path or set `AllowMemoryInProduction: true` if you really mean it.

## Typical Application Pattern

```go
func openDatabase(ctx context.Context, cfg AppConfig) (*sql.DB, error) {
    migrations, err := fs.Sub(migrationsFS, "migrations")
    if err != nil {
        return nil, err
    }

    dbCfg := sqliteinit.Config{
        Path:                  cfg.DatabasePath,
        Migrations:            migrations,
        RequiredSchemaVersion: SchemaVersion,
        AppVersion:            Version,
    }

    if cfg.DatabasePath == ":memory:" {
        return sqliteinit.Open(ctx, dbCfg)
    }

    // For persistent databases, create if missing
    status, err := sqliteinit.Status(ctx, dbCfg)
    if err != nil {
        return nil, err
    }

    if !status.IsInitialized {
        if err := sqliteinit.Create(ctx, dbCfg); err != nil {
            return nil, err
        }
    }

    return sqliteinit.Open(ctx, dbCfg)
}
```
