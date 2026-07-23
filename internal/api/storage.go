package api

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/google/uuid"
)

type saveStorageRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint"`
	PublicEndpoint  string `json:"public_endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Prefix          string `json:"prefix"`
	ForcePathStyle  bool   `json:"force_path_style"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Enabled         bool   `json:"enabled"`
	IsDefault       bool   `json:"is_default"`
}

func (s *Server) listStorageBackends(c fiber.Ctx) error {
	items, err := s.storageService.List(c.Context())
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func storageInput(req saveStorageRequest) service.SaveStorageInput {
	return service.SaveStorageInput{
		Name: req.Name, Provider: req.Provider, Endpoint: req.Endpoint, PublicEndpoint: req.PublicEndpoint,
		Region: req.Region, Bucket: req.Bucket, Prefix: req.Prefix, ForcePathStyle: req.ForcePathStyle,
		AccessKeyID: req.AccessKeyID, SecretAccessKey: req.SecretAccessKey, Enabled: req.Enabled, IsDefault: req.IsDefault,
	}
}

func (s *Server) createStorageBackend(c fiber.Ctx) error {
	var req saveStorageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	item, err := s.storageService.Save(c.Context(), storageInput(req))
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) updateStorageBackend(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req saveStorageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	input := storageInput(req)
	input.ID = id
	item, err := s.storageService.Save(c.Context(), input)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteStorageBackend(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.storageService.Delete(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) testStorageBackend(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.storageService.Test(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"available": true})
}

func (s *Server) listAssetExports(c fiber.Ctx) error {
	assetID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	items, err := s.store.ListAssetRemoteExports(c.Context(), assetID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) listProjectAssetExports(c fiber.Ctx) error {
	projectID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetProject(c.Context(), projectID); err != nil {
		return err
	}
	items, err := s.store.ListProjectAssetRemoteExports(c.Context(), projectID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) publishAsset(c fiber.Ctx) error {
	assetID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		ConnectionID uuid.UUID `json:"connection_id"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.ConnectionID == uuid.Nil {
		return fiber.NewError(400, "valid connection_id is required")
	}
	item, err := s.storageService.PublishAsset(c.Context(), assetID, req.ConnectionID)
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) deleteAssetExport(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.storageService.DeleteExport(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) getAssetExportURL(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	signed, err := s.storageService.ProviderURL(c.Context(), id, 24*time.Hour)
	if err != nil {
		return err
	}
	return writeOK(c, signed)
}
