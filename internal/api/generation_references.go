package api

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *Server) uploadGenerationReference(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	upload, err := openMultipartUpload(c, "file")
	if err != nil {
		return err
	}
	defer upload.File.Close()
	header := upload.Header
	mimeType := upload.MIMEType
	mediaType := mediaTypeFromMIME(mimeType)
	uploadID := uuid.New()
	managed, err := s.storage.UploadManaged(c.Context(), storage.ManagedUploadInput{
		ProjectID: &projectID, OwnerID: &uploadID, Purpose: "references", OriginalName: header.Filename,
		UploadInput: storage.UploadInput{Reader: upload.Reader, Size: header.Size, ContentType: mimeType},
	})
	if err != nil {
		return err
	}
	item, err := s.store.CreateGenerationReferenceUpload(c.Context(), db.CreateGenerationReferenceUploadInput{
		ID: uploadID, ProjectID: projectID, ObjectID: managed.Record.ID, Name: header.Filename, MediaType: mediaType, MimeType: mimeType,
		FileSizeBytes: header.Size,
		Metadata:      db.JSON(map[string]any{"original_filename": header.Filename}),
	})
	if err != nil {
		_ = s.storage.DeleteManaged(c.Context(), managed.Record.ID)
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) getGenerationReferenceFile(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetGenerationReferenceUpload(c.Context(), id)
	if err != nil {
		return err
	}
	return s.sendStoredFile(c, item.ObjectID, item.MimeType, item.Name, item.MediaType == "text")
}

func (s *Server) deleteGenerationReference(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.DeleteGenerationReferenceUpload(c.Context(), id)
	if err != nil {
		return err
	}
	if err := s.storage.DeleteManaged(c.Context(), item.ObjectID); err != nil {
		return fmt.Errorf("delete generation reference object: %w", err)
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) deleteGenerationReferenceObjects(ctx context.Context, uploads []db.GenerationReferenceUpload) {
	for _, upload := range uploads {
		if upload.ObjectID != uuid.Nil {
			if err := s.storage.DeleteManaged(ctx, upload.ObjectID); err != nil {
				s.log.Warn("delete managed generation reference", zap.String("reference_id", upload.ID.String()), zap.Error(err))
			}
		}
	}
}
