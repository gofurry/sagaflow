// Package promptcatalog owns SagaFlow's versioned built-in prompt presets.
package promptcatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const Version = "2026.07.26.2"

var builtins = concatPresets(
	textPrompts(),
	imagePrompts(),
	audioPrompts(),
	videoPrompts(),
)

func builtinPrompt(id, catalogKey, name, description, capability, content string) db.PromptPreset {
	return db.PromptPreset{
		ID:             uuid.MustParse(id),
		CatalogKey:     catalogKey,
		CatalogVersion: Version,
		Source:         "builtin",
		Enabled:        true,
		Name:           name,
		Description:    description,
		Capability:     capability,
		Content:        strings.TrimSpace(content),
	}
}

func concatPresets(groups ...[]db.PromptPreset) []db.PromptPreset {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	items := make([]db.PromptPreset, 0, total)
	for _, group := range groups {
		items = append(items, group...)
	}
	return items
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
	if err := store.SyncBuiltinPromptPresets(ctx, builtins); err != nil {
		return fmt.Errorf("synchronize built-in prompt presets: %w", err)
	}
	return nil
}
