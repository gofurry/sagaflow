package siliconflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
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
	SupportsGeneration bool     `json:"supports_generation"`
}

type Discovery struct {
	Server ServerInfo  `json:"server"`
	Models []ModelInfo `json:"models"`
}

func (d *Driver) Probe(ctx context.Context, runtime inference.Runtime) (ServerInfo, error) {
	discovery, err := d.Discover(ctx, runtime)
	return discovery.Server, err
}

func (d *Driver) Discover(ctx context.Context, runtime inference.Runtime) (Discovery, error) {
	if err := adapterutil.Required(runtime.Endpoint, "runtime endpoint", providerCode); err != nil {
		return Discovery{}, err
	}
	type query struct {
		values url.Values
		task   string
	}
	queries := []query{
		{values: url.Values{"sub_type": {"chat"}}, task: "chat"},
		{values: url.Values{"sub_type": {"text-to-image"}}, task: "image_generation"},
		{values: url.Values{"sub_type": {"image-to-image"}}, task: "image_edit"},
		{values: url.Values{"type": {"audio"}}, task: "speech_generation"},
		{values: url.Values{"type": {"video"}}, task: "text_to_video"},
	}
	byKey := map[string]ModelInfo{}
	for _, item := range queries {
		models, err := d.listModels(ctx, runtime, item.values)
		if err != nil {
			return Discovery{}, err
		}
		for _, modelID := range models {
			info := describeModel(modelID, item.task)
			key := strings.ToLower(info.Name) + "\x00" + info.Capability
			if current, exists := byKey[key]; exists {
				current.Features = mergeStrings(current.Features, info.Features)
				current.InputModalities = mergeStrings(current.InputModalities, info.InputModalities)
				if current.Task == "image_generation" && info.Task == "image_edit" {
					current.Task = info.Task
				}
				byKey[key] = current
				continue
			}
			byKey[key] = info
		}
	}
	models := make([]ModelInfo, 0, len(byKey))
	for _, model := range byKey {
		if model.SupportsGeneration {
			models = append(models, model)
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Capability != models[j].Capability {
			return models[i].Capability < models[j].Capability
		}
		return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
	})
	return Discovery{
		Server: ServerInfo{Provider: providerCode, ModelCount: len(models), LastCheckedAt: time.Now().UTC()},
		Models: models,
	}, nil
}

func (d *Driver) listModels(ctx context.Context, runtime inference.Runtime, query url.Values) ([]string, error) {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	target := endpoint(runtime.Endpoint, "/models")
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	if _, err := d.http.DoJSON(ctx, providerCode, http.MethodGet, target, runtime.APIKey, nil, &response); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(response.Data))
	for _, item := range response.Data {
		if value := strings.TrimSpace(item.ID); value != "" {
			models = append(models, value)
		}
	}
	return models, nil
}

func describeModel(modelID, fallbackTask string) ModelInfo {
	task := fallbackTask
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "sensevoice") || strings.Contains(lower, "speechasr"):
		task = "speech_recognition"
	case strings.Contains(lower, "image-edit"):
		task = "image_edit"
	case strings.Contains(lower, "i2v"):
		task = "image_to_video"
	case strings.Contains(lower, "t2v"):
		task = "text_to_video"
	}
	info := ModelInfo{
		Name: modelID, DisplayName: displayName(modelID), Task: task,
		SupportStatus: "compatible", SupportsGeneration: true,
	}
	switch task {
	case "chat":
		info.Capability = "text"
		info.InputModalities = []string{"text"}
		info.Features = []string{"chat"}
		if strings.Contains(lower, "vl") || strings.Contains(lower, "vision") || strings.Contains(lower, "omni") ||
			strings.Contains(lower, "qwen3.5") || strings.Contains(lower, "qwen3.6") {
			info.InputModalities = []string{"text", "image"}
			info.Features = append(info.Features, "vision")
		}
		if strings.Contains(lower, "thinking") || strings.Contains(lower, "reasoning") || strings.Contains(lower, "deepseek") ||
			strings.Contains(lower, "glm") || strings.Contains(lower, "qwen3") {
			info.Features = append(info.Features, "thinking")
		}
	case "image_generation":
		info.Capability = "image"
		info.InputModalities = []string{"text"}
		info.Features = []string{"image_generation"}
	case "image_edit":
		info.Capability = "image"
		info.InputModalities = []string{"text", "image"}
		info.Features = []string{"image_edit", "multi_reference"}
	case "speech_generation":
		info.Capability = "audio"
		info.InputModalities = []string{"text"}
		info.Features = []string{"speech_generation"}
		if strings.Contains(lower, "cosyvoice") || strings.Contains(lower, "moss-ttsd") {
			info.InputModalities = []string{"text", "audio"}
			info.Features = append(info.Features, "voice_clone")
		}
	case "speech_recognition":
		info.Capability = "text"
		info.InputModalities = []string{"audio"}
		info.Features = []string{"speech_recognition"}
	case "text_to_video":
		info.Capability = "video"
		info.InputModalities = []string{"text"}
		info.Features = []string{"video_generation", "text_to_video"}
	case "image_to_video":
		info.Capability = "video"
		info.InputModalities = []string{"text", "image"}
		info.Features = []string{"video_generation", "image_to_video"}
	default:
		info.SupportStatus = "experimental"
		info.SupportsGeneration = false
	}
	return info
}

func displayName(modelID string) string {
	value := path.Base(strings.TrimSpace(modelID))
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	return strings.Join(strings.Fields(value), " ")
}

func ParameterSchema(info ModelInfo) json.RawMessage {
	properties := map[string]any{}
	switch info.Task {
	case "chat":
		properties = map[string]any{
			"max_tokens":      map[string]any{"type": "integer", "title": "最大输出 Token", "minimum": 1, "maximum": 131072},
			"temperature":     map[string]any{"type": "number", "title": "温度", "minimum": 0, "maximum": 2},
			"top_p":           map[string]any{"type": "number", "title": "Top P", "minimum": 0, "maximum": 1},
			"enable_thinking": map[string]any{"type": "boolean", "title": "启用思考"},
			"thinking_budget": map[string]any{"type": "integer", "title": "思考预算", "minimum": 128, "maximum": 32768},
		}
	case "image_generation":
		examples := []string{"1024x1024", "1280x720", "720x1280"}
		lowerName := strings.ToLower(info.Name)
		if strings.Contains(lowerName, "kolors") {
			examples = []string{"1024x1024", "960x1280", "768x1024", "720x1440", "720x1280"}
		} else if strings.Contains(lowerName, "qwen") {
			examples = []string{"1328x1328", "1664x928", "928x1664", "1472x1140", "1140x1472", "1584x1056", "1056x1584"}
		}
		properties = map[string]any{
			"image_size":      map[string]any{"type": "string", "title": "图像尺寸", "description": "选择官方建议尺寸或输入具体模型支持的 宽x高。", "pattern": `^[1-9][0-9]{2,4}x[1-9][0-9]{2,4}$`, "examples": examples},
			"negative_prompt": map[string]any{"type": "string", "title": "负面 Prompt"},
			"seed":            map[string]any{"type": "integer", "title": "随机种子", "minimum": 0},
		}
		if strings.Contains(strings.ToLower(info.Name), "kolors") {
			properties["batch_size"] = map[string]any{"type": "integer", "title": "生成数量", "minimum": 1, "maximum": 4}
			properties["num_inference_steps"] = map[string]any{"type": "integer", "title": "推理步数", "minimum": 1, "maximum": 100}
			properties["guidance_scale"] = map[string]any{"type": "number", "title": "提示词引导", "minimum": 0, "maximum": 20}
		}
	case "image_edit":
		properties = map[string]any{
			"negative_prompt": map[string]any{"type": "string", "title": "负面 Prompt"},
			"seed":            map[string]any{"type": "integer", "title": "随机种子", "minimum": 0},
		}
	case "speech_generation":
		properties = map[string]any{
			"voice":           map[string]any{"type": "string", "title": "音色"},
			"response_format": map[string]any{"type": "string", "title": "输出格式", "enum": []string{"mp3", "wav", "pcm", "opus"}},
			"sample_rate":     map[string]any{"type": "integer", "title": "采样率"},
			"speed":           map[string]any{"type": "number", "title": "语速", "minimum": .25, "maximum": 4},
			"gain":            map[string]any{"type": "number", "title": "增益", "minimum": -10, "maximum": 10},
			"reference_text":  map[string]any{"type": "string", "title": "参考音频文本"},
		}
	case "text_to_video", "image_to_video":
		properties = map[string]any{
			"image_size":      map[string]any{"type": "string", "title": "视频尺寸", "enum": []string{"1280x720", "720x1280", "960x960"}},
			"negative_prompt": map[string]any{"type": "string", "title": "负面 Prompt"},
			"seed":            map[string]any{"type": "integer", "title": "随机种子", "minimum": 0},
		}
	}
	return mustJSON(map[string]any{"type": "object", "properties": properties})
}

func DefaultParameters(info ModelInfo) json.RawMessage {
	switch info.Task {
	case "chat":
		return mustJSON(map[string]any{"max_tokens": 4096, "temperature": .7})
	case "image_generation":
		parameters := map[string]any{"image_size": "1024x1024"}
		if strings.Contains(strings.ToLower(info.Name), "kolors") {
			parameters["batch_size"] = 1
			parameters["num_inference_steps"] = 20
			parameters["guidance_scale"] = 7.5
		}
		return mustJSON(parameters)
	case "speech_generation":
		return mustJSON(map[string]any{"response_format": "mp3", "sample_rate": 44100, "speed": 1, "gain": 0})
	case "text_to_video", "image_to_video":
		return mustJSON(map[string]any{"image_size": "1280x720"})
	default:
		return mustJSON(map[string]any{})
	}
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func mergeStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	values := make([]string, 0, len(left)+len(right))
	for _, items := range [][]string{left, right} {
		for _, item := range items {
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			values = append(values, item)
		}
	}
	return values
}
