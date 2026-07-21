package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/comfyui"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type WorkflowService struct {
	store       *db.Store
	credentials *CredentialService
	comfyui     *comfyui.Driver
}

type SaveWorkflowInput struct {
	ID                uuid.UUID
	Code              string
	Name              string
	Description       string
	Capability        string
	InputModalities   []string
	Workflow          json.RawMessage
	ParameterSchema   json.RawMessage
	DefaultParameters json.RawMessage
	Bindings          json.RawMessage
	Outputs           json.RawMessage
	Requirements      json.RawMessage
	Enabled           bool
}

func NewWorkflowService(store *db.Store, credentials *CredentialService, driver *comfyui.Driver) (*WorkflowService, error) {
	if store == nil || credentials == nil || driver == nil {
		return nil, fmt.Errorf("workflow service dependencies are required")
	}
	return &WorkflowService{store: store, credentials: credentials, comfyui: driver}, nil
}

func (s *WorkflowService) Create(ctx context.Context, input SaveWorkflowInput) (db.WorkflowTemplate, error) {
	workflow, err := workflowFromInput(input)
	if err != nil {
		return db.WorkflowTemplate{}, err
	}
	return s.store.CreateWorkflowTemplate(ctx, workflow)
}

func (s *WorkflowService) Update(ctx context.Context, input SaveWorkflowInput) (db.WorkflowTemplate, error) {
	current, err := s.store.GetWorkflowTemplate(ctx, input.ID)
	if err != nil {
		return db.WorkflowTemplate{}, mapStoreError(err)
	}
	workflow, err := workflowFromInput(input)
	if err != nil {
		return db.WorkflowTemplate{}, err
	}
	workflow.ID = current.ID
	return s.store.UpdateWorkflowTemplate(ctx, workflow)
}

func (s *WorkflowService) Analyze(ctx context.Context, workflow json.RawMessage, providerID *uuid.UUID) (comfyui.WorkflowAnalysis, error) {
	var discovery *comfyui.Discovery
	if providerID != nil {
		if *providerID == uuid.Nil {
			return comfyui.WorkflowAnalysis{}, fmt.Errorf("%w: provider_id is invalid", ErrInvalidInput)
		}
		provider, err := s.store.GetModelProvider(ctx, *providerID)
		if err != nil {
			return comfyui.WorkflowAnalysis{}, mapStoreError(err)
		}
		if provider.AdapterCode != ProviderComfyUI || !provider.Enabled {
			return comfyui.WorkflowAnalysis{}, fmt.Errorf("%w: selected connection is not an enabled ComfyUI service", ErrInvalidInput)
		}
		secret, err := s.credentials.SecretForProvider(ctx, provider)
		if err != nil {
			return comfyui.WorkflowAnalysis{}, err
		}
		discovered, err := s.comfyui.Discover(ctx, inference.Runtime{
			ProviderCode: provider.Code, AdapterCode: provider.AdapterCode, Endpoint: secret.BaseURL,
			APIKey: secret.APIKey, Configuration: provider.Metadata,
		})
		if err != nil {
			return comfyui.WorkflowAnalysis{}, providerConnectionError(provider)
		}
		discovery = &discovered
	}
	analysis, err := comfyui.AnalyzeWorkflow(workflow, discovery)
	if err != nil {
		return comfyui.WorkflowAnalysis{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return analysis, nil
}

func (s *WorkflowService) Check(ctx context.Context, workflowID, providerID uuid.UUID) (db.WorkflowCompatibility, error) {
	workflow, err := s.store.GetWorkflowTemplate(ctx, workflowID)
	if err != nil {
		return db.WorkflowCompatibility{}, mapStoreError(err)
	}
	provider, err := s.store.GetModelProvider(ctx, providerID)
	if err != nil {
		return db.WorkflowCompatibility{}, mapStoreError(err)
	}
	if provider.AdapterCode != ProviderComfyUI || !provider.Enabled {
		return db.WorkflowCompatibility{}, fmt.Errorf("%w: selected connection is not an enabled ComfyUI service", ErrInvalidInput)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return db.WorkflowCompatibility{}, err
	}
	spec, err := comfyui.DecodeWorkflowSpec(WorkflowTargetSpec(workflow))
	if err != nil {
		return db.WorkflowCompatibility{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	discovery, err := s.comfyui.Discover(ctx, inference.Runtime{
		ProviderCode: provider.Code, AdapterCode: provider.AdapterCode, Endpoint: secret.BaseURL,
		APIKey: secret.APIKey, Configuration: provider.Metadata,
	})
	if err != nil {
		return db.WorkflowCompatibility{}, providerConnectionError(provider)
	}
	status, report := comfyui.EvaluateCompatibility(spec, discovery)
	return s.store.UpsertWorkflowCompatibility(ctx, db.WorkflowCompatibility{
		WorkflowTemplateID: workflow.ID, ProviderID: provider.ID, Status: status, Report: db.JSON(report),
	})
}

func workflowFromInput(input SaveWorkflowInput) (db.WorkflowTemplate, error) {
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	input.Name = strings.TrimSpace(input.Name)
	if input.Code == "" || input.Name == "" {
		return db.WorkflowTemplate{}, fmt.Errorf("%w: workflow code and name are required", ErrInvalidInput)
	}
	switch input.Capability {
	case "text", "image", "audio", "video":
	default:
		return db.WorkflowTemplate{}, fmt.Errorf("%w: invalid workflow capability", ErrInvalidInput)
	}
	for _, modality := range input.InputModalities {
		switch modality {
		case "image":
		default:
			return db.WorkflowTemplate{}, fmt.Errorf("%w: ComfyUI workflow references currently support image inputs", ErrInvalidInput)
		}
	}
	workflow := db.WorkflowTemplate{
		Code: input.Code, Name: input.Name, Description: strings.TrimSpace(input.Description), Capability: input.Capability,
		InputModalities: input.InputModalities, Workflow: objectJSON(input.Workflow), ParameterSchema: objectJSON(input.ParameterSchema),
		DefaultParameters: objectJSON(input.DefaultParameters), Bindings: objectJSON(input.Bindings),
		Outputs: arrayJSON(input.Outputs), Requirements: objectJSON(input.Requirements), Enabled: input.Enabled, Version: 1,
	}
	if _, err := comfyui.DecodeWorkflowSpec(WorkflowTargetSpec(workflow)); err != nil {
		return db.WorkflowTemplate{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	workflow.Checksum = workflowChecksum(workflow)
	return workflow, nil
}

func WorkflowTargetSpec(workflow db.WorkflowTemplate) json.RawMessage {
	return db.JSON(map[string]any{
		"workflow": json.RawMessage(workflow.Workflow), "bindings": json.RawMessage(workflow.Bindings),
		"outputs": json.RawMessage(workflow.Outputs), "requirements": json.RawMessage(workflow.Requirements),
		"code": workflow.Code, "name": workflow.Name, "capability": workflow.Capability,
		"version": workflow.Version, "checksum": workflow.Checksum,
	})
}

func workflowChecksum(workflow db.WorkflowTemplate) string {
	payload := db.JSON(map[string]any{
		"workflow": json.RawMessage(workflow.Workflow), "bindings": json.RawMessage(workflow.Bindings),
		"outputs": json.RawMessage(workflow.Outputs), "requirements": json.RawMessage(workflow.Requirements),
		"parameter_schema": json.RawMessage(workflow.ParameterSchema), "default_parameters": json.RawMessage(workflow.DefaultParameters),
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func objectJSON(raw json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return json.RawMessage(`{}`)
	}
	return db.JSON(value)
}

func arrayJSON(raw json.RawMessage) json.RawMessage {
	var value []any
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return json.RawMessage(`[]`)
	}
	return db.JSON(value)
}
