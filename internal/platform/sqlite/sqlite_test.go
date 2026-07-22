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

	var localProviders int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM model_providers
		WHERE (adapter_code='ollama' AND base_url='http://127.0.0.1:11434' AND auth_type='none')
		   OR (adapter_code='comfyui' AND base_url='http://127.0.0.1:8188' AND auth_type='none')`).Scan(&localProviders); err != nil {
		t.Fatal(err)
	}
	if localProviders != 2 {
		t.Fatalf("expected two built-in local model connections, got %d", localProviders)
	}
	for _, table := range []string{"prompt_presets", "voice_profiles"} {
		rows, err := database.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if name == "project_id" {
				rows.Close()
				t.Fatalf("%s must be globally scoped", table)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
