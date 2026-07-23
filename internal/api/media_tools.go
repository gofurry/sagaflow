package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/google/uuid"
)

type createMediaJobRequest struct {
	Tool               string          `json:"tool"`
	SourceAssetIDs     []uuid.UUID     `json:"source_asset_ids"`
	TargetAssetGroupID *uuid.UUID      `json:"target_asset_group_id"`
	OutputName         string          `json:"output_name"`
	Parameters         json.RawMessage `json:"parameters"`
}

func (s *Server) mediaToolsStatus(c fiber.Ctx) error {
	if s.mediaTools == nil {
		return fmt.Errorf("%w: media tools are unavailable", service.ErrInvalidInput)
	}
	return writeOK(c, s.mediaTools.Status())
}

func (s *Server) getAssetMediaInfo(c fiber.Ctx) error {
	if s.mediaTools == nil {
		return fmt.Errorf("%w: media tools are unavailable", service.ErrInvalidInput)
	}
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	info, err := s.mediaTools.InspectAsset(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, info)
}

func (s *Server) createMediaJob(c fiber.Ctx) error {
	if s.mediaTools == nil || s.mediaQueue == nil {
		return fmt.Errorf("%w: media tools are unavailable", service.ErrInvalidInput)
	}
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req createMediaJobRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if len(req.Parameters) == 0 {
		req.Parameters = json.RawMessage(`{}`)
	}
	job, err := s.mediaTools.Create(c.Context(), service.CreateMediaJobInput{
		ProjectID: projectID, TargetAssetGroupID: req.TargetAssetGroupID, Tool: strings.TrimSpace(req.Tool),
		SourceAssetIDs: req.SourceAssetIDs, OutputName: strings.TrimSpace(req.OutputName), Parameters: req.Parameters,
	})
	if err != nil {
		return err
	}
	if _, err := s.mediaQueue.Enqueue(c.Context(), job.ID); err != nil {
		return err
	}
	return writeCreated(c, job)
}

func (s *Server) listMediaJobs(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListMediaJobs(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) getMediaJob(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetMediaJob(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) cancelMediaJob(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.RequestMediaJobCancel(c.Context(), id)
	if err != nil {
		return err
	}
	if s.mediaQueue != nil {
		s.mediaQueue.Cancel(id)
	}
	return writeOK(c, item)
}

func (s *Server) clearCompletedMediaJobs(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	count, err := s.store.DeleteCompletedMediaJobs(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": count})
}
