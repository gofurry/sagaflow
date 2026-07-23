package moonshot

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
	inputs, err := materializeInputs(ctx, request)
	if err != nil {
		return inference.Result{}, err
	}
	content := make([]any, 0, len(inputs)+1)
	traceContent := make([]any, 0, len(inputs)+1)
	for _, input := range inputs {
		switch input.MediaType {
		case "image":
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": input.Value}})
			traceContent = append(traceContent, map[string]any{"type": "image_url", "image_url": map[string]any{"url": traceReference(input.Value)}})
		case "video":
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]any{"url": input.Value}})
			traceContent = append(traceContent, map[string]any{"type": "video_url", "video_url": map[string]any{"url": traceReference(input.Value)}})
		}
	}
	content = append(content, map[string]any{"type": "text", "text": request.Prompt})
	traceContent = append(traceContent, map[string]any{"type": "text", "text": request.Prompt})

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
	for _, key := range []string{"max_tokens", "max_completion_tokens", "reasoning_effort"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	copyObjectParameter(payload, request.Parameters, "thinking", "type")
	copyObjectParameter(payload, request.Parameters, "response_format", "type")

	target := endpoint(request.Runtime.Endpoint, "/chat/completions")
	tracePayload := cloneMap(payload)
	tracePayload["messages"] = []map[string]any{{"role": "user", "content": traceUserContent}}
	if err := emitRequest(ctx, events, target, tracePayload); err != nil {
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
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Kimi response has no choices", false, nil)
	}
	text := strings.TrimSpace(contentText(response.Choices[0].Message.Content))
	if text == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Kimi response content is empty", false, nil)
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

func copyObjectParameter(payload, parameters map[string]any, key, scalarKey string) {
	value, ok := parameters[key]
	if !ok {
		return
	}
	if text, isString := value.(string); isString {
		if strings.TrimSpace(text) != "" {
			payload[key] = map[string]any{scalarKey: text}
		}
		return
	}
	payload[key] = value
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

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
