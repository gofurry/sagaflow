package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/google/uuid"
)

func TestCreateAndRestoreCompleteWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := config.Default()
	source.App.DataDir = filepath.Join(root, "source")
	if err := source.EnsureRuntimeDirs(); err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.Open(ctx, source.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects (id,title,description) VALUES (?,?,?)`, uuid.NewString(), "Story", "Local project"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	projectFiles := filepath.Join(source.App.DataDir, "projects", uuid.NewString(), "assets")
	if err := os.MkdirAll(projectFiles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectFiles, "asset.bin"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.MasterKeyPath(), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "workspace.zip")
	if _, err := Create(ctx, source, archivePath); err != nil {
		t.Fatal(err)
	}

	target := config.Default()
	target.App.DataDir = filepath.Join(root, "target")
	previous, err := Restore(ctx, target, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if previous != "" {
		t.Fatalf("did not expect a previous directory, got %s", previous)
	}
	entries, err := filepath.Glob(filepath.Join(target.App.DataDir, "projects", "*", "assets", "asset.bin"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("restored project files: %v, %v", entries, err)
	}
	if data, err := os.ReadFile(entries[0]); err != nil || string(data) != "asset" {
		t.Fatalf("restored object: %q, %v", data, err)
	}
	if data, err := os.ReadFile(target.MasterKeyPath()); err != nil || string(data) != "secret" {
		t.Fatalf("restored master key: %q, %v", data, err)
	}
	restored, err := sqlite.Open(ctx, target.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var count int
	if err := restored.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("restored projects: %d, %v", count, err)
	}
}
