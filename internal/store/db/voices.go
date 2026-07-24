package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const voiceProfileSelect = `
	SELECT id,name,description,kind,source_name,source_mime_type,source_file_size_bytes,
		reference_text,design_prompt,source_object_id,metadata,created_at,updated_at
	FROM voice_profiles`

const voiceBindingSelect = `
	SELECT b.*,p.code AS provider_code,p.display_name AS provider_name,
		m.model_id AS model_identifier,m.display_name AS model_name
	FROM voice_bindings b
	JOIN model_providers p ON p.id=b.provider_id
	JOIN model_catalog m ON m.id=b.model_id`

func (s *Store) ListVoiceProfiles(ctx context.Context) ([]VoiceProfile, error) {
	items, err := collectRows[VoiceProfile](s.pool.Query(ctx, voiceProfileSelect+` ORDER BY created_at DESC`))
	if err != nil {
		return nil, err
	}
	bindings, err := s.ListVoiceBindings(ctx, nil)
	if err != nil {
		return nil, err
	}
	byProfile := make(map[uuid.UUID][]VoiceBinding, len(items))
	for _, binding := range bindings {
		byProfile[binding.VoiceProfileID] = append(byProfile[binding.VoiceProfileID], binding)
	}
	for index := range items {
		items[index].Bindings = byProfile[items[index].ID]
		if items[index].Bindings == nil {
			items[index].Bindings = []VoiceBinding{}
		}
	}
	return items, nil
}

func (s *Store) GetVoiceProfile(ctx context.Context, id uuid.UUID) (VoiceProfile, error) {
	item, err := one[VoiceProfile](s.pool.Query(ctx, voiceProfileSelect+` WHERE id=$1`, id))
	if err != nil {
		return VoiceProfile{}, err
	}
	bindings, err := s.ListVoiceBindings(ctx, &id)
	if err != nil {
		return VoiceProfile{}, err
	}
	item.Bindings = bindings
	return item, nil
}

func (s *Store) CreateVoiceProfile(ctx context.Context, profile VoiceProfile) (VoiceProfile, error) {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO voice_profiles (
			id,name,description,kind,source_name,source_mime_type,source_file_size_bytes,
			reference_text,design_prompt,source_object_id,metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		profile.ID, strings.TrimSpace(profile.Name), strings.TrimSpace(profile.Description), profile.Kind,
		profile.SourceName, profile.SourceMimeType, profile.SourceFileSizeBytes,
		strings.TrimSpace(profile.ReferenceText), strings.TrimSpace(profile.DesignPrompt),
		profile.SourceObjectID, validJSON(profile.Metadata))
	if err != nil {
		return VoiceProfile{}, err
	}
	return s.GetVoiceProfile(ctx, profile.ID)
}

func (s *Store) UpdateVoiceProfile(ctx context.Context, id uuid.UUID, name, description string) (VoiceProfile, error) {
	result, err := s.pool.Exec(ctx, `UPDATE voice_profiles SET name=$2,description=$3,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, strings.TrimSpace(name), strings.TrimSpace(description))
	if err != nil {
		return VoiceProfile{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return VoiceProfile{}, ErrNotFound
	}
	return s.GetVoiceProfile(ctx, id)
}

func (s *Store) ConfirmDeleteVoiceProfile(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM voice_profiles WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func (s *Store) ListVoiceBindings(ctx context.Context, profileID *uuid.UUID) ([]VoiceBinding, error) {
	return collectRows[VoiceBinding](s.pool.Query(ctx, voiceBindingSelect+`
		WHERE ($1 IS NULL OR b.voice_profile_id=$1)
		ORDER BY b.created_at`, profileID))
}

func (s *Store) GetVoiceBinding(ctx context.Context, id uuid.UUID) (VoiceBinding, error) {
	return one[VoiceBinding](s.pool.Query(ctx, voiceBindingSelect+` WHERE b.id=$1`, id))
}

func (s *Store) CreateVoiceBinding(ctx context.Context, binding VoiceBinding) (VoiceBinding, error) {
	if binding.ID == uuid.Nil {
		binding.ID = uuid.New()
	}
	if binding.Status == "" {
		binding.Status = "ready"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO voice_bindings (
			id,voice_profile_id,provider_id,model_id,operation,voice_id,status,preview_text,
			preview_mime_type,preview_file_size_bytes,preview_object_id,provider_file_id,
			provider_prompt_file_id,activated_at,metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		binding.ID, binding.VoiceProfileID, binding.ProviderID, binding.ModelID, binding.Operation,
		strings.TrimSpace(binding.VoiceID), binding.Status, strings.TrimSpace(binding.PreviewText),
		binding.PreviewMimeType, binding.PreviewFileSizeBytes, binding.PreviewObjectID,
		binding.ProviderFileID, binding.ProviderPromptFileID, binding.ActivatedAt, validJSON(binding.Metadata))
	if err != nil {
		return VoiceBinding{}, err
	}
	return s.GetVoiceBinding(ctx, binding.ID)
}

func (s *Store) ConfirmDeleteVoiceBinding(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM voice_bindings WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}
