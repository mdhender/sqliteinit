// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package sqliteinit

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed schema.sql
var schemaFS embed.FS

// Config holds database configuration options.
type Config struct {
	// Path to database file. Use ":memory:" for in-memory databases.
	// Persistent paths must be absolute and have a .db extension.
	Path string

	// Migrations is an embedded filesystem containing application migration
	// scripts. Optional - if nil, only infrastructure tables are created.
	// Scripts must be named YYYYMMDDHHMMSS_comment.sql.
	Migrations fs.FS

	// Logger for operational logging. Uses slog.Default() if nil.
	Logger *slog.Logger

	// ProductionEnvVar is the environment variable checked to determine
	// production mode. If the variable equals "production" (case-insensitive),
	// in-memory databases are rejected unless AllowMemoryInProduction is true.
	// Default: "ENV".
	ProductionEnvVar string

	// AllowMemoryInProduction permits :memory: databases when the production
	// environment variable is set. Default: false.
	AllowMemoryInProduction bool

	// SkipMigrations disables automatic migration on Open.
	// By default, migrations run automatically.
	SkipMigrations bool

	// MigrationTimeout bounds migration execution time. Default: 90s.
	MigrationTimeout time.Duration

	// AppVersion is written to the config table after initialization.
	// Leave empty to skip writing app metadata.
	AppVersion string

	// RequiredSchemaVersion, if non-zero, causes Open to verify that the
	// database schema version exactly matches this value after any migrations
	// are applied. Returns an error if the versions don't match.
	// Useful for catching schema/code mismatches at startup.
	RequiredSchemaVersion int
}

// defaults returns a copy of cfg with default values applied.
func (cfg Config) defaults() Config {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.ProductionEnvVar == "" {
		cfg.ProductionEnvVar = "ENV"
	}
	if cfg.MigrationTimeout == 0 {
		cfg.MigrationTimeout = 90 * time.Second
	}
	return cfg
}

// isProduction returns true if the production environment variable is set.
func (cfg Config) isProduction() bool {
	return strings.EqualFold(os.Getenv(cfg.ProductionEnvVar), "production")
}

// isMemory returns true if Path indicates an in-memory database.
func (cfg Config) isMemory() bool {
	return cfg.Path == ":memory:" || strings.HasPrefix(cfg.Path, "file::memory:")
}

// MigrationStatus describes the current schema state.
type MigrationStatus struct {
	SchemaVersion int
	Applied       []AppliedMigration
	Pending       []string
	IsInitialized bool
}

// AppliedMigration describes a migration that has been applied.
type AppliedMigration struct {
	ID        int
	Comment   string
	Path      string
	AppliedAt time.Time
}

// Open opens a database and optionally applies migrations.
// For in-memory databases, it creates and initializes a new database.
// For persistent databases, it opens an existing file (use Create for new files).
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	cfg = cfg.defaults()

	if cfg.isMemory() {
		return openMemory(ctx, cfg)
	}
	return openPersistent(ctx, cfg)
}

// Create creates a new persistent database file and applies migrations.
// Returns an error if the file already exists.
func Create(ctx context.Context, cfg Config) error {
	cfg = cfg.defaults()

	if cfg.isMemory() {
		return fmt.Errorf("Create requires a persistent path, not :memory:")
	}

	if err := validatePersistentPath(cfg.Path); err != nil {
		return err
	}

	if fileExists(cfg.Path) {
		return fmt.Errorf("%s: file already exists", cfg.Path)
	}

	cfg.Logger.Info("creating database", "path", cfg.Path)

	db, err := openAndMigrate(ctx, cfg, persistentPragmas)
	if err != nil {
		return err
	}
	return db.Close()
}

// Delete removes a database file and its WAL sidecar files.
// Returns nil if the file does not exist.
func Delete(ctx context.Context, path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file::memory:") {
		return fmt.Errorf("cannot delete in-memory database")
	}

	if err := validatePersistentPath(path); err != nil {
		return err
	}

	if !fileExists(path) {
		return nil
	}

	// WAL mode creates sidecar files
	var firstErr error
	for _, suffix := range []string{"", "-shm", "-wal"} {
		name := path + suffix
		if !fileExists(name) {
			continue
		}
		if !isRegularFile(name) {
			err := fmt.Errorf("%s: not a regular file", name)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.Remove(name); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if firstErr != nil {
		return fmt.Errorf("delete %s: %w", path, firstErr)
	}

	if fileExists(path) {
		return fmt.Errorf("%s: still exists after delete", path)
	}

	return nil
}

// Status returns the current migration status without modifying the database.
func Status(ctx context.Context, cfg Config) (*MigrationStatus, error) {
	cfg = cfg.defaults()
	cfg.SkipMigrations = true // don't migrate when checking status

	var db *sql.DB
	var err error

	if cfg.isMemory() {
		// For memory DBs, we can't check status of a non-existent DB
		return nil, fmt.Errorf("cannot check status of in-memory database")
	}

	if !fileExists(cfg.Path) {
		return &MigrationStatus{IsInitialized: false}, nil
	}

	db, err = openPersistent(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	return getStatus(ctx, db, cfg)
}

// openMemory opens an in-memory database.
func openMemory(ctx context.Context, cfg Config) (*sql.DB, error) {
	if cfg.isProduction() && !cfg.AllowMemoryInProduction {
		return nil, fmt.Errorf("in-memory database not allowed in production (%s=production)", cfg.ProductionEnvVar)
	}

	cfg.Logger.Info("DB mode: in-memory")
	return openAndMigrate(ctx, cfg, memoryPragmas)
}

// openPersistent opens an existing persistent database.
func openPersistent(ctx context.Context, cfg Config) (*sql.DB, error) {
	if err := validatePersistentPath(cfg.Path); err != nil {
		return nil, err
	}

	if !fileExists(cfg.Path) {
		return nil, fmt.Errorf("%s: database file not found (use Create to make a new database)", cfg.Path)
	}

	cfg.Logger.Info("DB mode: persistent", "path", cfg.Path)
	return openAndMigrate(ctx, cfg, persistentPragmas)
}

// openAndMigrate opens a database with the given pragmas and runs migrations.
func openAndMigrate(ctx context.Context, cfg Config, pragmas []pragma) (*sql.DB, error) {
	dsn := buildDSN(cfg.Path, pragmas)
	cfg.Logger.Debug("opening database", "dsn", dsn)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// Ensure cleanup on error
	success := false
	defer func() {
		if !success {
			db.Close()
		}
	}()

	// SQLite works best with limited connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	if !cfg.SkipMigrations {
		migCtx, cancel := context.WithTimeout(ctx, cfg.MigrationTimeout)
		defer cancel()

		if err := migrate(migCtx, db, cfg); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}

	// Verify schema version if required
	if cfg.RequiredSchemaVersion != 0 {
		version, err := fetchSchemaVersion(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("fetch schema version: %w", err)
		}
		if version == nil {
			return nil, fmt.Errorf("schema version check failed: database not initialized")
		}
		if *version != cfg.RequiredSchemaVersion {
			return nil, fmt.Errorf("schema version mismatch: required %d, found %d", cfg.RequiredSchemaVersion, *version)
		}
	}

	success = true
	return db, nil
}

// validatePersistentPath checks that a path is valid for a persistent database.
func validatePersistentPath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s: persistent database path must be absolute", path)
	}
	if filepath.Ext(path) != ".db" {
		return fmt.Errorf("%s: expected .db extension", path)
	}
	if isDirectory(path) {
		return fmt.Errorf("%s: path is a directory", path)
	}
	dir := filepath.Dir(path)
	if !isDirectory(dir) {
		return fmt.Errorf("%s: parent directory does not exist", dir)
	}
	return nil
}

// File system helpers (inlined from fsck package)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() || info.IsDir()
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// getStatus returns the migration status for an open database.
func getStatus(ctx context.Context, db *sql.DB, cfg Config) (*MigrationStatus, error) {
	status := &MigrationStatus{}

	// Check if initialized
	version, err := fetchSchemaVersion(ctx, db)
	if err != nil {
		return nil, err
	}
	if version == nil {
		status.IsInitialized = false
		return status, nil
	}

	status.IsInitialized = true
	status.SchemaVersion = *version

	// Get applied migrations
	applied, err := fetchAppliedMigrations(ctx, db)
	if err != nil {
		return nil, err
	}
	status.Applied = applied

	// Get pending migrations
	if cfg.Migrations != nil {
		scripts, err := listMigrationFiles(cfg.Migrations, cfg.Logger)
		if err != nil {
			return nil, err
		}

		appliedPaths := make(map[string]bool)
		for _, a := range applied {
			appliedPaths[a.Path] = true
		}

		for _, s := range scripts {
			if !appliedPaths[s.Path] {
				status.Pending = append(status.Pending, s.Path)
			}
		}
	}

	return status, nil
}

// fetchSchemaVersion returns the schema version from the config table.
// Returns nil if the table doesn't exist (uninitialized database).
func fetchSchemaVersion(ctx context.Context, db *sql.DB) (*int, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = 'schema.version'`).Scan(&value)
	if err != nil {
		if isNoSuchTable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch schema.version: %w", err)
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("invalid schema.version %q: %w", value, err)
	}
	return &v, nil
}

// fetchAppliedMigrations returns all applied migrations in order.
func fetchAppliedMigrations(ctx context.Context, db *sql.DB) ([]AppliedMigration, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, comment, path, applied_at FROM schema_migrations ORDER BY path`)
	if err != nil {
		if isNoSuchTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var result []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		var appliedAt int64
		if err := rows.Scan(&m.ID, &m.Comment, &m.Path, &appliedAt); err != nil {
			return nil, err
		}
		m.AppliedAt = time.Unix(appliedAt, 0).UTC()
		result = append(result, m)
	}
	return result, rows.Err()
}

// isNoSuchTable checks if an error indicates a missing table.
func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "no such table")
}
