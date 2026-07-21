package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/comfyui"
	"github.com/gofurry/sagaflow/internal/inference/adapters/ollama"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type ModelConnectionService struct {
	store       *db.Store
	credentials *CredentialService
	ollama      *ollama.Driver
	comfyui     *comfyui.Driver
}

type ModelSyncResult struct {
	Server   ollama.ServerInfo `json:"server"`
	Models   []db.Model        `json:"models"`
	Imported int               `json:"imported"`
	Updated  int               `json:"updated"`
}

func NewModelConnectionService(store *db.Store, credentials *CredentialService, ollamaDriver *ollama.Driver, comfyDriver *comfyui.Driver) (*ModelConnectionService, error) {
	if store == nil || credentials == nil || ollamaDriver == nil || comfyDriver == nil {
		return nil, fmt.Errorf("model connection service dependencies are required")
	}
	return &ModelConnectionService{store: store, credentials: credentials, ollama: ollamaDriver, comfyui: comfyDriver}, nil
}

func (s *ModelConnectionService) Test(ctx context.Context, providerID uuid.UUID) (any, error) {
	provider, runtime, err := s.runtime(ctx, providerID)
	if err != nil {
		return nil, err
	}
	switch provider.AdapterCode {
	case ProviderOllama:
		server, probeErr := s.ollama.Probe(ctx, runtime)
		if probeErr != nil {
			return nil, providerConnectionError(provider)
		}
		if err := s.updateOllamaMetadata(ctx, provider, server); err != nil {
			return nil, err
		}
		return server, nil
	case ProviderComfyUI:
		server, probeErr := s.comfyui.Probe(ctx, runtime)
		if probeErr != nil {
			return nil, providerConnectionError(provider)
		}
		if err := s.updateComfyMetadata(ctx, provider, server); err != nil {
			return nil, err
		}
		return server, nil
	default:
		return nil, fmt.Errorf("%w: connection testing is unavailable for adapter %s", ErrInvalidInput, provider.AdapterCode)
	}
}

func (s *ModelConnectionService) Discover(ctx context.Context, providerID uuid.UUID) (any, error) {
	provider, runtime, err := s.runtime(ctx, providerID)
	if err != nil {
		return nil, err
	}
	switch provider.AdapterCode {
	case ProviderOllama:
		discovery, discoverErr := s.ollama.Discover(ctx, runtime)
		if discoverErr != nil {
			return nil, providerConnectionError(provider)
		}
		if err := s.updateOllamaMetadata(ctx, provider, discovery.Server); err != nil {
			return nil, err
		}
		return discovery, nil
	case ProviderComfyUI:
		discovery, discoverErr := s.comfyui.Discover(ctx, runtime)
		if discoverErr != nil {
			return nil, providerConnectionError(provider)
		}
		if err := s.updateComfyMetadata(ctx, provider, discovery.Server); err != nil {
			return nil, err
		}
		return discovery, nil
	default:
		return nil, fmt.Errorf("%w: discovery is unavailable for adapter %s", ErrInvalidInput, provider.AdapterCode)
	}
}

func (s *ModelConnectionService) Sync(ctx context.Context, providerID uuid.UUID, selectedModelIDs []string) (ModelSyncResult, error) {
	provider, runtime, err := s.runtime(ctx, providerID)
	if err != nil {
		return ModelSyncResult{}, err
	}
	if provider.AdapterCode != ProviderOllama {
		return ModelSyncResult{}, fmt.Errorf("%w: only Ollama connections synchronize model catalog entries", ErrInvalidInput)
	}
	discovery, err := s.ollama.Discover(ctx, runtime)
	if err != nil {
		return ModelSyncResult{}, providerConnectionError(provider)
	}
	if err := s.updateOllamaMetadata(ctx, provider, discovery.Server); err != nil {
		return ModelSyncResult{}, err
	}
	existing, err := s.store.ListModelsByProvider(ctx, providerID)
	if err != nil {
		return ModelSyncResult{}, err
	}
	selected := make(map[string]struct{}, len(selectedModelIDs))
	for _, modelID := range selectedModelIDs {
		if modelID = strings.TrimSpace(modelID); modelID != "" {
			selected[modelID] = struct{}{}
		}
	}
	selectAll := selectedModelIDs == nil
	existingByID := make(map[string]db.Model, len(existing))
	for _, model := range existing {
		existingByID[model.ModelID] = model
	}
	result := ModelSyncResult{Server: discovery.Server, Models: make([]db.Model, 0, len(discovery.Models))}
	availableModelIDs := make(map[string]struct{}, len(discovery.Models))
	for _, discovered := range discovery.Models {
		if !discovered.SupportsTextOutput {
			continue
		}
		availableModelIDs[discovered.Name] = struct{}{}
		metadata := db.JSON(map[string]any{
			"source": "ollama", "digest": discovered.Digest, "size": discovered.Size,
			"modified_at": discovered.ModifiedAt, "details": discovered.Details,
			"capabilities": discovered.Capabilities, "parameters": discovered.Parameters,
			"context_length": discovered.ContextLength, "last_seen_at": time.Now().UTC(),
		})
		if model, ok := existingByID[discovered.Name]; ok {
			model.InputModalities = discovered.InputModalities
			model.Features = discovered.Features
			model.ParameterSchema = ollamaParameterSchema(discovered.Capabilities)
			model.DefaultParameters = ollamaDefaultParameters()
			model.Available = true
			model.Metadata = metadata
			updated, updateErr := s.store.UpdateModel(ctx, model)
			if updateErr != nil {
				return ModelSyncResult{}, updateErr
			}
			result.Models = append(result.Models, updated)
			result.Updated++
			continue
		}
		if !selectAll {
			if _, ok := selected[discovered.Name]; !ok {
				continue
			}
		}
		created, createErr := s.store.CreateModel(ctx, db.Model{
			ProviderID: providerID, ModelID: discovered.Name, DisplayName: discovered.Name,
			Capability: "text", InputModalities: discovered.InputModalities, Features: discovered.Features,
			ParameterSchema: ollamaParameterSchema(discovered.Capabilities), DefaultParameters: ollamaDefaultParameters(),
			Enabled: true, Available: true, Metadata: metadata,
		})
		if createErr != nil {
			return ModelSyncResult{}, createErr
		}
		result.Models = append(result.Models, created)
		result.Imported++
	}
	for _, model := range existing {
		if _, available := availableModelIDs[model.ModelID]; available || !model.Available {
			continue
		}
		model.Available = false
		if _, err := s.store.UpdateModel(ctx, model); err != nil {
			return ModelSyncResult{}, err
		}
	}
	return result, nil
}

func providerConnectionError(provider db.ModelProvider) error {
	return fmt.Errorf("%w: 无法连接“%s”，请确认服务已启动且 Base URL 正确", ErrProviderUnavailable, provider.DisplayName)
}

func (s *ModelConnectionService) runtime(ctx context.Context, providerID uuid.UUID) (db.ModelProvider, inference.Runtime, error) {
	provider, err := s.store.GetModelProvider(ctx, providerID)
	if err != nil {
		return db.ModelProvider{}, inference.Runtime{}, mapStoreError(err)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return db.ModelProvider{}, inference.Runtime{}, err
	}
	return provider, inference.Runtime{
		ProviderCode: provider.Code, AdapterCode: provider.AdapterCode,
		Endpoint: secret.BaseURL, APIKey: secret.APIKey, Configuration: provider.Metadata,
	}, nil
}

func (s *ModelConnectionService) updateOllamaMetadata(ctx context.Context, provider db.ModelProvider, server ollama.ServerInfo) error {
	metadata := map[string]any{}
	_ = json.Unmarshal(provider.Metadata, &metadata)
	metadata["server_version"] = server.Version
	metadata["model_count"] = server.ModelCount
	metadata["running_count"] = server.RunningCount
	metadata["last_checked_at"] = time.Now().UTC()
	provider.Metadata = db.JSON(metadata)
	_, err := s.store.UpdateModelProvider(ctx, provider)
	return err
}

func (s *ModelConnectionService) updateComfyMetadata(ctx context.Context, provider db.ModelProvider, server comfyui.ServerInfo) error {
	metadata := map[string]any{}
	_ = json.Unmarshal(provider.Metadata, &metadata)
	metadata["server_version"] = server.Version
	metadata["node_count"] = server.NodeCount
	metadata["model_count"] = server.ModelCount
	metadata["devices"] = server.Devices
	metadata["features"] = server.Features
	metadata["last_checked_at"] = time.Now().UTC()
	provider.Metadata = db.JSON(metadata)
	_, err := s.store.UpdateModelProvider(ctx, provider)
	return err
}

func ollamaDefaultParameters() json.RawMessage {
	// Ollama models can define their own defaults in the Modelfile. Avoid
	// overriding those defaults until the user explicitly chooses a value.
	return db.JSON(map[string]any{})
}

func ollamaParameterSchema(capabilities []string) json.RawMessage {
	properties := map[string]any{
		"num_ctx":        map[string]any{"type": "integer", "title": "上下文长度", "minimum": 512, "maximum": 262144},
		"num_predict":    map[string]any{"type": "integer", "title": "最大输出 Token", "minimum": 1, "maximum": 32768},
		"temperature":    map[string]any{"type": "number", "title": "温度", "minimum": 0, "maximum": 2},
		"top_k":          map[string]any{"type": "integer", "title": "Top K", "minimum": 0, "maximum": 200},
		"top_p":          map[string]any{"type": "number", "title": "Top P", "minimum": 0, "maximum": 1},
		"min_p":          map[string]any{"type": "number", "title": "Min P", "minimum": 0, "maximum": 1},
		"repeat_penalty": map[string]any{"type": "number", "title": "重复惩罚", "minimum": 0, "maximum": 3},
		"repeat_last_n":  map[string]any{"type": "integer", "title": "重复检查窗口", "minimum": -1, "maximum": 32768},
		"seed":           map[string]any{"type": "integer", "title": "随机种子", "minimum": 0},
		"stop":           map[string]any{"type": "array", "title": "停止序列"},
		"keep_alive":     map[string]any{"type": "string", "title": "模型驻留时间", "description": "例如 5m；设为 0 可在请求后立即卸载"},
		"format":         map[string]any{"type": "string", "title": "输出格式", "enum": []string{"", "json"}},
		"logprobs":       map[string]any{"type": "boolean", "title": "返回 Logprobs"},
		"top_logprobs":   map[string]any{"type": "integer", "title": "Top Logprobs", "minimum": 0, "maximum": 20},
	}
	for _, capability := range capabilities {
		if strings.EqualFold(capability, "thinking") {
			properties["think"] = map[string]any{"type": "boolean", "title": "启用思考"}
			break
		}
	}
	return db.JSON(map[string]any{
		"type":       "object",
		"properties": properties,
	})
}
