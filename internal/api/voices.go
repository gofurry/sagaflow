package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *Server) listVoiceProfiles(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListVoiceProfiles(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) createVoiceProfile(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	modelID, err := parseOptionalUUID(c.FormValue("model_id"))
	if err != nil || modelID == nil {
		return fiber.NewError(fiber.StatusBadRequest, "model_id is required")
	}
	source, err := readVoiceFormFile(c, "file", true)
	if err != nil {
		return err
	}
	prompt, err := readVoiceFormFile(c, "prompt_file", false)
	if err != nil {
		return err
	}
	item, err := s.voices.Create(c.Context(), service.CreateVoiceProfileInput{
		ProjectID: projectID, ModelID: *modelID, Name: c.FormValue("name"), Description: c.FormValue("description"),
		VoiceID: c.FormValue("voice_id"), Source: *source, Prompt: prompt, PromptText: c.FormValue("prompt_text"),
		PreviewText:             c.FormValue("preview_text"),
		NeedNoiseReduction:      parseFormBool(c.FormValue("need_noise_reduction")),
		NeedVolumeNormalization: parseFormBool(c.FormValue("need_volume_normalization")),
	})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

type voiceProfileUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) updateVoiceProfile(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req voiceProfileUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", service.ErrInvalidInput)
	}
	item, err := s.store.UpdateVoiceProfile(c.Context(), id, req.Name, req.Description)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteVoiceProfile(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.voices.Delete(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) getVoiceProfileSource(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetVoiceProfile(c.Context(), id)
	if err != nil {
		return err
	}
	return s.sendStoredFile(c, item.SourceObjectID, item.SourceMimeType, item.SourceName, false)
}

func (s *Server) getVoiceProfilePreview(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetVoiceProfile(c.Context(), id)
	if err != nil {
		return err
	}
	if item.PreviewObjectID == uuid.Nil {
		return service.ErrNotFound
	}
	return s.sendStoredFile(c, item.PreviewObjectID, item.PreviewMimeType, item.Name+" 试听", false)
}

func readVoiceFormFile(c fiber.Ctx, field string, required bool) (*service.VoiceFile, error) {
	header, err := c.FormFile(field)
	if err != nil {
		if !required {
			return nil, nil
		}
		return nil, fiber.NewError(fiber.StatusBadRequest, field+" is required")
	}
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (20<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 20<<20 || int64(len(data)) != header.Size {
		return nil, fiber.NewError(fiber.StatusBadRequest, field+" must not exceed 20 MB")
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	return &service.VoiceFile{Name: header.Filename, MIMEType: mimeType, Data: data}, nil
}

func parseFormBool(value string) bool {
	parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
	return parsed
}

func (s *Server) deleteVoiceProfileObjects(ctx context.Context, profiles []db.VoiceProfile) {
	for _, profile := range profiles {
		for _, objectID := range []uuid.UUID{profile.SourceObjectID, profile.PreviewObjectID} {
			if err := s.storage.DeleteManaged(ctx, objectID); err != nil {
				s.log.Warn("delete managed voice profile object", zap.String("voice_profile_id", profile.ID.String()), zap.Error(err))
			}
		}
		if profile.PromptObjectID != nil {
			if err := s.storage.DeleteManaged(ctx, *profile.PromptObjectID); err != nil {
				s.log.Warn("delete managed voice prompt object", zap.String("voice_profile_id", profile.ID.String()), zap.Error(err))
			}
		}
	}
}
