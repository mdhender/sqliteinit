// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package sqliteinit provides SQLite database initialization and migration
// management with intentional persistence semantics.
//
// The package implements a database lifecycle model where:
//   - Databases default to in-memory (safe for agents, tests, CI)
//   - Persistent databases require explicit opt-in with absolute paths
//   - Migrations are embedded and applied automatically
//   - Schema tracking uses config and schema_migrations tables
//
// # Basic Usage
//
//	//go:embed migrations/*.sql
//	var migrationsFS embed.FS
//
//	func main() {
//	    migrations, _ := fs.Sub(migrationsFS, "migrations")
//	    db, err := sqliteinit.Open(ctx, sqliteinit.Config{
//	        Path:       ":memory:",
//	        Migrations: migrations,
//	    })
//	}
//
// # Driver Support
//
// This package supports two SQLite drivers via build tags:
//   - modernc.org/sqlite (default, pure Go, no CGO)
//   - github.com/mattn/go-sqlite3 (CGO, use -tags mattn)
//
// You must import the appropriate driver in your application:
//
//	import _ "modernc.org/sqlite"           // default
//	import _ "github.com/mattn/go-sqlite3"  // with -tags mattn
//
// # Migration Files
//
// Migration files must be named YYYYMMDDHHMMSS_comment.sql and are applied
// in lexicographic order. The package owns the init script that creates
// infrastructure tables (schema_migrations, config); users provide only
// their application-specific migrations.
//
// # Configuration
//
// Key Config fields:
//   - Path: ":memory:" for in-memory, or absolute path with .db extension
//   - Migrations: fs.FS containing your application's SQL migrations
//   - SkipMigrations: set to true to open without running migrations
//   - AppVersion: optional version string written to config table
//   - ProductionEnvVar: env var to check for production mode (default: "ENV")
package sqliteinit
