package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListModelProviders(ctx context.Context) ([]ModelProvider, error) {
	return collectRows[ModelProvider](s.pool.Query(ctx, `SELECT * FROM model_providers ORDER BY display_name`))
}

func (s *Store) GetModelProvider(ctx context.Context, id uuid.UUID) (ModelProvider, error) {
	return one[ModelProvider](s.pool.Query(ctx, `SELECT * FROM model_providers WHERE id=$1`, id))
}

func (s *Store) GetModelProviderByCode(ctx context.Context, code string) (ModelProvider, error) {
	return one[ModelProvider](s.pool.Query(ctx, `SELECT * FROM model_providers WHERE code=$1`, strings.ToLower(strings.TrimSpace(code))))
}

func (s *Store) CreateModelProvider(ctx context.Context, provider ModelProvider) (ModelProvider, error) {
	if provider.ID == uuid.Nil {
		provider.ID = uuid.New()
	}
	if provider.MaxConcurrency <= 0 {
		provider.MaxConcurrency = 1
	}
	return one[ModelProvider](s.pool.Query(ctx, `
		INSERT INTO model_providers (id,code,adapter_code,display_name,base_url,auth_type,capabilities,enabled,metadata,max_concurrency)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *`, provider.ID,
		strings.ToLower(strings.TrimSpace(provider.Code)), strings.ToLower(strings.TrimSpace(provider.AdapterCode)),
		strings.TrimSpace(provider.DisplayName), strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/"), provider.AuthType,
		JSON(nonNilStrings(provider.Capabilities)), provider.Enabled, validJSON(provider.Metadata), provider.MaxConcurrency))
}

func (s *Store) UpdateModelProvider(ctx context.Context, provider ModelProvider) (ModelProvider, error) {
	if provider.MaxConcurrency <= 0 {
		provider.MaxConcurrency = 1
	}
	return one[ModelProvider](s.pool.Query(ctx, `
		UPDATE model_providers SET code=$2,adapter_code=$3,display_name=$4,base_url=$5,auth_type=$6,
		capabilities=$7,enabled=$8,metadata=$9,max_concurrency=$10,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 RETURNING *`, provider.ID, strings.ToLower(strings.TrimSpace(provider.Code)), strings.ToLower(strings.TrimSpace(provider.AdapterCode)),
		strings.TrimSpace(provider.DisplayName), strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/"), provider.AuthType,
		JSON(nonNilStrings(provider.Capabilities)), provider.Enabled, validJSON(provider.Metadata), provider.MaxConcurrency))
}

func (s *Store) DeleteModelProvider(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM model_providers WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("%w: 该连接已被模型、生成记录或项目资源引用，请改为停用", ErrConflict)
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListModels(ctx context.Context, capability string) ([]Model, error) {
	return collectRows[Model](s.pool.Query(ctx, `
		SELECT m.*,p.code AS provider_code,p.display_name AS provider_name
		FROM model_catalog m JOIN model_providers p ON p.id=m.provider_id
		WHERE ($1='' OR m.capability=$1) ORDER BY m.capability,p.display_name,m.display_name`, capability))
}

func (s *Store) GetModel(ctx context.Context, id uuid.UUID) (Model, error) {
	return one[Model](s.pool.Query(ctx, `
		SELECT m.*,p.code AS provider_code,p.display_name AS provider_name
		FROM model_catalog m JOIN model_providers p ON p.id=m.provider_id WHERE m.id=$1`, id))
}

func (s *Store) CreateModel(ctx context.Context, model Model) (Model, error) {
	if model.ID == uuid.Nil {
		model.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO model_catalog (id,provider_id,model_id,display_name,capability,input_modalities,features,parameter_schema,default_parameters,enabled,available,metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, model.ID, model.ProviderID,
		strings.TrimSpace(model.ModelID), strings.TrimSpace(model.DisplayName), model.Capability,
		JSON(nonNilStrings(model.InputModalities)), JSON(nonNilStrings(model.Features)), validJSON(model.ParameterSchema), validJSON(model.DefaultParameters),
		model.Enabled, model.Available, validJSON(model.Metadata))
	if err != nil {
		return Model{}, err
	}
	return s.GetModel(ctx, model.ID)
}

func (s *Store) UpdateModel(ctx context.Context, model Model) (Model, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE model_catalog SET provider_id=$2,model_id=$3,display_name=$4,capability=$5,input_modalities=$6,features=$7,
		parameter_schema=$8,default_parameters=$9,enabled=$10,available=$11,metadata=$12,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, model.ID, model.ProviderID,
		strings.TrimSpace(model.ModelID), strings.TrimSpace(model.DisplayName), model.Capability,
		JSON(nonNilStrings(model.InputModalities)), JSON(nonNilStrings(model.Features)), validJSON(model.ParameterSchema), validJSON(model.DefaultParameters),
		model.Enabled, model.Available, validJSON(model.Metadata))
	if err != nil {
		return Model{}, err
	}
	return s.GetModel(ctx, model.ID)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (s *Store) ListModelsByProvider(ctx context.Context, providerID uuid.UUID) ([]Model, error) {
	return collectRows[Model](s.pool.Query(ctx, `
		SELECT m.*,p.code AS provider_code,p.display_name AS provider_name
		FROM model_catalog m JOIN model_providers p ON p.id=m.provider_id
		WHERE m.provider_id=$1 ORDER BY m.display_name`, providerID))
}

func (s *Store) ListModelPresets(ctx context.Context, modelID *uuid.UUID) ([]ModelPreset, error) {
	return collectRows[ModelPreset](s.pool.Query(ctx, `SELECT * FROM model_presets WHERE ($1 IS NULL OR model_id=$1) ORDER BY model_id,is_default DESC,name`, modelID))
}

func (s *Store) GetModelPreset(ctx context.Context, id uuid.UUID) (ModelPreset, error) {
	return one[ModelPreset](s.pool.Query(ctx, `SELECT * FROM model_presets WHERE id=$1`, id))
}

func (s *Store) CreateModelPreset(ctx context.Context, preset ModelPreset) (ModelPreset, error) {
	if preset.ID == uuid.Nil {
		preset.ID = uuid.New()
	}
	var created ModelPreset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		if preset.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE model_presets SET is_default=0 WHERE model_id=$1`, preset.ModelID); err != nil {
				return err
			}
		}
		var err error
		created, err = one[ModelPreset](tx.Query(ctx, `
			INSERT INTO model_presets (id,model_id,name,parameters,is_default) VALUES ($1,$2,$3,$4,$5) RETURNING *`,
			preset.ID, preset.ModelID, strings.TrimSpace(preset.Name), validJSON(preset.Parameters), preset.IsDefault))
		return err
	})
	return created, err
}

func (s *Store) UpdateModelPreset(ctx context.Context, preset ModelPreset) (ModelPreset, error) {
	var updated ModelPreset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[ModelPreset](tx.Query(ctx, `SELECT * FROM model_presets WHERE id=$1`, preset.ID))
		if err != nil {
			return err
		}
		preset.ModelID = current.ModelID
		if preset.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE model_presets SET is_default=0 WHERE model_id=$1 AND id<>$2`, preset.ModelID, preset.ID); err != nil {
				return err
			}
		}
		updated, err = one[ModelPreset](tx.Query(ctx, `
			UPDATE model_presets SET name=$2,parameters=$3,is_default=$4,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id=$1 RETURNING *`, preset.ID, strings.TrimSpace(preset.Name), validJSON(preset.Parameters), preset.IsDefault))
		return err
	})
	return updated, err
}

func (s *Store) DeleteModelPreset(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM model_presets WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func JSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
