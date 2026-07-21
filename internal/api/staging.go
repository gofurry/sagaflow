package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *Server) listStagedAssets(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "24"))
	createdFrom, err := parseOptionalTime(c.Query("created_from"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid created_from")
	}
	createdTo, err := parseOptionalTime(c.Query("created_to"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid created_to")
	}
	items, err := s.store.ListStagedAssets(c.Context(), db.StagedAssetFilter{
		ProjectID: projectID, Source: c.Query("source"), Processed: c.Query("processed"),
		Capability: c.Query("capability"), MediaType: c.Query("media_type"), Name: c.Query("name"),
		CreatedFrom: createdFrom, CreatedTo: createdTo, Sort: c.Query("sort"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) stagedAssetSummary(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	summary, err := s.store.StagedAssetSummary(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, summary)
}

func (s *Server) uploadStagedAsset(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	header, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 512<<20))
	if err != nil {
		return err
	}
	if int64(len(data)) != header.Size {
		return fiber.NewError(fiber.StatusBadRequest, "file is too large")
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	mediaType := strings.TrimSpace(c.FormValue("media_type"))
	if mediaType == "" {
		mediaType = mediaTypeFromMIME(mimeType)
	}
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		name = header.Filename
	}
	id := uuid.New()
	managed, err := s.storage.UploadManaged(c.Context(), storage.ManagedUploadInput{ProjectID: &projectID, Purpose: "staging", OriginalName: header.Filename, UploadInput: storage.UploadInput{Data: data, ContentType: mimeType}})
	if err != nil {
		return err
	}
	contentText := ""
	if mediaType == "text" {
		contentText = string(data)
	}
	item, err := s.store.CreateStagedAsset(c.Context(), db.CreateStagedAssetInput{
		ID: id, ProjectID: projectID, ObjectID: managed.Record.ID, Source: "upload", Name: name, MediaType: mediaType, MimeType: mimeType,
		FileSizeBytes: int64(len(data)), ContentText: contentText,
		Metadata: db.JSON(map[string]any{"original_filename": header.Filename}),
	})
	if err != nil {
		_ = s.storage.DeleteManaged(c.Context(), managed.Record.ID)
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) getStagedAssetFile(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetStagedAsset(c.Context(), id)
	if err != nil {
		return err
	}
	return s.sendStoredFile(c, item.ObjectID, item.MimeType, item.Name, item.MediaType == "text")
}

type stagedAssetUpdateRequest struct {
	Name string `json:"name"`
}

func (s *Server) updateStagedAsset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req stagedAssetUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", service.ErrInvalidInput)
	}
	item, err := s.store.UpdateStagedAssetName(c.Context(), id, req.Name)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

type importStagedAssetRequest struct {
	GroupID uuid.UUID `json:"group_id"`
	Name    string    `json:"name"`
}

func (s *Server) importStagedAsset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req importStagedAssetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if req.GroupID == uuid.Nil {
		return fmt.Errorf("%w: group_id is required", service.ErrInvalidInput)
	}
	asset, err := s.store.ImportStagedAsset(c.Context(), id, req.GroupID, req.Name)
	if err != nil {
		return err
	}
	return writeCreated(c, asset)
}

func (s *Server) deleteStagedAsset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.DeleteStagedAsset(c.Context(), id)
	if err != nil {
		return err
	}
	s.deleteStagedAssetObjects(c.Context(), []db.StagedAsset{item})
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) deleteStagedAssetObjects(ctx context.Context, items []db.StagedAsset) {
	for _, item := range items {
		if item.ObjectID != uuid.Nil {
			if err := s.storage.DeleteManaged(ctx, item.ObjectID); err != nil {
				s.log.Warn("delete managed staged object", zap.String("staged_asset_id", item.ID.String()), zap.Error(err))
			}
		}
	}
}

func parseOptionalTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
