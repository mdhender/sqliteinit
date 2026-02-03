// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package sqliteinit_test

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mdhender/sqliteinit"
	_ "modernc.org/sqlite"
)

//go:embed testdata/valid/*.sql
var validMigrationsFS embed.FS

//go:embed testdata/invalid/*.sql
var invalidMigrationsFS embed.FS

// validMigrations returns a sub-filesystem rooted at the valid migrations directory.
func validMigrations() fs.FS {
	sub, err := fs.Sub(validMigrationsFS, "testdata/valid")
	if err != nil {
		panic(err)
	}
	return sub
}

// invalidMigrations returns a sub-filesystem rooted at the invalid migrations directory.
func invalidMigrations() fs.FS {
	sub, err := fs.Sub(invalidMigrationsFS, "testdata/invalid")
	if err != nil {
		panic(err)
	}
	return sub
}

// TestOpen_Memory tests opening an in-memory database.
func TestOpen_Memory(t *testing.T) {
	ctx := context.Background()

	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path: ":memory:",
	})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify schema is initialized
	var version string
	err = db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = 'schema.version'`).Scan(&version)
	if err != nil {
		t.Fatalf("query schema.version: %v", err)
	}
	if version != "0" {
		t.Errorf("expected schema.version '0', got %q", version)
	}
}

// TestOpen_Memory_WithMigrations tests migrations on in-memory database.
func TestOpen_Memory_WithMigrations(t *testing.T) {
	ctx := context.Background()

	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:       ":memory:",
		Migrations: validMigrations(),
	})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Check that user migrations were applied
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		t.Fatalf("query migrations count: %v", err)
	}
	// Should have init + 2 user migrations
	if count != 3 {
		t.Errorf("expected 3 migrations (init + 2 user), got %d", count)
	}

	// Check users table exists (from migration)
	_, err = db.ExecContext(ctx, `SELECT 1 FROM users LIMIT 1`)
	if err != nil {
		t.Errorf("users table should exist: %v", err)
	}
}

// TestOpen_Memory_ProductionRejection tests that memory DBs are rejected in production.
func TestOpen_Memory_ProductionRejection(t *testing.T) {
	// Set production environment
	os.Setenv("TEST_ENV", "production")
	defer os.Unsetenv("TEST_ENV")

	ctx := context.Background()

	_, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:             ":memory:",
		ProductionEnvVar: "TEST_ENV",
	})
	if err == nil {
		t.Fatal("expected error for memory DB in production")
	}
}

// TestOpen_Memory_ProductionAllowed tests AllowMemoryInProduction flag.
func TestOpen_Memory_ProductionAllowed(t *testing.T) {
	os.Setenv("TEST_ENV", "production")
	defer os.Unsetenv("TEST_ENV")

	ctx := context.Background()

	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:                    ":memory:",
		ProductionEnvVar:        "TEST_ENV",
		AllowMemoryInProduction: true,
	})
	if err != nil {
		t.Fatalf("Open should succeed with AllowMemoryInProduction: %v", err)
	}
	db.Close()
}

// TestOpen_Memory_AppVersion tests that AppVersion is written to config.
func TestOpen_Memory_AppVersion(t *testing.T) {
	ctx := context.Background()

	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:       ":memory:",
		AppVersion: "1.2.3-test",
	})
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var appVersion string
	err = db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = 'app.version'`).Scan(&appVersion)
	if err != nil {
		t.Fatalf("query app.version: %v", err)
	}
	if appVersion != "1.2.3-test" {
		t.Errorf("expected app.version '1.2.3-test', got %q", appVersion)
	}

	var dbCreatedAt string
	err = db.QueryRowContext(ctx, `SELECT value FROM config WHERE key = 'db.created_at'`).Scan(&dbCreatedAt)
	if err != nil {
		t.Fatalf("query db.created_at: %v", err)
	}
	if dbCreatedAt == "" {
		t.Error("db.created_at should be set when AppVersion is provided")
	}
}

// TestCreate_Persistent tests creating a new persistent database.
func TestCreate_Persistent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	err := sqliteinit.Create(ctx, sqliteinit.Config{
		Path:       path,
		Migrations: validMigrations(),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("database file should exist")
	}

	// Open and verify
	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:       path,
		Migrations: validMigrations(),
	})
	if err != nil {
		t.Fatalf("Open after Create failed: %v", err)
	}
	defer db.Close()

	var count int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	// Table should exist
}

// TestCreate_Persistent_ExistingFile tests that Create fails if file exists.
func TestCreate_Persistent_ExistingFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create first time
	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Create second time should fail
	err = sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err == nil {
		t.Fatal("Create should fail when file exists")
	}
}

// TestCreate_RelativePath tests that relative paths are rejected.
func TestCreate_RelativePath(t *testing.T) {
	ctx := context.Background()

	err := sqliteinit.Create(ctx, sqliteinit.Config{
		Path: "relative/path/test.db",
	})
	if err == nil {
		t.Fatal("Create should reject relative paths")
	}
}

// TestCreate_NoExtension tests that paths without .db are rejected.
func TestCreate_NoExtension(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test")

	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err == nil {
		t.Fatal("Create should reject paths without .db extension")
	}
}

// TestCreate_ParentDirMissing tests that missing parent directories are rejected.
func TestCreate_ParentDirMissing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "test.db")

	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err == nil {
		t.Fatal("Create should reject paths with missing parent directory")
	}
}

// TestCreate_Memory tests that Create rejects memory paths.
func TestCreate_Memory(t *testing.T) {
	ctx := context.Background()

	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: ":memory:"})
	if err == nil {
		t.Fatal("Create should reject :memory:")
	}
}

// TestOpen_Persistent_MissingFile tests that Open fails for missing files.
func TestOpen_Persistent_MissingFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.db")

	_, err := sqliteinit.Open(ctx, sqliteinit.Config{Path: path})
	if err == nil {
		t.Fatal("Open should fail for missing file")
	}
}

// TestDelete tests deleting a database and its sidecars.
func TestDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create database
	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Open and close to potentially create WAL files
	db, _ := sqliteinit.Open(ctx, sqliteinit.Config{Path: path})
	db.Close()

	// Delete
	err = sqliteinit.Delete(ctx, path)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("database file should not exist after delete")
	}
}

// TestDelete_NonExistent tests that deleting a non-existent file is OK.
func TestDelete_NonExistent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.db")

	err := sqliteinit.Delete(ctx, path)
	if err != nil {
		t.Fatalf("Delete of non-existent should succeed: %v", err)
	}
}

// TestDelete_Memory tests that deleting memory DB fails.
func TestDelete_Memory(t *testing.T) {
	ctx := context.Background()

	err := sqliteinit.Delete(ctx, ":memory:")
	if err == nil {
		t.Fatal("Delete should reject :memory:")
	}
}

// TestStatus tests getting migration status.
func TestStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create without some migrations
	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Check status with migrations FS
	status, err := sqliteinit.Status(ctx, sqliteinit.Config{
		Path:       path,
		Migrations: validMigrations(),
	})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if !status.IsInitialized {
		t.Error("expected IsInitialized=true")
	}
	if len(status.Pending) != 2 {
		t.Errorf("expected 2 pending migrations, got %d", len(status.Pending))
	}
}

// TestStatus_Uninitialized tests status of non-existent database.
func TestStatus_Uninitialized(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.db")

	status, err := sqliteinit.Status(ctx, sqliteinit.Config{Path: path})
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status.IsInitialized {
		t.Error("expected IsInitialized=false for non-existent DB")
	}
}

// TestMigrate_DuplicateID tests that duplicate migration IDs are rejected.
func TestMigrate_DuplicateID(t *testing.T) {
	ctx := context.Background()

	_, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:       ":memory:",
		Migrations: invalidMigrations(),
	})
	if err == nil {
		t.Fatal("expected error for duplicate migration IDs")
	}
}

// TestMigrate_Timeout tests migration timeout behavior.
func TestMigrate_Timeout(t *testing.T) {
	ctx := context.Background()

	// Use a very short timeout
	_, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:             ":memory:",
		Migrations:       validMigrations(),
		MigrationTimeout: 1 * time.Nanosecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestOpen_SkipMigrations tests opening without auto-migration.
func TestOpen_SkipMigrations(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	// Create first
	err := sqliteinit.Create(ctx, sqliteinit.Config{Path: path})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Open with SkipMigrations=true and pending migrations
	db, err := sqliteinit.Open(ctx, sqliteinit.Config{
		Path:           path,
		Migrations:     validMigrations(),
		SkipMigrations: true,
	})
	if err != nil {
		t.Fatalf("Open with SkipMigrations=true failed: %v", err)
	}
	defer db.Close()

	// Migrations should not have been applied
	var count int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != 1 { // only init
		t.Errorf("expected only init migration (1), got %d", count)
	}
}
