package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type CreateGenerationReferenceUploadInput struct {
	ID            uuid.UUID
	ProjectID     uuid.UUID
	ObjectID      uuid.UUID
	Name          string
	MediaType     string
	MimeType      string
	FileSizeBytes int64
	Metadata      json.RawMessage
}

func (s *Store) CreateGenerationReferenceUpload(ctx context.Context, input CreateGenerationReferenceUploadInput) (GenerationReferenceUpload, error) {
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
	}
	if input.MimeType == "" {
		input.MimeType = "application/octet-stream"
	}
	return one[GenerationReferenceUpload](s.pool.Query(ctx, `
		INSERT INTO generation_reference_uploads (
			id,project_id,object_id,name,media_type,mime_type,file_size_bytes,metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING *`,
		input.ID, input.ProjectID, input.ObjectID, strings.TrimSpace(input.Name), input.MediaType, input.MimeType, input.FileSizeBytes,
		validJSON(input.Metadata)))
}

func (s *Store) GetGenerationReferenceUpload(ctx context.Context, id uuid.UUID) (GenerationReferenceUpload, error) {
	return one[GenerationReferenceUpload](s.pool.Query(ctx, `SELECT * FROM generation_reference_uploads WHERE id=$1`, id))
}

func (s *Store) ListProjectGenerationReferenceUploads(ctx context.Context, projectID uuid.UUID) ([]GenerationReferenceUpload, error) {
	return collectRows[GenerationReferenceUpload](s.pool.Query(ctx, `
		SELECT * FROM generation_reference_uploads
		WHERE project_id=$1 ORDER BY created_at DESC`, projectID))
}

func (s *Store) DeleteGenerationReferenceUpload(ctx context.Context, id uuid.UUID) (GenerationReferenceUpload, error) {
	upload, err := s.GetGenerationReferenceUpload(ctx, id)
	if err != nil {
		return GenerationReferenceUpload{}, err
	}
	if upload.JobID != nil {
		return GenerationReferenceUpload{}, fmt.Errorf("%w: reference is already attached to a generation job", ErrConflict)
	}
	deleted, err := one[GenerationReferenceUpload](s.pool.Query(ctx, `
		DELETE FROM generation_reference_uploads
		WHERE id=$1 AND job_id IS NULL
		RETURNING *`, id))
	if err != nil {
		return GenerationReferenceUpload{}, err
	}
	return deleted, nil
}
