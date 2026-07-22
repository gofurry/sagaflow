package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListPromptPresets(ctx context.Context, capability string) ([]PromptPreset, error) {
	return collectRows[PromptPreset](s.pool.Query(ctx, `
		SELECT * FROM prompt_presets
		WHERE ($1='' OR capability=$1)
		ORDER BY capability,name`, capability))
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
