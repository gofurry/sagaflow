package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestOpenMigratesCleanDatabaseAndEnforcesSingleAccount(t *testing.T) {
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO account (id,username,display_name,password_hash) VALUES (?,?,?,?)`, uuid.NewString(), "creator", "Creator", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO account (id,username,display_name,password_hash) VALUES (?,?,?,?)`, uuid.NewString(), "other", "Other", "hash"); err == nil {
		t.Fatal("expected the database to reject a second account")
	}
	var mode string
	if err := database.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %q", mode)
	}
}
