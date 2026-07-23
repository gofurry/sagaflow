package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
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

type assetGroupRequest struct {
	ParentID    *uuid.UUID `json:"parent_id"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	SortOrder   int32      `json:"sort_order"`
}

func (s *Server) listAssetGroups(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListAssetGroups(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) createAssetGroup(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req assetGroupRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	group := db.AssetGroup{ProjectID: projectID, ParentID: req.ParentID, Kind: req.Kind, Name: req.Name, Description: req.Description, SortOrder: req.SortOrder}
	if err := db.ValidateAssetGroup(group); err != nil {
		return fmt.Errorf("%w: %v", service.ErrInvalidInput, err)
	}
	item, err := s.store.CreateAssetGroup(c.Context(), group)
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) updateAssetGroup(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	group, err := s.store.GetAssetGroup(c.Context(), id)
	if err != nil {
		return err
	}
	var req assetGroupRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	group.ParentID = req.ParentID
	if req.Kind != "" {
		group.Kind = req.Kind
	}
	if strings.TrimSpace(req.Name) != "" {
		group.Name = req.Name
	}
	group.Description = req.Description
	group.SortOrder = req.SortOrder
	if err := db.ValidateAssetGroup(group); err != nil {
		return fmt.Errorf("%w: %v", service.ErrInvalidInput, err)
	}
	item, err := s.store.UpdateAssetGroup(c.Context(), group)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteAssetGroup(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	assets, err := s.store.ListAssetGroupSubtreeAssets(c.Context(), id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteAssetGroup(c.Context(), id); err != nil {
		return err
	}
	s.deleteAssetObjects(c.Context(), assets)
	return writeOK(c, fiber.Map{"deleted": true, "asset_count": len(assets)})
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Server) listAssets(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	groupID, err := parseOptionalUUID(c.Query("group_id"))
	if err != nil {
		return fiber.NewError(400, "invalid group_id")
	}
	episodeID, err := parseOptionalUUID(c.Query("episode_id"))
	if err != nil {
		return fiber.NewError(400, "invalid episode_id")
	}
	items, err := s.store.ListAssets(c.Context(), db.AssetFilter{ProjectID: projectID, GroupID: groupID, EpisodeID: episodeID, Status: c.Query("status"), MediaType: c.Query("media_type")})
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) listGroupAssets(c fiber.Ctx) error {
	groupID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	group, err := s.store.GetAssetGroup(c.Context(), groupID)
	if err != nil {
		return err
	}
	items, err := s.store.ListAssets(c.Context(), db.AssetFilter{ProjectID: group.ProjectID, GroupID: &groupID, Status: c.Query("status"), MediaType: c.Query("media_type")})
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) uploadAsset(c fiber.Ctx) error {
	groupID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	group, err := s.store.GetAssetGroup(c.Context(), groupID)
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), group.ProjectID); err != nil {
		return err
	}
	header, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(400, "file is required")
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
		return fiber.NewError(400, "file is too large")
	}
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		name = header.Filename
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	mediaType := strings.TrimSpace(c.FormValue("media_type"))
	if mediaType == "" {
		mediaType = mediaTypeFromMIME(mimeType)
	}
	episodeID, err := parseOptionalUUID(c.FormValue("episode_id"))
	if err != nil {
		return fiber.NewError(400, "invalid episode_id")
	}
	status := c.FormValue("status", "candidate")
	if status != "candidate" && status != "adopted" {
		status = "candidate"
	}
	assetID := uuid.New()
	managed, err := s.storage.UploadManaged(c.Context(), storage.ManagedUploadInput{ProjectID: &group.ProjectID, Purpose: "assets", OriginalName: header.Filename, UploadInput: storage.UploadInput{Data: data, ContentType: mimeType}})
	if err != nil {
		return err
	}
	objectID := managed.Record.ID
	asset, err := s.store.CreateAsset(c.Context(), db.CreateAssetInput{ID: assetID, ProjectID: group.ProjectID, GroupID: &groupID, EpisodeID: episodeID, ObjectID: objectID, Name: name, MediaType: mediaType, Source: "upload", Status: status, MimeType: mimeType, FileSizeBytes: int64(len(data)), Metadata: db.JSON(map[string]any{"original_filename": header.Filename})})
	if err != nil {
		_ = s.storage.DeleteManaged(c.Context(), managed.Record.ID)
		return err
	}
	return writeCreated(c, asset)
}

func (s *Server) getAssetFile(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	asset, err := s.store.GetAsset(c.Context(), id)
	if err != nil {
		return err
	}
	return s.sendStoredFile(c, asset.ObjectID, asset.MimeType, asset.Name, asset.MediaType == "text")
}

type assetUpdateRequest struct {
	Name string `json:"name"`
}

func (s *Server) updateAsset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req assetUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", service.ErrInvalidInput)
	}
	item, err := s.store.UpdateAssetName(c.Context(), id, req.Name)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) sendStoredFile(c fiber.Ctx, objectID uuid.UUID, mimeType, name string, proxy bool) error {
	object, err := s.storage.Object(c.Context(), objectID)
	if err != nil {
		return err
	}
	download := c.Query("download") == "1"
	data, err := s.storage.Read(c.Context(), object)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, mimeType)
	disposition := "inline"
	if download {
		disposition = "attachment"
	}
	if proxy {
		disposition = "inline"
	}
	c.Set(fiber.HeaderContentDisposition, contentDisposition(disposition, name))
	c.Set("Accept-Ranges", "bytes")
	if rangeHeader := strings.TrimSpace(c.Get("Range")); rangeHeader != "" {
		start, end, ok := parseByteRange(rangeHeader, int64(len(data)))
		if !ok {
			c.Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
		}
		c.Status(fiber.StatusPartialContent)
		c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		c.Set(fiber.HeaderContentLength, strconv.FormatInt(end-start+1, 10))
		return c.Send(data[start : end+1])
	}
	return c.Send(data)
}

func parseByteRange(header string, size int64) (int64, int64, bool) {
	if size <= 0 || !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, 0, false
	}
	value := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	startText, endText, found := strings.Cut(value, "-")
	if !found {
		return 0, 0, false
	}
	if startText == "" {
		suffix, err := strconv.ParseInt(endText, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if endText != "" {
		end, err = strconv.ParseInt(endText, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}

func (s *Server) adoptAsset(c fiber.Ctx) error {
	return s.setAssetStatus(c, "adopted")
}

func (s *Server) discardAsset(c fiber.Ctx) error {
	return s.setAssetStatus(c, "discarded")
}

func (s *Server) setAssetStatus(c fiber.Ctx, status string) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.UpdateAssetStatus(c.Context(), id, status)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteAsset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	exports, err := s.store.ListAssetRemoteExports(c.Context(), id)
	if err != nil {
		return err
	}
	if len(exports) > 0 {
		return fmt.Errorf("%w: 删除本地资产前请先在 S3 副本窗口删除其远端副本", db.ErrConflict)
	}
	asset, err := s.store.DeleteAsset(c.Context(), id)
	if err != nil {
		return err
	}
	s.deleteAssetObjects(c.Context(), []db.Asset{asset})
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) deleteAssetObjects(ctx context.Context, assets []db.Asset) {
	for _, asset := range assets {
		if asset.StagedAssetID != nil {
			continue
		}
		if asset.ObjectID != uuid.Nil {
			if err := s.storage.DeleteManaged(ctx, asset.ObjectID); err != nil {
				s.log.Warn("delete managed asset object", zap.String("asset_id", asset.ID.String()), zap.Error(err))
			}
		}
	}
}

func mediaTypeFromMIME(value string) string {
	switch {
	case strings.HasPrefix(value, "image/"):
		return "image"
	case strings.HasPrefix(value, "audio/"):
		return "audio"
	case strings.HasPrefix(value, "video/"):
		return "video"
	case strings.HasPrefix(value, "text/"):
		return "text"
	default:
		return "file"
	}
}

func safeFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "" {
		return strconv.FormatInt(time.Now().Unix(), 10)
	}
	return strings.ReplaceAll(value, "\"", "")
}

func contentDisposition(disposition, name string) string {
	name = safeFilename(name)
	ascii := "download" + filepath.Ext(name)
	escaped := strings.ReplaceAll(url.PathEscape(name), "+", "%20")
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, ascii, escaped)
}
