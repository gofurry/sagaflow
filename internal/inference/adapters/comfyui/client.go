package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const maxJSONResponse = int64(64 << 20)

// Driver is safe for concurrent use. It serializes workflows per ComfyUI
// connection by default so one worker cannot accidentally overload a local GPU.
type Driver struct {
	http         HTTPDoer
	pollInterval time.Duration
	mu           sync.Mutex
	semaphores   map[string]chan struct{}
}

func New(config Config) *Driver {
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	interval := config.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	return &Driver{http: client, pollInterval: interval, semaphores: make(map[string]chan struct{})}
}

func (d *Driver) Probe(ctx context.Context, runtime inference.Runtime) (ServerInfo, error) {
	var stats systemStatsResponse
	if err := d.getJSON(ctx, runtime, "/system_stats", &stats); err != nil {
		return ServerInfo{}, err
	}
	features := map[string]any{}
	if err := d.getJSON(ctx, runtime, "/features", &features); err != nil && !responseStatus(err, http.StatusNotFound) {
		return ServerInfo{}, err
	}
	return ServerInfo{
		Version: stringValue(stats.System["comfyui_version"]), System: stats.System,
		Devices: stats.Devices, Features: features,
	}, nil
}

func (d *Driver) Discover(ctx context.Context, runtime inference.Runtime) (Discovery, error) {
	server, err := d.Probe(ctx, runtime)
	if err != nil {
		return Discovery{}, err
	}
	objectInfo := map[string]json.RawMessage{}
	if err := d.getJSON(ctx, runtime, "/object_info", &objectInfo); err != nil {
		return Discovery{}, err
	}
	nodes := make([]string, 0, len(objectInfo))
	for name := range objectInfo {
		nodes = append(nodes, name)
	}
	sort.Strings(nodes)
	var folders []string
	if err := d.getJSON(ctx, runtime, "/models", &folders); err != nil {
		return Discovery{}, err
	}
	models := make(map[string][]string, len(folders))
	for _, folder := range folders {
		var names []string
		if err := d.getJSON(ctx, runtime, "/models/"+url.PathEscape(folder), &names); err != nil {
			return Discovery{}, err
		}
		sort.Strings(names)
		models[folder] = names
		server.ModelCount += len(names)
	}
	server.NodeCount = len(nodes)
	return Discovery{Server: server, Nodes: nodes, Models: models, NodeDefinitions: objectInfo}, nil
}

func (d *Driver) acquire(ctx context.Context, runtime inference.Runtime) (func(), error) {
	limit := maxConcurrency(runtime.Configuration)
	connection := strings.TrimSpace(runtime.ProviderCode)
	if connection == "" {
		connection = strings.TrimRight(strings.TrimSpace(runtime.Endpoint), "/")
	}
	key := connection + ":" + strconv.Itoa(limit)
	d.mu.Lock()
	semaphore := d.semaphores[key]
	if semaphore == nil {
		semaphore = make(chan struct{}, limit)
		d.semaphores[key] = semaphore
	}
	d.mu.Unlock()
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func maxConcurrency(raw json.RawMessage) int {
	var config struct {
		MaxConcurrency int `json:"max_concurrency"`
	}
	_ = json.Unmarshal(raw, &config)
	if config.MaxConcurrency < 1 || config.MaxConcurrency > 8 {
		return 1
	}
	return config.MaxConcurrency
}

func (d *Driver) getJSON(ctx context.Context, runtime inference.Runtime, path string, out any) error {
	return d.doJSON(ctx, runtime, http.MethodGet, path, nil, out)
}

func (d *Driver) doJSON(ctx context.Context, runtime inference.Runtime, method, path string, payload, out any) error {
	endpoint, err := apiEndpoint(runtime.Endpoint, path)
	if err != nil {
		return inference.NewError(inference.ErrorInvalidRequest, providerName(runtime), "invalid ComfyUI endpoint", false, err)
	}
	var body io.Reader
	if payload != nil {
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return inference.NewError(inference.ErrorInvalidRequest, providerName(runtime), "encode ComfyUI request", false, marshalErr)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return inference.NewError(inference.ErrorInvalidRequest, providerName(runtime), "create ComfyUI request", false, err)
	}
	setHeaders(request, runtime.APIKey)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := d.http.Do(request)
	if err != nil {
		return inference.NewError(inference.ErrorUnavailable, providerName(runtime), "send ComfyUI request", true, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxJSONResponse+1))
	if err != nil {
		return inference.NewError(inference.ErrorProvider, providerName(runtime), "read ComfyUI response", true, err)
	}
	if int64(len(data)) > maxJSONResponse {
		return inference.NewError(inference.ErrorInvalidOutput, providerName(runtime), "ComfyUI response exceeds JSON limit", false, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return inference.ResponseError(providerName(runtime), response.StatusCode, adapterutil.Truncate(string(data), 1200))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return inference.NewError(inference.ErrorInvalidOutput, providerName(runtime), "decode ComfyUI response", false, err)
	}
	return nil
}

func apiEndpoint(baseURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("endpoint must be an absolute http(s) URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return parsed.String(), nil
}

func setHeaders(request *http.Request, key string) {
	request.Header.Set("Accept", "application/json")
	if key = strings.TrimSpace(key); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
}

func providerName(runtime inference.Runtime) string {
	if value := strings.TrimSpace(runtime.ProviderCode); value != "" {
		return value
	}
	return "comfyui"
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func responseStatus(err error, status int) bool {
	var responseErr *inference.Error
	return errors.As(err, &responseErr) && responseErr.StatusCode == status
}
