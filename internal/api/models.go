package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type providerRequest struct {
	Code         string          `json:"code"`
	AdapterCode  string          `json:"adapter_code"`
	DisplayName  string          `json:"display_name"`
	BaseURL      string          `json:"base_url"`
	AuthType     string          `json:"auth_type"`
	Capabilities []string        `json:"capabilities"`
	Enabled      *bool           `json:"enabled"`
	Metadata     json.RawMessage `json:"metadata"`
}

func (s *Server) listModelProviders(c fiber.Ctx) error {
	items, err := s.store.ListModelProviders(c.Context())
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) createModelProvider(c fiber.Ctx) error {
	var req providerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := validateProviderRequest(req); err != nil {
		return err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := s.store.CreateModelProvider(c.Context(), db.ModelProvider{Code: req.Code, AdapterCode: req.AdapterCode, DisplayName: req.DisplayName, BaseURL: req.BaseURL, AuthType: req.AuthType, Capabilities: req.Capabilities, Enabled: enabled, Metadata: req.Metadata})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) updateModelProvider(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.store.GetModelProvider(c.Context(), id)
	if err != nil {
		return err
	}
	var req providerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if req.Code != "" {
		item.Code = req.Code
	}
	if req.AdapterCode != "" {
		item.AdapterCode = req.AdapterCode
	}
	if req.DisplayName != "" {
		item.DisplayName = req.DisplayName
	}
	item.BaseURL = req.BaseURL
	if req.AuthType != "" {
		item.AuthType = req.AuthType
	}
	if req.Capabilities != nil {
		item.Capabilities = req.Capabilities
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if len(req.Metadata) > 0 {
		item.Metadata = req.Metadata
	}
	if err := validateProviderRequest(providerRequest{Code: item.Code, AdapterCode: item.AdapterCode, DisplayName: item.DisplayName, BaseURL: item.BaseURL, AuthType: item.AuthType}); err != nil {
		return err
	}
	item, err = s.store.UpdateModelProvider(c.Context(), item)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteModelProvider(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeleteModelProvider(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func validateProviderRequest(req providerRequest) error {
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.AdapterCode) == "" || strings.TrimSpace(req.DisplayName) == "" {
		return fmt.Errorf("%w: code, adapter_code and display_name are required", service.ErrInvalidInput)
	}
	if req.AuthType != "none" && req.AuthType != "api_key" && req.AuthType != "bearer" {
		return fmt.Errorf("%w: auth_type must be none, api_key or bearer", service.ErrInvalidInput)
	}
	parsed, err := url.Parse(strings.TrimSpace(req.BaseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: base_url must be an absolute http(s) URL", service.ErrInvalidInput)
	}
	return nil
}

func (s *Server) testModelProvider(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	server, err := s.modelConnections.Test(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, server)
}

func (s *Server) discoverProviderModels(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	discovery, err := s.modelConnections.Discover(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, discovery)
}

type syncProviderModelsRequest struct {
	ModelIDs []string `json:"model_ids"`
}

func (s *Server) syncProviderModels(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req syncProviderModelsRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	result, err := s.modelConnections.Sync(c.Context(), id, req.ModelIDs)
	if err != nil {
		return err
	}
	return writeOK(c, result)
}

type modelRequest struct {
	ProviderID        uuid.UUID       `json:"provider_id"`
	ModelID           string          `json:"model_id"`
	DisplayName       string          `json:"display_name"`
	Capability        string          `json:"capability"`
	InputModalities   []string        `json:"input_modalities"`
	Features          []string        `json:"features"`
	ParameterSchema   json.RawMessage `json:"parameter_schema"`
	DefaultParameters json.RawMessage `json:"default_parameters"`
	Enabled           *bool           `json:"enabled"`
	Available         *bool           `json:"available"`
	Metadata          json.RawMessage `json:"metadata"`
}

func (s *Server) listModels(c fiber.Ctx) error {
	items, err := s.store.ListModels(c.Context(), c.Query("capability"))
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) createModel(c fiber.Ctx) error {
	var req modelRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := validateModelRequest(req); err != nil {
		return err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	available := true
	if req.Available != nil {
		available = *req.Available
	}
	modalities := req.InputModalities
	if len(modalities) == 0 {
		modalities = []string{"text"}
	}
	item, err := s.store.CreateModel(c.Context(), db.Model{ProviderID: req.ProviderID, ModelID: req.ModelID, DisplayName: req.DisplayName, Capability: req.Capability, InputModalities: modalities, Features: req.Features, ParameterSchema: req.ParameterSchema, DefaultParameters: req.DefaultParameters, Enabled: enabled, Available: available, Metadata: req.Metadata})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) updateModel(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	model, err := s.store.GetModel(c.Context(), id)
	if err != nil {
		return err
	}
	var req modelRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if req.ProviderID != uuid.Nil {
		model.ProviderID = req.ProviderID
	}
	if req.ModelID != "" {
		model.ModelID = req.ModelID
	}
	if req.DisplayName != "" {
		model.DisplayName = req.DisplayName
	}
	if req.Capability != "" {
		model.Capability = req.Capability
	}
	if req.InputModalities != nil {
		model.InputModalities = req.InputModalities
	}
	if req.Features != nil {
		model.Features = req.Features
	}
	if len(req.ParameterSchema) > 0 {
		model.ParameterSchema = req.ParameterSchema
	}
	if len(req.DefaultParameters) > 0 {
		model.DefaultParameters = req.DefaultParameters
	}
	if req.Enabled != nil {
		model.Enabled = *req.Enabled
	}
	if req.Available != nil {
		model.Available = *req.Available
	}
	if len(req.Metadata) > 0 {
		model.Metadata = req.Metadata
	}
	item, err := s.store.UpdateModel(c.Context(), model)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
func validateModelRequest(req modelRequest) error {
	if req.ProviderID == uuid.Nil || strings.TrimSpace(req.ModelID) == "" || strings.TrimSpace(req.DisplayName) == "" {
		return fmt.Errorf("%w: provider_id, model_id and display_name are required", service.ErrInvalidInput)
	}
	switch req.Capability {
	case "text", "image", "audio", "video", "multimodal":
		return nil
	}
	return fmt.Errorf("%w: invalid capability", service.ErrInvalidInput)
}

type presetRequest struct {
	ModelID    uuid.UUID       `json:"model_id"`
	Name       string          `json:"name"`
	Parameters json.RawMessage `json:"parameters"`
	IsDefault  bool            `json:"is_default"`
}

func (s *Server) listModelPresets(c fiber.Ctx) error {
	modelID, err := parseOptionalUUID(c.Query("model_id"))
	if err != nil {
		return fiber.NewError(400, "invalid model_id")
	}
	items, err := s.store.ListModelPresets(c.Context(), modelID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) createModelPreset(c fiber.Ctx) error {
	var req presetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := validatePresetRequest(req, true); err != nil {
		return err
	}
	item, err := s.store.CreateModelPreset(c.Context(), db.ModelPreset{ModelID: req.ModelID, Name: req.Name, Parameters: req.Parameters, IsDefault: req.IsDefault})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) updateModelPreset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	current, err := s.store.GetModelPreset(c.Context(), id)
	if err != nil {
		return err
	}
	var req presetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if err := validatePresetRequest(req, false); err != nil {
		return err
	}
	current.Name = req.Name
	current.Parameters = req.Parameters
	current.IsDefault = req.IsDefault
	item, err := s.store.UpdateModelPreset(c.Context(), current)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteModelPreset(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeleteModelPreset(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func validatePresetRequest(req presetRequest, requireModel bool) error {
	if requireModel && req.ModelID == uuid.Nil {
		return fmt.Errorf("%w: model_id is required", service.ErrInvalidInput)
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("%w: name is required", service.ErrInvalidInput)
	}
	var parameters map[string]any
	if len(req.Parameters) > 0 {
		if err := json.Unmarshal(req.Parameters, &parameters); err != nil || parameters == nil {
			return fmt.Errorf("%w: parameters must be a JSON object", service.ErrInvalidInput)
		}
	}
	return nil
}

type credentialRequest struct {
	ProviderID uuid.UUID `json:"provider_id"`
	Name       string    `json:"name"`
	APIKey     string    `json:"api_key"`
	Activate   bool      `json:"activate"`
}

func (s *Server) listProviderCredentials(c fiber.Ctx) error {
	providerID, err := parseOptionalUUID(c.Query("provider_id"))
	if err != nil {
		return fiber.NewError(400, "invalid provider_id")
	}
	items, err := s.credentials.List(c.Context(), providerID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}
func (s *Server) createProviderCredential(c fiber.Ctx) error {
	var req credentialRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	item, err := s.credentials.Create(c.Context(), service.SaveCredentialInput{ProviderID: req.ProviderID, Name: req.Name, APIKey: req.APIKey, Activate: req.Activate})
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}
func (s *Server) updateProviderCredential(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req credentialRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	item, err := s.credentials.Update(c.Context(), service.SaveCredentialInput{ID: id, ProviderID: req.ProviderID, Name: req.Name, APIKey: req.APIKey, Activate: req.Activate})
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
func (s *Server) activateProviderCredential(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	item, err := s.credentials.Activate(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}
func (s *Server) testProviderCredential(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	secret, err := s.credentials.SecretByID(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"available": strings.TrimSpace(secret.APIKey) != "", "provider": secret.ProviderCode, "source": secret.Source, "message": "credential decrypted successfully"})
}
func (s *Server) deleteProviderCredential(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.credentials.Delete(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}
