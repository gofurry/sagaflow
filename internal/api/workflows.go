package api

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/google/uuid"
)

type workflowRequest struct {
	Code              string          `json:"code"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Capability        string          `json:"capability"`
	InputModalities   []string        `json:"input_modalities"`
	Workflow          json.RawMessage `json:"workflow"`
	ParameterSchema   json.RawMessage `json:"parameter_schema"`
	DefaultParameters json.RawMessage `json:"default_parameters"`
	Bindings          json.RawMessage `json:"bindings"`
	Outputs           json.RawMessage `json:"outputs"`
	Requirements      json.RawMessage `json:"requirements"`
	Enabled           *bool           `json:"enabled"`
}

type workflowAnalysisRequest struct {
	Workflow   json.RawMessage `json:"workflow"`
	ProviderID *uuid.UUID      `json:"provider_id"`
}

func (s *Server) listWorkflowTemplates(c fiber.Ctx) error {
	items, err := s.store.ListWorkflowTemplates(c.Context(), c.Query("capability"))
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) createWorkflowTemplate(c fiber.Ctx) error {
	var req workflowRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	item, err := s.workflows.Create(c.Context(), workflowInput(req, uuid.Nil, true))
	if err != nil {
		return err
	}
	return writeCreated(c, item)
}

func (s *Server) analyzeWorkflowTemplate(c fiber.Ctx) error {
	var req workflowAnalysisRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if len(req.Workflow) == 0 {
		return fmt.Errorf("%w: workflow is required", service.ErrInvalidInput)
	}
	analysis, err := s.workflows.Analyze(c.Context(), req.Workflow, req.ProviderID)
	if err != nil {
		return err
	}
	return writeOK(c, analysis)
}

func (s *Server) updateWorkflowTemplate(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	current, err := s.store.GetWorkflowTemplate(c.Context(), id)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &fields); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	var req workflowRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if len(fields) == 1 && req.Enabled != nil {
		item, updateErr := s.store.SetWorkflowTemplateEnabled(c.Context(), id, *req.Enabled)
		if updateErr != nil {
			return updateErr
		}
		return writeOK(c, item)
	}
	if _, exists := fields["code"]; !exists {
		req.Code = current.Code
	}
	if _, exists := fields["name"]; !exists {
		req.Name = current.Name
	}
	if _, exists := fields["description"]; !exists {
		req.Description = current.Description
	}
	if _, exists := fields["capability"]; !exists {
		req.Capability = current.Capability
	}
	if _, exists := fields["input_modalities"]; !exists {
		req.InputModalities = current.InputModalities
	}
	if _, exists := fields["workflow"]; !exists {
		req.Workflow = current.Workflow
	}
	if _, exists := fields["parameter_schema"]; !exists {
		req.ParameterSchema = current.ParameterSchema
	}
	if _, exists := fields["default_parameters"]; !exists {
		req.DefaultParameters = current.DefaultParameters
	}
	if _, exists := fields["bindings"]; !exists {
		req.Bindings = current.Bindings
	}
	if _, exists := fields["outputs"]; !exists {
		req.Outputs = current.Outputs
	}
	if _, exists := fields["requirements"]; !exists {
		req.Requirements = current.Requirements
	}
	item, err := s.workflows.Update(c.Context(), workflowInput(req, id, current.Enabled))
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func (s *Server) deleteWorkflowTemplate(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if err := s.store.DeleteWorkflowTemplate(c.Context(), id); err != nil {
		return err
	}
	return writeOK(c, fiber.Map{"deleted": true})
}

func (s *Server) listWorkflowCompatibilities(c fiber.Ctx) error {
	workflowID, err := parseOptionalUUID(c.Query("workflow_template_id"))
	if err != nil {
		return fiber.NewError(400, "invalid workflow_template_id")
	}
	providerID, err := parseOptionalUUID(c.Query("provider_id"))
	if err != nil {
		return fiber.NewError(400, "invalid provider_id")
	}
	items, err := s.store.ListWorkflowCompatibilities(c.Context(), workflowID, providerID)
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func (s *Server) checkWorkflowCompatibility(c fiber.Ctx) error {
	workflowID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		ProviderID uuid.UUID `json:"provider_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if req.ProviderID == uuid.Nil {
		return fmt.Errorf("%w: provider_id is required", service.ErrInvalidInput)
	}
	item, err := s.workflows.Check(c.Context(), workflowID, req.ProviderID)
	if err != nil {
		return err
	}
	return writeOK(c, item)
}

func workflowInput(req workflowRequest, id uuid.UUID, defaultEnabled bool) service.SaveWorkflowInput {
	enabled := defaultEnabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return service.SaveWorkflowInput{
		ID: id, Code: req.Code, Name: req.Name, Description: req.Description, Capability: req.Capability,
		InputModalities: req.InputModalities, Workflow: req.Workflow, ParameterSchema: req.ParameterSchema,
		DefaultParameters: req.DefaultParameters, Bindings: req.Bindings, Outputs: req.Outputs,
		Requirements: req.Requirements, Enabled: enabled,
	}
}
