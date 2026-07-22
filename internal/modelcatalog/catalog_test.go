package modelcatalog_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/modelcatalog"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestSyncCreatesVersionedBuiltinsIdempotently(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := db.New(database)

	first, err := modelcatalog.Sync(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created != len(modelcatalog.Builtins()) {
		t.Fatalf("expected %d built-ins, got %#v", len(modelcatalog.Builtins()), first)
	}
	second, err := modelcatalog.Sync(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Updated != 0 || second.Skipped != 0 {
		t.Fatalf("expected idempotent sync, got %#v", second)
	}
	models, err := store.ListModels(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != len(modelcatalog.Builtins()) {
		t.Fatalf("expected %d catalog models, got %d", len(modelcatalog.Builtins()), len(models))
	}
	var arkModels int
	var bailianModels int
	deepSeekModels := make(map[string]bool)
	for _, model := range models {
		if model.ProviderCode == "volcengine" {
			arkModels++
		}
		if model.ProviderCode == "deepseek" {
			deepSeekModels[model.ModelID] = true
		}
		if model.ProviderCode == "aliyun_bailian" {
			bailianModels++
		}
	}
	if arkModels != 12 {
		t.Fatalf("expected twelve unified Ark models, got %d", arkModels)
	}
	if !deepSeekModels["deepseek-v4-flash"] || !deepSeekModels["deepseek-v4-pro"] {
		t.Fatalf("expected current DeepSeek V4 catalog, got %#v", deepSeekModels)
	}
	if bailianModels != 12 {
		t.Fatalf("expected twelve Bailian models, got %d", bailianModels)
	}
}
