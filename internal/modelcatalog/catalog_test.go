package modelcatalog_test

import (
	"context"
	"encoding/json"
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
	var preciseImageEdit bool
	var tencentModels int
	var moonshotModels int
	arkThinkingDefaults := make(map[string]string)
	deepSeekModels := make(map[string]bool)
	for _, model := range models {
		if model.ProviderCode == "volcengine" {
			arkModels++
			if model.ModelID == "doubao-seed-2-1-pro-260628" || model.ModelID == "doubao-seed-2-1-turbo-260628" {
				var defaults map[string]any
				if err := json.Unmarshal(model.DefaultParameters, &defaults); err != nil {
					t.Fatalf("decode %s defaults: %v", model.ModelID, err)
				}
				arkThinkingDefaults[model.ModelID], _ = defaults["thinking"].(string)
			}
		}
		if model.ProviderCode == "deepseek" {
			deepSeekModels[model.ModelID] = true
		}
		if model.ProviderCode == "aliyun_bailian" {
			bailianModels++
			if model.ModelID == "wanx2.1-imageedit" {
				preciseImageEdit = containsAll(model.Features, "outpaint", "inpaint", "mask_input", "local_reference")
			}
		}
		if model.ProviderCode == "tencent_tokenhub" {
			tencentModels++
		}
		if model.ProviderCode == "moonshot" {
			moonshotModels++
		}
	}
	if arkModels != 12 {
		t.Fatalf("expected twelve unified Ark models, got %d", arkModels)
	}
	for _, modelID := range []string{"doubao-seed-2-1-pro-260628", "doubao-seed-2-1-turbo-260628"} {
		if arkThinkingDefaults[modelID] != "disabled" {
			t.Fatalf("expected %s to use a supported thinking default, got %q", modelID, arkThinkingDefaults[modelID])
		}
	}
	if !deepSeekModels["deepseek-v4-flash"] || !deepSeekModels["deepseek-v4-pro"] {
		t.Fatalf("expected current DeepSeek V4 catalog, got %#v", deepSeekModels)
	}
	if bailianModels != 13 {
		t.Fatalf("expected thirteen Bailian models, got %d", bailianModels)
	}
	if !preciseImageEdit {
		t.Fatal("expected Wan 2.1 precise image edit capabilities")
	}
	if tencentModels != 11 {
		t.Fatalf("expected eleven TokenHub models, got %d", tencentModels)
	}
	if moonshotModels != 4 {
		t.Fatalf("expected four Moonshot models, got %d", moonshotModels)
	}
}

func containsAll(values []string, expected ...string) bool {
	available := make(map[string]struct{}, len(values))
	for _, value := range values {
		available[value] = struct{}{}
	}
	for _, value := range expected {
		if _, exists := available[value]; !exists {
			return false
		}
	}
	return true
}
