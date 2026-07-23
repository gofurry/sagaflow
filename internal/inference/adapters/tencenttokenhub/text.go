package tencenttokenhub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateText(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", providerName); err != nil {
		return inference.Result{}, err
	}
	inputs, err := materializeInputs(ctx, request, "image", "video")
	if err != nil {
		return inference.Result{}, err
	}
	if strings.EqualFold(request.Target.ID, "hy-vision-2.0-instruct") && len(inputs) > 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Hunyuan Vision accepts one image reference", false, nil)
	}
	content := make([]any, 0, len(inputs)+1)
	content = append(content, map[string]any{"type": "text", "text": request.Prompt})
	traceContent := make([]any, 0, len(inputs)+1)
	traceContent = append(traceContent, map[string]any{"type": "text", "text": request.Prompt})
	for _, input := range inputs {
		switch input.MediaType {
		case "image":
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": input.dataURL()}})
			traceContent = append(traceContent, map[string]any{"type": "image_url", "image_url": map[string]any{"url": input.traceValue()}})
		case "video":
			if input.URL == "" {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "TokenHub video understanding requires an S3-published video URL", false, nil)
			}
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": input.URL}})
			traceContent = append(traceContent, map[string]any{"type": "video_url", "video_url": map[string]any{"url": input.URL}})
		}
	}
	var userContent any = request.Prompt
	var traceUserContent any = request.Prompt
	if len(inputs) > 0 {
		userContent = content
		traceUserContent = traceContent
	}
	payload := map[string]any{
		"model":    request.Target.ID,
		"messages": []map[string]any{{"role": "user", "content": userContent}},
		"stream":   false,
	}
	for _, key := range []string{
		"max_tokens", "temperature", "top_p", "stop", "seed", "frequency_penalty",
		"presence_penalty", "reasoning_effort", "response_format",
	} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	if thinking, ok := request.Parameters["thinking"]; ok {
		if value, isString := thinking.(string); isString {
			payload["thinking"] = map[string]any{"type": value}
		} else {
			payload["thinking"] = thinking
		}
	}
	target := endpoint(request.Runtime.Endpoint, "/chat/completions")
	trace := cloneMap(payload)
	trace["messages"] = []map[string]any{{"role": "user", "content": traceUserContent}}
	if err := emitRequest(ctx, events, target, trace); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]any `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	raw, err := d.http.DoJSON(ctx, providerName, http.MethodPost, target, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if response.Error != nil {
		return inference.Result{}, inference.NewError(inference.ErrorProvider, providerName, response.Error.Message, false, nil)
	}
	if len(response.Choices) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub text response has no choices", false, nil)
	}
	text := strings.TrimSpace(contentText(response.Choices[0].Message.Content))
	if text == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub text response is empty", false, nil)
	}
	return inference.Result{
		Artifacts: []inference.Artifact{{
			MediaType: "text", MIMEType: "text/markdown; charset=utf-8",
			Metadata: adapterutil.JSONSummary(raw, "id", "model", "usage"),
			Content:  inference.BytesContent([]byte(text), "text/markdown; charset=utf-8"),
		}},
		Usage: response.Usage,
	}, nil
}

func contentText(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text, ok := block["text"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}
