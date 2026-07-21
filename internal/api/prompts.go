package api

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type promptPresetRequest struct {
	ModelID       *uuid.UUID `json:"model_id"`
	ModelPresetID *uuid.UUID `json:"model_preset_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Capability    string     `json:"capability"`
	Content       string     `json:"content"`
}

func (s *Server) listPromptPresets(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListPromptPresets(c.Context(), projectID, c.Query("capability"))
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) createPromptPreset(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req promptPresetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := s.validatePromptPreset(c, projectID, req); err != nil {
		return err
	}
	item, err := s.store.CreatePromptPreset(c.Context(), db.PromptPreset{ProjectID: projectID, ModelID: req.ModelID, ModelPresetID: req.ModelPresetID, Name: req.Name, Description: req.Description, Capability: req.Capability, Content: req.Content})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) updatePromptPreset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	current, err := s.store.GetPromptPreset(c.Context(), id)
	if err != nil {
		return err
	}
	var req promptPresetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := s.validatePromptPreset(c, current.ProjectID, req); err != nil {
		return err
	}
	current.ModelID = req.ModelID
	current.ModelPresetID = req.ModelPresetID
	current.Name = req.Name
	current.Description = req.Description
	current.Capability = req.Capability
	current.Content = req.Content
	item, err := s.store.UpdatePromptPreset(c.Context(), current)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deletePromptPreset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeletePromptPreset(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) validatePromptPreset(c fiber.Ctx, projectID uuid.UUID, req promptPresetRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", service.ErrInvalidInput)
	}
	switch req.Capability {
	case "text", "image", "audio", "video":
	default:
		return fmt.Errorf("%w: invalid capability", service.ErrInvalidInput)
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	if req.ModelID == nil {
		if req.ModelPresetID != nil {
			return fmt.Errorf("%w: a parameter preset requires a default model", service.ErrInvalidInput)
		}
		return nil
	}
	model, err := s.store.GetModel(c.Context(), *req.ModelID)
	if err != nil {
		return err
	}
	if model.Capability != req.Capability {
		return fmt.Errorf("%w: default model capability does not match prompt capability", service.ErrInvalidInput)
	}
	if req.ModelPresetID != nil {
		preset, err := s.store.GetModelPreset(c.Context(), *req.ModelPresetID)
		if err != nil {
			return err
		}
		if preset.ModelID != *req.ModelID {
			return fmt.Errorf("%w: parameter preset must belong to the default model", service.ErrInvalidInput)
		}
	}
	return nil
}
