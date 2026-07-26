package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListPromptPresets(ctx context.Context, capability string) ([]PromptPreset, error) {
	return collectRows[PromptPreset](s.pool.Query(ctx, `
		SELECT * FROM prompt_presets
		WHERE enabled=1 AND ($1='' OR capability=$1)
		ORDER BY source,name`, capability))
}

func (s *Store) SyncBuiltinPromptPresets(ctx context.Context, presets []PromptPreset) error {
	return withTx(ctx, s.pool, func(tx *Tx) error {
		catalogKeys := make([]string, len(presets))
		for index, preset := range presets {
			catalogKeys[index] = strings.TrimSpace(preset.CatalogKey)
		}
		if len(catalogKeys) == 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM prompt_presets WHERE source='builtin'`); err != nil {
				return err
			}
		} else {
			placeholders := make([]string, len(catalogKeys))
			arguments := make([]any, len(catalogKeys))
			for index, catalogKey := range catalogKeys {
				placeholders[index] = "?"
				arguments[index] = catalogKey
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM prompt_presets
				WHERE source='builtin' AND catalog_key NOT IN (`+strings.Join(placeholders, ",")+`)`, arguments...); err != nil {
				return err
			}
		}

		for _, preset := range presets {
			if _, err := tx.Exec(ctx, `
				INSERT INTO prompt_presets (
					id,model_id,model_preset_id,catalog_key,source,catalog_version,enabled,name,description,capability,content
				) VALUES ($1,NULL,NULL,$2,'builtin',$3,$4,$5,$6,$7,$8)
				ON CONFLICT(catalog_key) WHERE source='builtin' DO UPDATE SET
					catalog_version=excluded.catalog_version,name=excluded.name,description=excluded.description,
					capability=excluded.capability,content=excluded.content,
					updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
				preset.ID, strings.TrimSpace(preset.CatalogKey), strings.TrimSpace(preset.CatalogVersion),
				preset.Enabled, strings.TrimSpace(preset.Name), strings.TrimSpace(preset.Description),
				preset.Capability, preset.Content); err != nil {
				return fmt.Errorf("upsert built-in prompt preset %s: %w", preset.CatalogKey, err)
			}
		}
		return nil
	})
}

func (s *Store) GetPromptPreset(ctx context.Context, id uuid.UUID) (PromptPreset, error) {
	return one[PromptPreset](s.pool.Query(ctx, `SELECT * FROM prompt_presets WHERE id=$1`, id))
}

func (s *Store) CreatePromptPreset(ctx context.Context, preset PromptPreset) (PromptPreset, error) {
	return one[PromptPreset](s.pool.Query(ctx, `
		INSERT INTO prompt_presets (id,model_id,model_preset_id,name,description,capability,content)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *`,
		uuid.New(), preset.ModelID, preset.ModelPresetID, strings.TrimSpace(preset.Name), strings.TrimSpace(preset.Description), preset.Capability, preset.Content))
}

func (s *Store) UpdatePromptPreset(ctx context.Context, preset PromptPreset) (PromptPreset, error) {
	return one[PromptPreset](s.pool.Query(ctx, `
		UPDATE prompt_presets
		SET model_id=$2,model_preset_id=$3,name=$4,description=$5,capability=$6,content=$7,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 RETURNING *`,
		preset.ID, preset.ModelID, preset.ModelPresetID, strings.TrimSpace(preset.Name), strings.TrimSpace(preset.Description), preset.Capability, preset.Content))
}

func (s *Store) DeletePromptPreset(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM prompt_presets WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}
