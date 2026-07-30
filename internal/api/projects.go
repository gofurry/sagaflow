package api

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/platform/filemanager"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

func idParam(c fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "invalid "+name)
	}
	return id, nil
}

type projectRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AspectRatio string   `json:"aspect_ratio"`
	Resolution  string   `json:"resolution"`
	FrameRate   *float64 `json:"frame_rate"`
}

func (s *Server) listProjects(c fiber.Ctx) error {
	items, err := s.store.ListProjects(c.Context())
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) getProject(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetProject(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) revealProjectDirectory(c fiber.Ctx) error {
	if !s.cfg.IsLoopback() {
		return fmt.Errorf("%w: 只有本机工作台可以打开项目文件夹", db.ErrConflict)
	}
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), id); err != nil {
		return err
	}
	if err := s.storage.RefreshProjectManifest(c.Context(), id); err != nil {
		return err
	}
	directory, err := s.storage.ProjectDirectory(id)
	if err != nil {
		return err
	}
	if err := filemanager.OpenDirectory(directory); err != nil {
		return fmt.Errorf("打开项目文件夹: %w", err)
	}
	return writeOK(c, fiber.Map{"opened": true})
}
func (s *Server) createProject(c fiber.Ctx) error {
	var req projectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("%w: title is required", service.ErrInvalidInput)
	}
	if err := validateProjectRequest(req); err != nil {
		return err
	}
	item, err := s.store.CreateProject(c.Context(), req.Title, req.Description, req.AspectRatio, req.Resolution, req.FrameRate)
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) updateProject(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req projectRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("%w: title is required", service.ErrInvalidInput)
	}
	if err := validateProjectRequest(req); err != nil {
		return err
	}
	item, err := s.store.UpdateProject(c.Context(), id, req.Title, req.Description, req.AspectRatio, req.Resolution, req.FrameRate)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func validateProjectRequest(req projectRequest) error {
	if len([]rune(strings.TrimSpace(req.AspectRatio))) > 32 {
		return fmt.Errorf("%w: aspect ratio is too long", service.ErrInvalidInput)
	}
	if len([]rune(strings.TrimSpace(req.Resolution))) > 64 {
		return fmt.Errorf("%w: resolution is too long", service.ErrInvalidInput)
	}
	if req.FrameRate != nil && (*req.FrameRate < 1 || *req.FrameRate > 240) {
		return fmt.Errorf("%w: frame rate must be between 1 and 240", service.ErrInvalidInput)
	}
	return nil
}
func (s *Server) deleteProject(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	assets, err := s.store.ListAssets(c.Context(), db.AssetFilter{ProjectID: id})
	if err != nil {
		return err
	}
	for _, asset := range assets {
		exports, err := s.store.ListAssetRemoteExports(c.Context(), asset.ID)
		if err != nil {
			return err
		}
		if len(exports) > 0 {
			return fmt.Errorf("%w: 项目仍有 S3 远端副本，请先从资产页删除这些副本", db.ErrConflict)
		}
	}
	stagedAssets, err := s.store.ListProjectStagedAssets(c.Context(), id)
	if err != nil {
		return err
	}
	references, err := s.store.ListProjectGenerationReferenceUploads(c.Context(), id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProject(c.Context(), id); err != nil {
		return err
	}
	s.deleteAssetObjects(c.Context(), assets)
	s.deleteStagedAssetObjects(c.Context(), stagedAssets)
	s.deleteGenerationReferenceObjects(c.Context(), references)
	return writeOK(c, fiber.Map{"deleted": true})
}

type episodeRequest struct {
	EpisodeNumber         int32  `json:"episode_number"`
	Title                 string `json:"title"`
	Notes                 string `json:"notes"`
	TargetDurationSeconds *int32 `json:"target_duration_seconds"`
	TargetShotCount       *int32 `json:"target_shot_count"`
	ScriptBody            string `json:"script_body"`
}

func (s *Server) listEpisodes(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListEpisodes(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) getEpisode(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetEpisode(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
func (s *Server) createEpisode(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req episodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	input := db.CreateEpisodeInput{ProjectID: projectID, EpisodeNumber: req.EpisodeNumber, Title: req.Title, Notes: req.Notes, TargetDurationSeconds: req.TargetDurationSeconds, TargetShotCount: req.TargetShotCount, ScriptBody: req.ScriptBody}
	if err := db.ValidateEpisode(input); err != nil {
		return fmt.Errorf("%w: %v", service.ErrInvalidInput, err)
	}
	item, err := s.store.CreateEpisode(c.Context(), input)
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) updateEpisode(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	existing, err := s.store.GetEpisode(c.Context(), id)
	if err != nil {
		return err
	}
	var req episodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if req.EpisodeNumber > 0 {
		existing.EpisodeNumber = req.EpisodeNumber
	}
	if strings.TrimSpace(req.Title) != "" {
		existing.Title = req.Title
	}
	existing.Notes = req.Notes
	existing.TargetDurationSeconds = req.TargetDurationSeconds
	existing.TargetShotCount = req.TargetShotCount
	item, err := s.store.UpdateEpisode(c.Context(), existing)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
func (s *Server) deleteEpisode(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeleteEpisode(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

type scriptRequest struct {
	Body  string `json:"body"`
	Note  string `json:"note"`
	Adopt bool   `json:"adopt"`
}

func (s *Server) listEpisodeScripts(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListEpisodeScripts(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) createEpisodeScript(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req scriptRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	item, err := s.store.CreateEpisodeScript(c.Context(), id, req.Body, req.Note, req.Adopt)
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) adoptEpisodeScript(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.AdoptEpisodeScript(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
