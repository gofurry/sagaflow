// Package ollama implements SagaFlow's native Ollama inference adapter.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const (
	maxImageCount       = 8
	maxImageBytes       = int64(16 << 20)
	maxTotalImageBytes  = int64(48 << 20)
	maxStreamBytes      = int64(32 << 20)
	maxGeneratedTextLen = 32 << 20
)

// Driver is safe for concurrent use. Each configured Ollama connection is
// serialized by default so a local GPU is not overloaded by the global queue.
type Driver struct {
	http       *http.Client
	mu         sync.Mutex
	semaphores map[string]chan struct{}
}

func New(client *http.Client) *Driver {
	if client == nil {
		client = http.DefaultClient
	}
	return &Driver{http: client, semaphores: make(map[string]chan struct{})}
}

type chatRequest struct {
	Model       string         `json:"model"`
	Messages    []chatMessage  `json:"messages"`
	Options     map[string]any `json:"options,omitempty"`
	Stream      bool           `json:"stream"`
	Think       any            `json:"think,omitempty"`
	KeepAlive   any            `json:"keep_alive,omitempty"`
	Format      any            `json:"format,omitempty"`
	Logprobs    any            `json:"logprobs,omitempty"`
	TopLogprobs any            `json:"top_logprobs,omitempty"`
}

type chatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type chatChunk struct {
	Model   string `json:"model"`
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int64  `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int64  `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
	Error              string `json:"error"`
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := providerName(request.Runtime)
	if request.Target.Kind != inference.TargetModel || request.Target.Capability != inference.CapabilityText {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "Ollama adapter currently supports text output only", false, nil)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "prompt is required", false, nil)
	}
	release, err := d.acquire(ctx, request.Runtime)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorUnavailable, provider, "wait for Ollama connection capacity", true, err)
	}
	defer release()
	images, imageBytes, err := loadImages(ctx, provider, request.Inputs)
	if err != nil {
		return inference.Result{}, err
	}
	payload := chatRequest{
		Model:    request.Target.ID,
		Messages: []chatMessage{{Role: "user", Content: request.Prompt, Images: images}},
		Options:  map[string]any{},
		Stream:   true,
	}
	params := request.Parameters
	for _, key := range []string{"num_ctx", "num_predict", "temperature", "top_k", "top_p", "min_p", "repeat_penalty", "repeat_last_n", "seed", "stop"} {
		if value, ok := params[key]; ok {
			payload.Options[key] = value
		}
	}
	if len(payload.Options) == 0 {
		payload.Options = nil
	}
	payload.Think = params["think"]
	payload.KeepAlive = params["keep_alive"]
	payload.Format = params["format"]
	payload.Logprobs = params["logprobs"]
	payload.TopLogprobs = params["top_logprobs"]

	endpoint, err := apiEndpoint(request.Runtime.Endpoint, "/api/chat")
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "invalid runtime endpoint", false, err)
	}
	tracePayload := map[string]any{
		"model": payload.Model, "messages": []map[string]any{{"role": "user", "content": request.Prompt, "image_count": len(images), "image_bytes": imageBytes}},
		"options": payload.Options, "stream": true,
	}
	for key, value := range map[string]any{"think": payload.Think, "keep_alive": payload.KeepAlive, "format": payload.Format, "logprobs": payload.Logprobs, "top_logprobs": payload.TopLogprobs} {
		if value != nil {
			tracePayload[key] = value
		}
	}
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Ollama chat request prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": tracePayload},
	}); err != nil {
		return inference.Result{}, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "encode Ollama request", false, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "create Ollama request", false, err)
	}
	httpRequest.Header.Set("Accept", "application/x-ndjson")
	httpRequest.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(request.Runtime.APIKey); key != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := d.http.Do(httpRequest)
	if err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorUnavailable, provider, "send Ollama request", true, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 601))
		return inference.Result{}, inference.ResponseError(provider, response.StatusCode, adapterutil.Truncate(string(message), 600))
	}

	limited := &countingLimitReader{reader: bufio.NewReader(response.Body), remaining: maxStreamBytes}
	decoder := json.NewDecoder(limited)
	var content strings.Builder
	var thinking strings.Builder
	var final chatChunk
	firstOutput := false
	for {
		var chunk chatChunk
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			if limited.exceeded {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Ollama stream exceeds response limit", false, nil)
			}
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "decode Ollama stream", false, err)
		}
		if chunk.Error != "" {
			return inference.Result{}, inference.NewError(inference.ErrorProvider, provider, chunk.Error, false, nil)
		}
		if content.Len()+len(chunk.Message.Content) > maxGeneratedTextLen || thinking.Len()+len(chunk.Message.Thinking) > maxGeneratedTextLen {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Ollama response exceeds text limit", false, nil)
		}
		content.WriteString(chunk.Message.Content)
		thinking.WriteString(chunk.Message.Thinking)
		if !firstOutput && (chunk.Message.Content != "" || chunk.Message.Thinking != "") {
			firstOutput = true
			if err := inference.Emit(ctx, events, inference.Event{Stage: "generating", Progress: .45, Message: "Ollama returned the first output token"}); err != nil {
				return inference.Result{}, err
			}
		}
		if chunk.Done {
			final = chunk
			break
		}
	}
	if limited.exceeded {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Ollama stream exceeds response limit", false, nil)
	}
	text := strings.TrimSpace(content.String())
	if !final.Done || text == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Ollama returned no completed text output", false, nil)
	}
	usage := map[string]any{
		"total_duration_ns": final.TotalDuration, "load_duration_ns": final.LoadDuration,
		"prompt_eval_count": final.PromptEvalCount, "prompt_eval_duration_ns": final.PromptEvalDuration,
		"eval_count": final.EvalCount, "eval_duration_ns": final.EvalDuration,
	}
	metadata := adapterutil.JSON(map[string]any{
		"model": final.Model, "done_reason": final.DoneReason, "thinking_characters": thinking.Len(),
		"image_count": len(images), "image_bytes": imageBytes,
	})
	return inference.Result{
		Artifacts: []inference.Artifact{{
			MediaType: "text", MIMEType: "text/markdown; charset=utf-8", Metadata: metadata,
			Content: inference.BytesContent([]byte(text), "text/markdown; charset=utf-8"),
		}},
		Usage: usage,
	}, nil
}

func (d *Driver) acquire(ctx context.Context, runtime inference.Runtime) (func(), error) {
	limit := maxConcurrency(runtime.Configuration)
	connection := strings.TrimSpace(runtime.ProviderCode)
	if connection == "" {
		connection = strings.TrimRight(strings.TrimSpace(runtime.Endpoint), "/")
	}
	// Include the capacity so a changed connection setting takes effect for new
	// requests without requiring the worker process to restart.
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

func loadImages(ctx context.Context, provider string, inputs []inference.Input) ([]string, int64, error) {
	if len(inputs) > maxImageCount {
		return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, fmt.Sprintf("Ollama accepts at most %d reference images", maxImageCount), false, nil)
	}
	images := make([]string, 0, len(inputs))
	var total int64
	for _, input := range inputs {
		if input.MediaType != "image" || input.Content == nil {
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "Ollama text generation only accepts image references", false, nil)
		}
		reader, info, err := input.Content.Open(ctx)
		if err != nil {
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "open Ollama image reference", false, err)
		}
		if info.Size > maxImageBytes || (info.Size >= 0 && total+info.Size > maxTotalImageBytes) {
			reader.Close()
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "Ollama image references exceed size limit", false, nil)
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, maxImageBytes+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "read Ollama image reference", false, readErr)
		}
		if closeErr != nil {
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "close Ollama image reference", false, closeErr)
		}
		if int64(len(data)) > maxImageBytes || total+int64(len(data)) > maxTotalImageBytes {
			return nil, 0, inference.NewError(inference.ErrorInvalidRequest, provider, "Ollama image references exceed size limit", false, nil)
		}
		total += int64(len(data))
		images = append(images, base64.StdEncoding.EncodeToString(data))
	}
	return images, total, nil
}

func providerName(runtime inference.Runtime) string {
	if value := strings.TrimSpace(runtime.ProviderCode); value != "" {
		return value
	}
	return "ollama"
}

func apiEndpoint(baseURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("endpoint must be an absolute http(s) URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), "/api") + path
	return parsed.String(), nil
}

type countingLimitReader struct {
	reader    io.Reader
	remaining int64
	exceeded  bool
}

func (r *countingLimitReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			r.exceeded = true
			return 0, fmt.Errorf("response limit exceeded")
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}
