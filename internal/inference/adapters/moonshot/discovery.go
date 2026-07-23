package moonshot

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type ServerInfo struct {
	Provider      string    `json:"provider"`
	ModelCount    int       `json:"model_count"`
	LastCheckedAt time.Time `json:"last_checked_at"`
}

type ModelInfo struct {
	Name               string   `json:"name"`
	DisplayName        string   `json:"display_name"`
	Task               string   `json:"task"`
	Capability         string   `json:"capability"`
	InputModalities    []string `json:"input_modalities"`
	Features           []string `json:"features"`
	SupportStatus      string   `json:"support_status"`
	LifecycleStatus    string   `json:"lifecycle_status"`
	RemoteStatus       string   `json:"remote_status"`
	ContextLength      int      `json:"context_length"`
	SupportsGeneration bool     `json:"supports_generation"`
}

type Discovery struct {
	Server ServerInfo  `json:"server"`
	Models []ModelInfo `json:"models"`
}

var supportedModels = map[string]ModelInfo{
	"kimi-k3": {
		DisplayName: "Kimi K3", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image", "video"},
		Features:        []string{"chat", "thinking", "structured_output", "vision", "video_understanding", "long_context", "tools"},
		SupportStatus:   "verified", SupportsGeneration: true,
	},
	"kimi-k2.6": {
		DisplayName: "Kimi K2.6", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image", "video"},
		Features:        []string{"chat", "thinking", "structured_output", "vision", "video_understanding", "long_context", "tools"},
		SupportStatus:   "verified", SupportsGeneration: true,
	},
	"kimi-k2.7-code": {
		DisplayName: "Kimi K2.7 Code", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image", "video"},
		Features:        []string{"chat", "thinking", "vision", "video_understanding", "long_context", "tools", "coding"},
		SupportStatus:   "compatible", SupportsGeneration: true,
	},
	"kimi-k2.7-code-highspeed": {
		DisplayName: "Kimi K2.7 Code Highspeed", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image", "video"},
		Features:        []string{"chat", "thinking", "vision", "video_understanding", "long_context", "tools", "coding", "fast"},
		SupportStatus:   "compatible", SupportsGeneration: true,
	},
}

func (d *Driver) Probe(ctx context.Context, runtime inference.Runtime) (ServerInfo, error) {
	discovery, err := d.Discover(ctx, runtime)
	return discovery.Server, err
}

func (d *Driver) Discover(ctx context.Context, runtime inference.Runtime) (Discovery, error) {
	if err := adapterutil.Required(runtime.Endpoint, "runtime endpoint", providerCode); err != nil {
		return Discovery{}, err
	}
	var response struct {
		Data []struct {
			ID                string `json:"id"`
			ContextLength     int    `json:"context_length"`
			SupportsImageIn   bool   `json:"supports_image_in"`
			SupportsVideoIn   bool   `json:"supports_video_in"`
			SupportsReasoning bool   `json:"supports_reasoning"`
		} `json:"data"`
	}
	if _, err := d.http.DoJSON(ctx, providerCode, http.MethodGet, endpoint(runtime.Endpoint, "/models"), runtime.APIKey, nil, &response); err != nil {
		return Discovery{}, err
	}
	models := make([]ModelInfo, 0, len(supportedModels))
	for _, remote := range response.Data {
		template, ok := supportedModels[remote.ID]
		if !ok {
			continue
		}
		template.Name = remote.ID
		template.ContextLength = remote.ContextLength
		template.LifecycleStatus = "active"
		template.RemoteStatus = "online"
		models = append(models, template)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return Discovery{
		Server: ServerInfo{Provider: providerCode, ModelCount: len(models), LastCheckedAt: time.Now().UTC()},
		Models: models,
	}, nil
}
