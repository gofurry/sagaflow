package tencenttokenhub

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
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
	SupportsGeneration bool     `json:"supports_generation"`
}

type Discovery struct {
	Server ServerInfo  `json:"server"`
	Models []ModelInfo `json:"models"`
}

var supportedModels = map[string]ModelInfo{
	"hy3": {
		DisplayName: "Hunyuan 3", Task: "chat", Capability: "text",
		InputModalities: []string{"text"}, Features: []string{"chat", "thinking", "structured_output", "long_context"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hy3-preview": {
		DisplayName: "Hunyuan 3 Preview", Task: "chat", Capability: "text",
		InputModalities: []string{"text"}, Features: []string{"chat", "thinking", "structured_output", "long_context"},
		SupportStatus: "compatible", SupportsGeneration: true,
	},
	"hunyuan-role-latest": {
		DisplayName: "Hunyuan Role", Task: "chat", Capability: "text",
		InputModalities: []string{"text"}, Features: []string{"chat", "roleplay", "dialogue"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hy-vision-2.0-instruct": {
		DisplayName: "Hunyuan Vision 2.0 Instruct", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image"}, Features: []string{"chat", "vision"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hunyuan-t1-vision-20250916": {
		DisplayName: "Hunyuan T1 Vision", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image"}, Features: []string{"chat", "vision", "thinking"},
		SupportStatus: "compatible", SupportsGeneration: true,
	},
	"youtu-vita": {
		DisplayName: "YT VITA", Task: "chat", Capability: "text",
		InputModalities: []string{"text", "image", "video"}, Features: []string{"chat", "vision", "video_understanding"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hy-image-v3.0": {
		DisplayName: "Hunyuan Image 3.0", Task: "image_generation", Capability: "image",
		InputModalities: []string{"text", "image"}, Features: []string{"image_generation", "image_edit", "multi_reference", "chinese_text", "remote_reference_required"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hy-image-lite": {
		DisplayName: "Hunyuan Image Lite", Task: "image_generation_lite", Capability: "image",
		InputModalities: []string{"text"}, Features: []string{"image_generation", "fast", "chinese_text"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"hy-video-1.5": {
		DisplayName: "Hunyuan Video 1.5", Task: "text_to_video", Capability: "video",
		InputModalities: []string{"text", "image"}, Features: []string{"video_generation", "text_to_video", "image_to_video"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"kl-video-v3": {
		DisplayName: "Kling Video v3", Task: "text_to_video", Capability: "video",
		InputModalities: []string{"text", "image"}, Features: []string{"video_generation", "text_to_video", "image_to_video", "first_last_frame", "native_audio", "smart_storyboard"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"vd-video-q3-pro": {
		DisplayName: "Vidu Video Q3 Pro", Task: "text_to_video", Capability: "video",
		InputModalities: []string{"text", "image"}, Features: []string{"video_generation", "text_to_video", "image_to_video", "first_last_frame", "native_audio", "1080p"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"vd-video-q3-turbo": {
		DisplayName: "Vidu Video Q3 Turbo", Task: "text_to_video", Capability: "video",
		InputModalities: []string{"text", "image"}, Features: []string{"video_generation", "text_to_video", "image_to_video", "first_last_frame", "native_audio", "1080p", "fast"},
		SupportStatus: "verified", SupportsGeneration: true,
	},
	"yt-video-2.0": {
		DisplayName: "YT Video 2.0", Task: "image_to_video", Capability: "video",
		InputModalities: []string{"text", "image"}, Features: []string{"video_generation", "image_to_video", "remote_reference_required"},
		SupportStatus: "compatible", SupportsGeneration: true,
	},
	"hy-mt2-pro": {
		DisplayName: "Hunyuan MT 2 Pro", Task: "chat", Capability: "text",
		InputModalities: []string{"text"}, Features: []string{"chat", "translation"},
		SupportStatus: "compatible", SupportsGeneration: true,
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
		Data []json.RawMessage `json:"data"`
	}
	target := endpoint(runtime.Endpoint, "/models")
	if _, err := d.http.DoJSON(ctx, providerCode, http.MethodGet, target, runtime.APIKey, nil, &response); err != nil {
		return Discovery{}, err
	}
	models := make([]ModelInfo, 0, len(supportedModels))
	for _, raw := range response.Data {
		var item map[string]any
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		modelID := firstString(item, "id", "model", "name")
		template, ok := supportedModels[modelID]
		if !ok {
			continue
		}
		template.Name = modelID
		template.RemoteStatus = strings.ToLower(firstString(item, "status", "state", "lifecycle_status"))
		if template.RemoteStatus == "" {
			template.RemoteStatus = "online"
		}
		template.LifecycleStatus = "active"
		if template.RemoteStatus == "pre-offline" {
			template.LifecycleStatus = "deprecated"
		}
		models = append(models, template)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Capability != models[j].Capability {
			return models[i].Capability < models[j].Capability
		}
		return models[i].Name < models[j].Name
	})
	return Discovery{
		Server: ServerInfo{Provider: providerCode, ModelCount: len(models), LastCheckedAt: time.Now().UTC()},
		Models: models,
	}, nil
}
