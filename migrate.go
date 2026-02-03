// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package sqliteinit

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// migrationScript represents a single migration file.
type migrationScript struct {
	ID      int
	Comment string
	Path    string
}

// reMigrationFile matches YYYYMMDDHHMMSS_comment.sql
var reMigrationFile = regexp.MustCompile(`^(\d{14})_(.+)\.sql$`)

// migrate applies pending migrations to the database.
func migrate(ctx context.Context, db *sql.DB, cfg Config) error {
	cfg.Logger.Debug("starting migration")

	// Check current state
	version, err := fetchSchemaVersion(ctx, db)
	if err != nil {
		return err
	}

	needsInit := version == nil

	// If uninitialized, apply the package's schema first
	if needsInit {
		cfg.Logger.Debug("initializing schema")
		if err := applySchemaInit(ctx, db, cfg); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}

	// If no user migrations provided, we're done
	if cfg.Migrations == nil {
		return nil
	}

	// List available migrations
	scripts, err := listMigrationFiles(cfg.Migrations, cfg.Logger)
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	if len(scripts) == 0 {
		cfg.Logger.Debug("no user migrations to apply")
		return nil
	}

	// Get currently applied migrations
	applied, err := fetchAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("fetch applied: %w", err)
	}

	appliedPaths := make(map[string]bool, len(applied))
	for _, a := range applied {
		appliedPaths[a.Path] = true
	}

	// Apply pending migrations
	now := time.Now().UTC()
	for _, s := range scripts {
		if appliedPaths[s.Path] {
			continue
		}

		cfg.Logger.Debug("applying migration", "path", s.Path)
		if err := applyMigration(ctx, db, cfg.Migrations, s, now); err != nil {
			return fmt.Errorf("apply %s: %w", s.Path, err)
		}
	}

	return nil
}

// applySchemaInit applies the package's internal schema initialization script.
func applySchemaInit(ctx context.Context, db *sql.DB, cfg Config) error {
	sqlBytes, err := fs.ReadFile(schemaFS, "schema.sql")
	if err != nil {
		return fmt.Errorf("read schema.sql: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("exec schema.sql: %w", err)
	}

	// Record the init as migration ID 0
	now := time.Now().UTC()
	ts := now.Unix()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (id, comment, path, applied_at, created_at, updated_at)
		VALUES (0, 'init', 'schema.sql', ?, ?, ?)
	`, ts, ts, ts)
	if err != nil {
		return fmt.Errorf("record init: %w", err)
	}

	// Set app metadata if provided
	if cfg.AppVersion != "" {
		_, err = tx.ExecContext(ctx, `UPDATE config SET value = ?, updated_at = ? WHERE key = 'app.version'`, cfg.AppVersion, ts)
		if err != nil {
			return fmt.Errorf("set app.version: %w", err)
		}
		_, err = tx.ExecContext(ctx, `UPDATE config SET value = ?, updated_at = ? WHERE key = 'db.created_at'`, strconv.FormatInt(ts, 10), ts)
		if err != nil {
			return fmt.Errorf("set db.created_at: %w", err)
		}
	}

	return tx.Commit()
}

// applyMigration applies a single user migration script.
func applyMigration(ctx context.Context, db *sql.DB, migrationsFS fs.FS, s migrationScript, now time.Time) error {
	sqlBytes, err := fs.ReadFile(migrationsFS, s.Path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute the migration
	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	// Record the migration
	ts := now.Unix()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (id, comment, path, applied_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.ID, s.Comment, s.Path, ts, ts, ts)
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}

	// Update schema version
	res, err := tx.ExecContext(ctx, `
		UPDATE config SET value = ?, updated_at = ? WHERE key = 'schema.version'
	`, strconv.Itoa(s.ID), ts)
	if err != nil {
		return fmt.Errorf("update schema.version: %w", err)
	}

	// Verify the update succeeded
	rows, err := res.RowsAffected()
	if err == nil && rows != 1 {
		return fmt.Errorf("schema.version update affected %d rows, expected 1", rows)
	}

	return tx.Commit()
}

// listMigrationFiles reads migration scripts from the filesystem.
// Returns scripts sorted in lexicographic order by path.
func listMigrationFiles(migrationsFS fs.FS, logger *slog.Logger) ([]migrationScript, error) {
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return nil, err
	}

	var scripts []migrationScript
	seenIDs := make(map[int]string)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		matches := reMigrationFile.FindStringSubmatch(name)
		if matches == nil {
			logger.Debug("skipping non-migration file", "name", name)
			continue
		}

		id, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid migration id in %q: %w", name, err)
		}

		// Check for duplicate IDs
		if existing, ok := seenIDs[id]; ok {
			return nil, fmt.Errorf("duplicate migration ID %d: %q and %q", id, existing, name)
		}
		seenIDs[id] = name

		scripts = append(scripts, migrationScript{
			ID:      id,
			Comment: matches[2],
			Path:    name,
		})
	}

	// Sort by path (lexicographic order is part of the contract)
	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Path < scripts[j].Path
	})

	return scripts, nil
}
