package api

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type generationRequest struct {
	ProjectID          uuid.UUID                         `json:"project_id"`
	EpisodeID          *uuid.UUID                        `json:"episode_id"`
	CanvasNodeID       *uuid.UUID                        `json:"canvas_node_id"`
	TargetAssetGroupID *uuid.UUID                        `json:"target_asset_group_id"`
	PromptPresetID     *uuid.UUID                        `json:"prompt_preset_id"`
	ModelPresetID      *uuid.UUID                        `json:"model_preset_id"`
	TargetKind         string                            `json:"target_kind"`
	ProviderID         *uuid.UUID                        `json:"provider_id"`
	ModelID            *uuid.UUID                        `json:"model_id"`
	WorkflowTemplateID *uuid.UUID                        `json:"workflow_template_id"`
	OutputName         string                            `json:"output_name"`
	Prompt             string                            `json:"prompt"`
	Parameters         json.RawMessage                   `json:"parameters"`
	InputReferences    []generationInputReferenceRequest `json:"input_references"`
	ImageTask          *generationImageTaskRequest       `json:"image_task"`
}

type generationImageTaskRequest struct {
	Type         string  `json:"type"`
	SourceWidth  int     `json:"source_width"`
	SourceHeight int     `json:"source_height"`
	TargetWidth  int     `json:"target_width,omitempty"`
	TargetHeight int     `json:"target_height,omitempty"`
	SourceX      int     `json:"source_x,omitempty"`
	SourceY      int     `json:"source_y,omitempty"`
	TopScale     float64 `json:"top_scale,omitempty"`
	BottomScale  float64 `json:"bottom_scale,omitempty"`
	LeftScale    float64 `json:"left_scale,omitempty"`
	RightScale   float64 `json:"right_scale,omitempty"`
}

type generationInputReferenceRequest struct {
	Source         string     `json:"source"`
	ID             uuid.UUID  `json:"id"`
	RemoteExportID *uuid.UUID `json:"remote_export_id"`
}

type resolvedGenerationTarget struct {
	Kind              string
	ProviderID        *uuid.UUID
	ModelID           *uuid.UUID
	WorkflowID        *uuid.UUID
	Capability        string
	InputModalities   []string
	Features          []string
	ProviderCode      string
	AdapterCode       string
	Identifier        string
	DefaultParameters json.RawMessage
	Snapshot          json.RawMessage
}

func (s *Server) createGenerationJob(c fiber.Ctx) error {
	var req generationRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	if req.ProjectID == uuid.Nil {
		return fmt.Errorf("%w: project_id is required", service.ErrInvalidInput)
	}
	if _, err := s.store.GetProject(c.Context(), req.ProjectID); err != nil {
		return err
	}
	target, err := s.resolveGenerationTarget(c, req)
	if err != nil {
		return err
	}
	if err := validateImageTask(target, req.ImageTask, req.InputReferences); err != nil {
		return err
	}
	if target.Capability == "video" {
		if req.EpisodeID == nil || req.CanvasNodeID == nil {
			return fmt.Errorf("%w: video generation requires an episode storyboard shot", service.ErrInvalidInput)
		}
		episode, episodeErr := s.store.GetEpisode(c.Context(), *req.EpisodeID)
		if episodeErr != nil {
			return episodeErr
		}
		node, nodeErr := s.store.GetCanvasNode(c.Context(), *req.CanvasNodeID)
		if nodeErr != nil {
			return nodeErr
		}
		if episode.ProjectID != req.ProjectID || node.EpisodeID != episode.ID || node.NodeType != "video" {
			return fmt.Errorf("%w: storyboard shot must belong to the selected project and episode", service.ErrInvalidInput)
		}
		referenceIDs, referenceErr := s.store.ListCanvasReferenceAssetIDs(c.Context(), node.ID)
		if referenceErr != nil {
			return referenceErr
		}
		canvasReferences := make(map[uuid.UUID]struct{}, len(referenceIDs))
		for _, referenceID := range referenceIDs {
			canvasReferences[referenceID] = struct{}{}
		}
		requestedExports := make(map[uuid.UUID]*uuid.UUID, len(req.InputReferences))
		for _, reference := range req.InputReferences {
			if reference.Source != "asset" {
				return fmt.Errorf("%w: storyboard references must use assets", service.ErrInvalidInput)
			}
			if _, connected := canvasReferences[reference.ID]; !connected {
				return fmt.Errorf("%w: storyboard reference is not connected to this shot", service.ErrInvalidInput)
			}
			requestedExports[reference.ID] = reference.RemoteExportID
		}
		req.InputReferences = make([]generationInputReferenceRequest, 0, len(referenceIDs))
		for _, referenceID := range referenceIDs {
			req.InputReferences = append(req.InputReferences, generationInputReferenceRequest{Source: "asset", ID: referenceID, RemoteExportID: requestedExports[referenceID]})
		}
		req.TargetAssetGroupID = nil
	} else if req.CanvasNodeID != nil {
		return fmt.Errorf("%w: only video generation can be bound to a storyboard shot", service.ErrInvalidInput)
	}
	if err := validateVideoReferences(target, req.InputReferences); err != nil {
		return err
	}
	if len(req.Parameters) > 0 {
		var parameterObject map[string]any
		if err := json.Unmarshal(req.Parameters, &parameterObject); err != nil || parameterObject == nil {
			return fmt.Errorf("%w: parameters must be a JSON object", service.ErrInvalidInput)
		}
	}
	baseParameters := target.DefaultParameters
	if req.ModelPresetID != nil {
		if target.Kind != "model" || target.ModelID == nil {
			return fmt.Errorf("%w: workflow targets do not use model parameter presets", service.ErrInvalidInput)
		}
		preset, presetErr := s.store.GetModelPreset(c.Context(), *req.ModelPresetID)
		if presetErr != nil {
			return presetErr
		}
		if preset.ModelID != *target.ModelID {
			return fmt.Errorf("%w: parameter preset must belong to the selected model", service.ErrInvalidInput)
		}
		baseParameters = mergeJSON(baseParameters, preset.Parameters)
	}
	parameters := mergeJSON(baseParameters, req.Parameters)
	if req.TargetAssetGroupID != nil {
		targetGroup, groupErr := s.store.GetAssetGroup(c.Context(), *req.TargetAssetGroupID)
		if groupErr != nil {
			return groupErr
		}
		if targetGroup.ProjectID != req.ProjectID {
			return fmt.Errorf("%w: target asset group must belong to the selected project", service.ErrInvalidInput)
		}
	}
	if req.PromptPresetID != nil {
		preset, presetErr := s.store.GetPromptPreset(c.Context(), *req.PromptPresetID)
		if presetErr != nil {
			return presetErr
		}
		if preset.Capability != target.Capability {
			return fmt.Errorf("%w: prompt preset must match the model capability", service.ErrInvalidInput)
		}
	}
	references, err := s.validateGenerationReferences(c, req.ProjectID, target.InputModalities, target.Features, target.Capability, target.AdapterCode, req.InputReferences)
	if err != nil {
		return err
	}
	snapshot := db.JSON(map[string]any{
		"prompt": req.Prompt, "prompt_preset_id": req.PromptPresetID, "model_preset_id": req.ModelPresetID,
		"input_references": references, "target_kind": target.Kind, "target_id": target.Identifier, "provider": target.ProviderCode,
		"parameters": json.RawMessage(parameters), "output_name": strings.TrimSpace(req.OutputName), "image_task": req.ImageTask,
	})
	job, err := s.store.CreateGenerationJob(c.Context(), db.CreateGenerationJobInput{
		ProjectID: req.ProjectID, EpisodeID: req.EpisodeID, CanvasNodeID: req.CanvasNodeID,
		TargetAssetGroupID: req.TargetAssetGroupID, PromptPresetID: req.PromptPresetID,
		ModelPresetID: req.ModelPresetID, TargetKind: target.Kind, ProviderID: target.ProviderID,
		ModelID: target.ModelID, WorkflowTemplateID: target.WorkflowID, TargetSnapshot: target.Snapshot, Capability: target.Capability,
		Prompt: strings.TrimSpace(req.Prompt), Parameters: parameters, InputReferences: references,
		InputSnapshot: snapshot, OutputName: req.OutputName,
	})
	if err != nil {
		return err
	}
	if _, err := s.queue.EnqueueGeneration(c.Context(), job.ID); err != nil {
		_, _ = s.store.MarkGenerationFailed(c.Context(), job.ID, "enqueue generation: "+err.Error())
		return err
	}
	return writeCreated(c, job)
}

func validateVideoReferences(target resolvedGenerationTarget, references []generationInputReferenceRequest) error {
	if target.Kind != "model" || target.Capability != "video" || len(references) > 0 || slices.Contains(target.Features, "text_to_video") {
		return nil
	}
	for _, feature := range []string{"image_to_video", "first_frame", "first_last_frame", "multi_reference", "subject_reference", "video_continuation", "audio_driven"} {
		if slices.Contains(target.Features, feature) {
			return fmt.Errorf("%w: selected video model requires at least one storyboard reference", service.ErrInvalidInput)
		}
	}
	return nil
}

func validateImageTask(target resolvedGenerationTarget, task *generationImageTaskRequest, references []generationInputReferenceRequest) error {
	if task == nil {
		return nil
	}
	task.Type = strings.ToLower(strings.TrimSpace(task.Type))
	if target.Kind != "model" || target.Capability != "image" {
		return fmt.Errorf("%w: special image tasks require an image model", service.ErrInvalidInput)
	}
	if task.SourceWidth < 512 || task.SourceWidth > 4096 || task.SourceHeight < 512 || task.SourceHeight > 4096 {
		return fmt.Errorf("%w: source image dimensions must be between 512 and 4096 pixels", service.ErrInvalidInput)
	}
	switch task.Type {
	case "outpaint":
		if !slices.Contains(target.Features, "outpaint") {
			return fmt.Errorf("%w: selected model does not support outpainting", service.ErrInvalidInput)
		}
		if len(references) != 1 {
			return fmt.Errorf("%w: outpainting requires exactly one source image", service.ErrInvalidInput)
		}
		for _, scale := range []float64{task.TopScale, task.BottomScale, task.LeftScale, task.RightScale} {
			if scale < 1 || scale > 2 {
				return fmt.Errorf("%w: outpainting scales must be between 1 and 2", service.ErrInvalidInput)
			}
		}
		if task.TopScale == 1 && task.BottomScale == 1 && task.LeftScale == 1 && task.RightScale == 1 {
			return fmt.Errorf("%w: outpainting canvas must extend beyond the source image", service.ErrInvalidInput)
		}
		if task.TargetWidth < task.SourceWidth || task.TargetHeight < task.SourceHeight ||
			(task.TargetWidth == task.SourceWidth && task.TargetHeight == task.SourceHeight) || task.SourceX < 0 || task.SourceY < 0 {
			return fmt.Errorf("%w: outpainting canvas geometry is invalid", service.ErrInvalidInput)
		}
		expectedWidth := int(math.Round(float64(task.SourceWidth) * (task.LeftScale + task.RightScale - 1)))
		expectedHeight := int(math.Round(float64(task.SourceHeight) * (task.TopScale + task.BottomScale - 1)))
		expectedX := int(math.Round(float64(task.SourceWidth) * (task.LeftScale - 1)))
		expectedY := int(math.Round(float64(task.SourceHeight) * (task.TopScale - 1)))
		if task.TargetWidth != expectedWidth || task.TargetHeight != expectedHeight || task.SourceX != expectedX || task.SourceY != expectedY {
			return fmt.Errorf("%w: outpainting geometry does not match its scale values", service.ErrInvalidInput)
		}
	case "inpaint":
		if !slices.Contains(target.Features, "inpaint") || !slices.Contains(target.Features, "mask_input") {
			return fmt.Errorf("%w: selected model does not support masked inpainting", service.ErrInvalidInput)
		}
		if len(references) != 2 || references[1].Source != "upload" {
			return fmt.Errorf("%w: masked inpainting requires one source image and one generated mask", service.ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: image_task.type must be outpaint or inpaint", service.ErrInvalidInput)
	}
	return nil
}

func (s *Server) resolveGenerationTarget(c fiber.Ctx, req generationRequest) (resolvedGenerationTarget, error) {
	kind := strings.TrimSpace(req.TargetKind)
	if kind == "" {
		if req.WorkflowTemplateID != nil {
			kind = "workflow"
		} else {
			kind = "model"
		}
	}
	switch kind {
	case "model":
		if req.ModelID == nil || *req.ModelID == uuid.Nil || req.WorkflowTemplateID != nil || req.ProviderID != nil {
			return resolvedGenerationTarget{}, fmt.Errorf("%w: model target requires only model_id", service.ErrInvalidInput)
		}
		model, err := s.store.GetModel(c.Context(), *req.ModelID)
		if err != nil {
			return resolvedGenerationTarget{}, err
		}
		if !model.Enabled || !model.Available {
			return resolvedGenerationTarget{}, fmt.Errorf("%w: selected model is disabled or unavailable", service.ErrInvalidInput)
		}
		provider, err := s.store.GetModelProvider(c.Context(), model.ProviderID)
		if err != nil {
			return resolvedGenerationTarget{}, err
		}
		return resolvedGenerationTarget{
			Kind: "model", ModelID: req.ModelID, Capability: model.Capability, InputModalities: model.InputModalities, Features: model.Features,
			ProviderCode: model.ProviderCode, AdapterCode: provider.AdapterCode, Identifier: model.ModelID, DefaultParameters: model.DefaultParameters,
			Snapshot: db.JSON(map[string]any{"kind": "model", "id": model.ModelID, "capability": model.Capability}),
		}, nil
	case "workflow":
		if req.WorkflowTemplateID == nil || *req.WorkflowTemplateID == uuid.Nil || req.ProviderID == nil || *req.ProviderID == uuid.Nil || req.ModelID != nil || req.ModelPresetID != nil {
			return resolvedGenerationTarget{}, fmt.Errorf("%w: workflow target requires provider_id and workflow_template_id", service.ErrInvalidInput)
		}
		workflow, err := s.store.GetWorkflowTemplate(c.Context(), *req.WorkflowTemplateID)
		if err != nil {
			return resolvedGenerationTarget{}, err
		}
		provider, err := s.store.GetModelProvider(c.Context(), *req.ProviderID)
		if err != nil {
			return resolvedGenerationTarget{}, err
		}
		if !workflow.Enabled || !provider.Enabled || provider.AdapterCode != service.ProviderComfyUI {
			return resolvedGenerationTarget{}, fmt.Errorf("%w: selected ComfyUI workflow or connection is disabled", service.ErrInvalidInput)
		}
		workflowID, providerID := workflow.ID, provider.ID
		compatibilities, err := s.store.ListWorkflowCompatibilities(c.Context(), &workflowID, &providerID)
		if err != nil {
			return resolvedGenerationTarget{}, err
		}
		if len(compatibilities) == 0 || compatibilities[0].Status != "ready" {
			return resolvedGenerationTarget{}, fmt.Errorf("%w: check this workflow against the selected ComfyUI connection before generating", service.ErrInvalidInput)
		}
		return resolvedGenerationTarget{
			Kind: "workflow", ProviderID: req.ProviderID, WorkflowID: req.WorkflowTemplateID,
			Capability: workflow.Capability, InputModalities: workflow.InputModalities,
			ProviderCode: provider.Code, AdapterCode: provider.AdapterCode, Identifier: fmt.Sprintf("%s@%d", workflow.Code, workflow.Version),
			DefaultParameters: workflow.DefaultParameters, Snapshot: service.WorkflowTargetSpec(workflow),
		}, nil
	default:
		return resolvedGenerationTarget{}, fmt.Errorf("%w: target_kind must be model or workflow", service.ErrInvalidInput)
	}
}

func (s *Server) validateGenerationReferences(c fiber.Ctx, projectID uuid.UUID, modalities, features []string, capability, adapterCode string, requested []generationInputReferenceRequest) ([]db.GenerationInputReference, error) {
	allowed := allowedReferenceMedia(modalities, capability)
	if len(requested) > 0 && len(allowed) == 0 {
		return nil, fmt.Errorf("%w: selected execution target does not accept reference files", service.ErrInvalidInput)
	}
	seen := make(map[uuid.UUID]struct{}, len(requested))
	references := make([]db.GenerationInputReference, 0, len(requested))
	for _, requestedReference := range requested {
		if requestedReference.ID == uuid.Nil {
			return nil, fmt.Errorf("%w: reference id is required", service.ErrInvalidInput)
		}
		if _, duplicate := seen[requestedReference.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate generation reference", service.ErrInvalidInput)
		}
		seen[requestedReference.ID] = struct{}{}
		var mediaType string
		var publicSourceURL string
		switch requestedReference.Source {
		case "asset":
			asset, err := s.store.GetAsset(c.Context(), requestedReference.ID)
			if err != nil {
				return nil, err
			}
			if asset.ProjectID != projectID || asset.Status == "discarded" {
				return nil, fmt.Errorf("%w: reference asset is unavailable", service.ErrInvalidInput)
			}
			mediaType = asset.MediaType
		case "upload":
			upload, err := s.store.GetGenerationReferenceUpload(c.Context(), requestedReference.ID)
			if err != nil {
				return nil, err
			}
			if upload.ProjectID != projectID || upload.JobID != nil {
				return nil, fmt.Errorf("%w: temporary reference is unavailable", service.ErrInvalidInput)
			}
			mediaType = upload.MediaType
			publicSourceURL = generationReferenceSourceURL(upload.Metadata)
		default:
			return nil, fmt.Errorf("%w: reference source must be asset or upload", service.ErrInvalidInput)
		}
		if _, ok := allowed[mediaType]; !ok {
			return nil, fmt.Errorf("%w: %s references are not supported by this model capability", service.ErrInvalidInput, mediaType)
		}
		remoteExportID := requestedReference.RemoteExportID
		requiresRemote := providerRequiresRemoteReferences(adapterCode) && !slices.Contains(features, "local_reference")
		if requiresRemote || slices.Contains(features, "remote_reference_required") {
			if requestedReference.Source == "upload" && publicSourceURL != "" {
				remoteExportID = nil
			} else if requestedReference.Source != "asset" {
				return nil, fmt.Errorf("%w: temporary references cannot be sent to a cloud model; import and publish the asset first", service.ErrInvalidInput)
			} else if remoteExportID == nil {
				exports, exportErr := s.store.ListAssetRemoteExports(c.Context(), requestedReference.ID)
				if exportErr != nil {
					return nil, exportErr
				}
				for _, exported := range exports {
					if exported.State == "ready" && exported.ConnectionEnabled && exported.ConnectionIsDefault {
						exportID := exported.ID
						remoteExportID = &exportID
						break
					}
				}
				if remoteExportID == nil {
					for _, exported := range exports {
						if exported.State == "ready" && exported.ConnectionEnabled {
							exportID := exported.ID
							remoteExportID = &exportID
							break
						}
					}
				}
			}
			if requestedReference.Source == "asset" && remoteExportID == nil {
				return nil, fmt.Errorf("%w: publish reference asset %s to S3 before using it with a cloud model", service.ErrInvalidInput, requestedReference.ID)
			}
			if remoteExportID != nil {
				exported, exportErr := s.store.GetAssetRemoteExport(c.Context(), *remoteExportID)
				if exportErr != nil || exported.AssetID != requestedReference.ID || exported.State != "ready" || !exported.ConnectionEnabled {
					return nil, fmt.Errorf("%w: remote export does not belong to the reference asset", service.ErrInvalidInput)
				}
			}
		}
		references = append(references, db.GenerationInputReference{Source: requestedReference.Source, ID: requestedReference.ID, RemoteExportID: remoteExportID})
	}
	return references, nil
}

func providerRequiresRemoteReferences(adapterCode string) bool {
	switch adapterCode {
	case service.ProviderOllama, service.ProviderComfyUI, service.ProviderSiliconFlow, service.ProviderZhipu, service.ProviderTencentTokenHub, service.ProviderMoonshot:
		return false
	default:
		return true
	}
}

func allowedReferenceMedia(modalities []string, capability string) map[string]struct{} {
	if len(modalities) > 0 {
		allowed := make(map[string]struct{}, len(modalities))
		for _, mediaType := range modalities {
			if mediaType != "text" {
				allowed[mediaType] = struct{}{}
			}
		}
		return allowed
	}
	switch capability {
	case "image":
		return map[string]struct{}{"image": {}}
	case "video":
		return map[string]struct{}{"image": {}, "video": {}, "audio": {}}
	case "multimodal":
		return map[string]struct{}{"image": {}, "video": {}, "audio": {}, "text": {}, "file": {}}
	default:
		return map[string]struct{}{}
	}
}
func (s *Server) getGenerationJob(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	job, err := s.store.GetGenerationJob(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, job)
}

func (s *Server) getGenerationInvocation(c fiber.Ctx) error {
	jobID, err := idParam(c, "id")
	if err != nil {
		return err
	}
	if _, err := s.store.GetGenerationJob(c.Context(), jobID); err != nil {
		return err
	}
	invocation, err := s.store.GetGenerationInvocationByJob(c.Context(), jobID)
	if err != nil {
		return err
	}
	return writeOK(c, invocation)
}

func (s *Server) listGenerationJobs(c fiber.Ctx) error {
	projectID, err := uuid.Parse(c.Query("project_id"))
	if err != nil {
		return fiber.NewError(400, "project_id is required")
	}
	episodeID, err := parseOptionalUUID(c.Query("episode_id"))
	if err != nil {
		return fiber.NewError(400, "invalid episode_id")
	}
	page, pageSize := parsePage(c, 20, 100)
	items, err := s.store.ListGenerationJobs(c.Context(), db.GenerationJobFilter{
		ProjectID: projectID, EpisodeID: episodeID, Status: c.Query("status"), Capability: c.Query("capability"),
		Search: c.Query("search"), Page: page, PageSize: pageSize,
	})
	if err != nil {
		return err
	}
	return writeOK(c, items)
}

func mergeJSON(base, override json.RawMessage) json.RawMessage {
	values := map[string]any{}
	_ = json.Unmarshal(base, &values)
	extra := map[string]any{}
	_ = json.Unmarshal(override, &extra)
	for key, value := range extra {
		values[key] = value
	}
	return db.JSON(values)
}
