package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const voiceProfileSelect = `
	SELECT v.*, p.code AS provider_code, m.model_id AS model_identifier
	FROM voice_profiles v
	JOIN model_providers p ON p.id=v.provider_id
	JOIN model_catalog m ON m.id=v.model_id`

func (s *Store) ListVoiceProfiles(ctx context.Context, projectID uuid.UUID) ([]VoiceProfile, error) {
	return collectRows[VoiceProfile](s.pool.Query(ctx, voiceProfileSelect+` WHERE v.project_id=$1 ORDER BY v.created_at DESC`, projectID))
}

func (s *Store) GetVoiceProfile(ctx context.Context, id uuid.UUID) (VoiceProfile, error) {
	return one[VoiceProfile](s.pool.Query(ctx, voiceProfileSelect+` WHERE v.id=$1`, id))
}

func (s *Store) CreateVoiceProfile(ctx context.Context, profile VoiceProfile) (VoiceProfile, error) {
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	if profile.Status == "" {
		profile.Status = "ready"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO voice_profiles (
			id,project_id,provider_id,model_id,name,description,voice_id,status,
			source_name,source_mime_type,source_file_size_bytes,prompt_text,
			preview_mime_type,preview_file_size_bytes,
			source_object_id,prompt_object_id,preview_object_id,provider_file_id,provider_prompt_file_id,activated_at,metadata
		)
		VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
		)`,
		profile.ID, profile.ProjectID, profile.ProviderID, profile.ModelID, strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.Description), strings.TrimSpace(profile.VoiceID), profile.Status,
		profile.SourceName, profile.SourceMimeType, profile.SourceFileSizeBytes, profile.PromptText,
		profile.PreviewMimeType, profile.PreviewFileSizeBytes, profile.SourceObjectID, profile.PromptObjectID, profile.PreviewObjectID,
		profile.ProviderFileID, profile.ProviderPromptFileID, profile.ActivatedAt, validJSON(profile.Metadata))
	if err != nil {
		return VoiceProfile{}, err
	}
	return s.GetVoiceProfile(ctx, profile.ID)
}

func (s *Store) UpdateVoiceProfile(ctx context.Context, id uuid.UUID, name, description string) (VoiceProfile, error) {
	_, err := s.pool.Exec(ctx, `UPDATE voice_profiles SET name=$2,description=$3,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, strings.TrimSpace(name), strings.TrimSpace(description))
	if err != nil {
		return VoiceProfile{}, err
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
