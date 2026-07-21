package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type ServerInfo struct {
	Version      string `json:"version"`
	ModelCount   int    `json:"model_count"`
	RunningCount int    `json:"running_count"`
}

type ModelInfo struct {
	Name               string         `json:"name"`
	Digest             string         `json:"digest"`
	Size               int64          `json:"size"`
	ModifiedAt         time.Time      `json:"modified_at"`
	Details            map[string]any `json:"details"`
	Capabilities       []string       `json:"capabilities"`
	Parameters         string         `json:"parameters"`
	ContextLength      int64          `json:"context_length"`
	InputModalities    []string       `json:"input_modalities"`
	Features           []string       `json:"features"`
	SupportsTextOutput bool           `json:"supports_text_output"`
}

type Discovery struct {
	Server ServerInfo  `json:"server"`
	Models []ModelInfo `json:"models"`
}

func (d *Driver) Probe(ctx context.Context, runtime inference.Runtime) (ServerInfo, error) {
	var version struct {
		Version string `json:"version"`
	}
	if err := d.doJSON(ctx, runtime, http.MethodGet, "/api/version", nil, &version); err != nil {
		return ServerInfo{}, err
	}
	var tags struct {
		Models []tagModel `json:"models"`
	}
	if err := d.doJSON(ctx, runtime, http.MethodGet, "/api/tags", nil, &tags); err != nil {
		return ServerInfo{}, err
	}
	var ps struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := d.doJSON(ctx, runtime, http.MethodGet, "/api/ps", nil, &ps); err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{Version: version.Version, ModelCount: len(tags.Models), RunningCount: len(ps.Models)}, nil
}

func (d *Driver) Discover(ctx context.Context, runtime inference.Runtime) (Discovery, error) {
	server, err := d.Probe(ctx, runtime)
	if err != nil {
		return Discovery{}, err
	}
	var tags struct {
		Models []tagModel `json:"models"`
	}
	if err := d.doJSON(ctx, runtime, http.MethodGet, "/api/tags", nil, &tags); err != nil {
		return Discovery{}, err
	}
	models := make([]ModelInfo, 0, len(tags.Models))
	for _, tag := range tags.Models {
		var show showModel
		if err := d.doJSON(ctx, runtime, http.MethodPost, "/api/show", map[string]any{"model": tag.Model, "verbose": false}, &show); err != nil {
			return Discovery{}, err
		}
		capabilities := uniqueStrings(show.Capabilities)
		features := make([]string, 0, len(capabilities))
		inputs := []string{"text"}
		if contains(capabilities, "vision") {
			inputs = append(inputs, "image")
		}
		for _, capability := range capabilities {
			if capability != "completion" && capability != "vision" {
				features = append(features, capability)
			}
		}
		models = append(models, ModelInfo{
			Name: tag.Model, Digest: tag.Digest, Size: tag.Size, ModifiedAt: tag.ModifiedAt,
			Details: tag.Details, Capabilities: capabilities, Parameters: show.Parameters,
			ContextLength: contextLength(show.ModelInfo), InputModalities: inputs, Features: features,
			SupportsTextOutput: contains(capabilities, "completion"),
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return Discovery{Server: server, Models: models}, nil
}

type tagModel struct {
	Name       string         `json:"name"`
	Model      string         `json:"model"`
	ModifiedAt time.Time      `json:"modified_at"`
	Size       int64          `json:"size"`
	Digest     string         `json:"digest"`
	Details    map[string]any `json:"details"`
}

type showModel struct {
	Parameters   string         `json:"parameters"`
	Capabilities []string       `json:"capabilities"`
	ModelInfo    map[string]any `json:"model_info"`
}

func (d *Driver) doJSON(ctx context.Context, runtime inference.Runtime, method, path string, payload, output any) error {
	provider := providerName(runtime)
	endpoint, err := apiEndpoint(runtime.Endpoint, path)
	if err != nil {
		return inference.NewError(inference.ErrorInvalidRequest, provider, "invalid runtime endpoint", false, err)
	}
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		data, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return inference.NewError(inference.ErrorInvalidRequest, provider, "encode Ollama catalog request", false, encodeErr)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return inference.NewError(inference.ErrorInvalidRequest, provider, "create Ollama catalog request", false, err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if key := strings.TrimSpace(runtime.APIKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := d.http.Do(request)
	if err != nil {
		return inference.NewError(inference.ErrorUnavailable, provider, "send Ollama catalog request", true, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return inference.NewError(inference.ErrorProvider, provider, "read Ollama catalog response", true, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return inference.ResponseError(provider, response.StatusCode, adapterutil.Truncate(string(data), 600))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return inference.NewError(inference.ErrorInvalidOutput, provider, "decode Ollama catalog response", false, err)
	}
	return nil
}

func contextLength(info map[string]any) int64 {
	for key, value := range info {
		if !strings.HasSuffix(key, ".context_length") {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed)
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		}
	}
	return 0
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
