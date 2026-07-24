package promptcatalog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/promptcatalog"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestSyncIsIdempotent(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := db.New(database)
	if err := promptcatalog.Sync(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := promptcatalog.Sync(ctx, store); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPromptPresets(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Source != "builtin" || items[0].ModelID != nil || items[0].ModelPresetID != nil {
		t.Fatalf("unexpected built-in prompts: %#v", items)
	}
}
