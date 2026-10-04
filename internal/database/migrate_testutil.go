package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// OpenTestDB connects using DATABASE_URL, resets schema, and applies migrations.
func OpenTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; start Postgres with docker compose and export DATABASE_URL")
	}
	ctx := context.Background()
	db, err := NewDB(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	applyMigrations(t, db)
	return db
}

func applyMigrations(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	// Reset schema for isolated repo tests.
	_, err := db.Pool.Exec(ctx, `
DROP TABLE IF EXISTS wager_transactions;
DROP TABLE IF EXISTS wallet_ledger_entries;
DROP TABLE IF EXISTS wallets;
`)
	if err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	for _, name := range []string{
		"000001_wallets_ledger.up.sql",
		"000002_wager_transactions.up.sql",
		"000003_wager_pending_reference_ttl.up.sql",
	} {
		upSQL, err := os.ReadFile(migrationPath(name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := db.Pool.Exec(ctx, string(upSQL)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func migrationPath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("migrations", name)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "migrations", name)
}
