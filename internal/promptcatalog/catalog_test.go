package promptcatalog_test

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/promptcatalog"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

var catalogKeyPattern = regexp.MustCompile(`^[a-z]+(\.[a-z0-9-]+)+$`)

var expectedCatalog = map[string]string{
	"text.story.concept-expand":             "故事创意扩写",
	"text.story.episode-outline":            "分集大纲生成",
	"text.character.profile":                "人物设定卡",
	"text.production.asset-extraction":      "场景与道具提取",
	"text.script.dialogue-polish":           "对白润色",
	"text.general.comic-creation-assistant": "通用漫剧创作助手",
	"image.generate.character-sheet":        "角色标准设定图",
	"image.generate.expression-pose-sheet":  "角色表情与动作表",
	"image.generate.scene-keyframe":         "场景关键帧",
	"image.generate.storyboard-frame":       "分镜关键帧",
	"audio.speech.narration":                "旁白表演",
	"video.generate.image-to-video":         "通用图生视频镜头",
}

func TestBuiltinsAreCompleteAndUnique(t *testing.T) {
	items := promptcatalog.Builtins()
	if len(items) != 12 {
		t.Fatalf("built-in prompt count = %d, want 12", len(items))
	}

	wantCapabilities := map[string]int{"text": 6, "image": 4, "audio": 1, "video": 1}
	gotCapabilities := make(map[string]int, len(wantCapabilities))
	ids := make(map[uuid.UUID]string, len(items))
	keys := make(map[string]uuid.UUID, len(items))
	for _, item := range items {
		if item.ID == uuid.Nil {
			t.Fatalf("prompt %q has nil ID", item.Name)
		}
		if previous, exists := ids[item.ID]; exists {
			t.Fatalf("duplicate ID %s for %q and %q", item.ID, previous, item.Name)
		}
		ids[item.ID] = item.Name

		if strings.TrimSpace(item.CatalogKey) == "" {
			t.Fatalf("prompt %q has empty catalog key", item.Name)
		}
		if previous, exists := keys[item.CatalogKey]; exists {
			t.Fatalf("duplicate catalog key %q for %s and %s", item.CatalogKey, previous, item.ID)
		}
		keys[item.CatalogKey] = item.ID
		if !catalogKeyPattern.MatchString(item.CatalogKey) {
			t.Errorf("catalog key %q does not follow the naming convention", item.CatalogKey)
		}
		if expectedName, exists := expectedCatalog[item.CatalogKey]; !exists {
			t.Errorf("unexpected built-in prompt %q", item.CatalogKey)
		} else if item.Name != expectedName {
			t.Errorf("prompt %q name = %q, want %q", item.CatalogKey, item.Name, expectedName)
		}

		if _, allowed := wantCapabilities[item.Capability]; !allowed {
			t.Errorf("prompt %q has unsupported capability %q", item.Name, item.Capability)
		}
		gotCapabilities[item.Capability]++
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" || strings.TrimSpace(item.Content) == "" {
			t.Errorf("prompt %q has incomplete display content", item.CatalogKey)
		}
		if len([]rune(item.Content)) < 200 {
			t.Errorf("prompt %q is unexpectedly short", item.CatalogKey)
		}
		if item.CatalogVersion != promptcatalog.Version {
			t.Errorf("prompt %q version = %q, want %q", item.CatalogKey, item.CatalogVersion, promptcatalog.Version)
		}
		if item.Source != "builtin" || !item.Enabled {
			t.Errorf("prompt %q source/enabled = %q/%t, want builtin/true", item.CatalogKey, item.Source, item.Enabled)
		}
		if item.ModelID != nil || item.ModelPresetID != nil {
			t.Errorf("built-in prompt %q is unexpectedly bound to a model", item.CatalogKey)
		}
	}

	for capability, want := range wantCapabilities {
		if got := gotCapabilities[capability]; got != want {
			t.Errorf("%s prompt count = %d, want %d", capability, got, want)
		}
	}
}

func TestBuiltinsReturnsIndependentSlice(t *testing.T) {
	first := promptcatalog.Builtins()
	first[0].Name = "changed"
	second := promptcatalog.Builtins()
	if second[0].Name == "changed" {
		t.Fatal("Builtins returned mutable catalog storage")
	}
}

func TestSyncPrunesRetiredBuiltinsAndPreservesUserState(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := db.New(database)

	staleID := uuid.MustParse("30000000-0000-0000-0000-000000000099")
	if _, err := database.ExecContext(ctx, `
		INSERT INTO prompt_presets (
			id,catalog_key,source,catalog_version,enabled,name,description,capability,content
		) VALUES ($1,'audio.stale.test','builtin','stale',1,'过期项','仅用于同步测试','audio','stale')`, staleID); err != nil {
		t.Fatal(err)
	}
	userPrompt, err := store.CreatePromptPreset(ctx, db.PromptPreset{
		Name:        "用户自建 Prompt",
		Description: "同步内置目录时必须保留",
		Capability:  "text",
		Content:     "user content",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := promptcatalog.Sync(ctx, store); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListPromptPresets(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 13 {
		t.Fatalf("prompt count after first sync = %d, want 13", len(items))
	}
	if _, err := store.GetPromptPreset(ctx, staleID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("stale built-in prompt lookup error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetPromptPreset(ctx, userPrompt.ID); err != nil {
		t.Fatalf("user prompt was removed during catalog sync: %v", err)
	}

	var disabled db.PromptPreset
	for _, item := range items {
		if item.Source == "builtin" {
			disabled = item
			break
		}
	}
	if disabled.ID == uuid.Nil {
		t.Fatal("no built-in prompt found to test disabled state")
	}
	if _, err := database.ExecContext(ctx, `UPDATE prompt_presets SET enabled=0 WHERE id=$1`, disabled.ID); err != nil {
		t.Fatal(err)
	}
	if err := promptcatalog.Sync(ctx, store); err != nil {
		t.Fatal(err)
	}

	allEnabled, err := store.ListPromptPresets(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(allEnabled) != 12 {
		t.Fatalf("enabled prompt count after second sync = %d, want 12", len(allEnabled))
	}
	preserved, err := store.GetPromptPreset(ctx, disabled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Enabled {
		t.Fatal("catalog sync overwrote the user's disabled state")
	}
	if preserved.CatalogVersion != promptcatalog.Version {
		t.Fatalf("disabled prompt version = %q, want %q", preserved.CatalogVersion, promptcatalog.Version)
	}
}

func TestSyncRejectsNilStore(t *testing.T) {
	if err := promptcatalog.Sync(context.Background(), nil); err == nil {
		t.Fatal("expected nil store error")
	}
}
