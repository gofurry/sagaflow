package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/google/uuid"
)

func (s *Server) listVoiceCapabilities(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	items, err := s.voices.Capabilities(c.Context())
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) listVoiceProfiles(c fiber.Ctx) error {
	items, err := s.store.ListVoiceProfiles(c.Context())
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) createVoiceProfile(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	kind := strings.ToLower(strings.TrimSpace(c.FormValue("kind")))
	var source *service.VoiceFile
	var err error
	if kind == "clone" {
		source, err = readVoiceFormFile(c, "file", true)
		if err != nil {
			return err
		}
	}
	item, err := s.voices.CreateProfile(c.Context(), service.CreateVoiceProfileInput{
		Name: c.FormValue("name"), Description: c.FormValue("description"), Kind: kind,
		Source: source, ReferenceText: c.FormValue("reference_text"), DesignPrompt: c.FormValue("design_prompt"),
	})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

type voiceBindingCreateRequest struct {
	ModelID                 uuid.UUID `json:"model_id"`
	VoiceID                 string    `json:"voice_id"`
	PreviewText             string    `json:"preview_text"`
	SourceURL               string    `json:"source_url"`
	NeedNoiseReduction      bool      `json:"need_noise_reduction"`
	NeedVolumeNormalization bool      `json:"need_volume_normalization"`
}

func (s *Server) createVoiceBinding(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	profileID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req voiceBindingCreateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	item, err := s.voices.CreateBinding(c.Context(), service.CreateVoiceBindingInput{
		ProfileID: profileID, ModelID: req.ModelID, VoiceID: req.VoiceID,
		PreviewText: req.PreviewText, SourceURL: req.SourceURL,
		NeedNoiseReduction: req.NeedNoiseReduction, NeedVolumeNormalization: req.NeedVolumeNormalization,
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
	if err := s.voices.DeleteProfile(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) deleteVoiceBinding(c fiber.Ctx) error {
	if s.voices == nil {
		return fmt.Errorf("%w: voice service is unavailable", service.ErrInvalidInput)
	}
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.voices.DeleteBinding(c.Context(), id); err != nil {
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
	if item.SourceObjectID == nil {
		return service.ErrNotFound
	}
	return s.sendStoredFile(c, *item.SourceObjectID, item.SourceMimeType, item.SourceName, false)
}

func (s *Server) getVoiceBindingPreview(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetVoiceBinding(c.Context(), id)
	if err != nil {
		return err
	}
	if item.PreviewObjectID == nil {
		return service.ErrNotFound
	}
	return s.sendStoredFile(c, *item.PreviewObjectID, item.PreviewMimeType, item.VoiceID+" 试听", false)
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
