// Package promptcatalog owns SagaFlow's versioned built-in prompt presets.
package promptcatalog

import (
	"context"
	"fmt"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const Version = "2026.07.24.1"

var builtins = []db.PromptPreset{
	{
		ID:             uuid.MustParse("30000000-0000-0000-0000-000000000001"),
		CatalogKey:     "general-comic-creation-assistant",
		CatalogVersion: Version,
		Source:         "builtin",
		Enabled:        true,
		Name:           "通用漫剧创作助手",
		Description:    "模型无关的基础创作约束，适合作为第一次使用时的通用起点。",
		Capability:     "text",
		Content:        "你是一名专业的漫剧创作助手。请根据用户提供的素材完成任务，保持人物设定、世界观和叙事连续性；信息不足时明确列出合理假设；输出简洁、结构清晰，并可直接用于后续制作。",
	},
}

func Builtins() []db.PromptPreset {
	items := make([]db.PromptPreset, len(builtins))
	copy(items, builtins)
	return items
}

func Sync(ctx context.Context, store *db.Store) error {
	if store == nil {
		return fmt.Errorf("prompt catalog store is required")
	}
	for _, preset := range builtins {
		if _, err := store.UpsertBuiltinPromptPreset(ctx, preset); err != nil {
			return fmt.Errorf("synchronize prompt preset %s: %w", preset.CatalogKey, err)
		}
	}
	return nil
}
