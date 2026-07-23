package openaicompat

import (
	"encoding/json"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func chatMessages(provider string, request inference.Request) ([]map[string]any, error) {
	if len(request.Inputs) == 0 {
		return []map[string]any{{"role": "user", "content": request.Prompt}}, nil
	}
	content := []any{map[string]any{"type": "text", "text": request.Prompt}}
	for _, input := range request.Inputs {
		if input.MediaType != "image" {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "OpenAI Chat references currently support images only", false, nil)
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return nil, err
		}
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": input.URL}})
	}
	return []map[string]any{{"role": "user", "content": content}}, nil
}

func responsesInput(provider string, request inference.Request) (any, error) {
	if len(request.Inputs) == 0 {
		return request.Prompt, nil
	}
	content := []any{map[string]any{"type": "input_text", "text": request.Prompt}}
	for _, input := range request.Inputs {
		if input.MediaType != "image" {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "Responses API references currently support images only", false, nil)
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return nil, err
		}
		content = append(content, map[string]any{"type": "input_image", "image_url": input.URL})
	}
	return []map[string]any{{"role": "user", "content": content}}, nil
}

func copyChatParameters(payload, parameters map[string]any) {
	for _, key := range []string{
		"max_tokens", "max_completion_tokens", "temperature", "top_p", "top_k", "min_p", "stop", "seed",
		"frequency_penalty", "presence_penalty", "logprobs", "top_logprobs", "n", "user",
		"reasoning_effort", "reasoning_split", "enable_thinking", "thinking_budget",
	} {
		adapterutil.CopyParam(payload, parameters, key)
	}
	copyStructuredParameter(payload, parameters, "thinking", "type")
	copyStructuredParameter(payload, parameters, "response_format", "type")
}

func copyResponsesParameters(payload, parameters map[string]any) {
	for _, key := range []string{"max_output_tokens", "temperature", "top_p", "store", "user"} {
		adapterutil.CopyParam(payload, parameters, key)
	}
	copyStructuredParameter(payload, parameters, "thinking", "type")
	if value, ok := parameters["reasoning"]; ok {
		payload["reasoning"] = value
	} else if effort := adapterutil.StringParam(parameters, "reasoning_effort", ""); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if value, ok := parameters["text"]; ok {
		payload["text"] = value
	}
}

func copyStructuredParameter(payload, parameters map[string]any, key, scalarKey string) {
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

func chatContent(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if value, ok := block["text"].(string); ok && strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "")
}

func responsesText(response map[string]any) string {
	if text, ok := response["output_text"].(string); ok && strings.TrimSpace(text) != "" {
		return text
	}
	outputs, _ := response["output"].([]any)
	parts := make([]string, 0)
	for _, output := range outputs {
		item, _ := output.(map[string]any)
		content, _ := item["content"].([]any)
		for _, raw := range content {
			block, _ := raw.(map[string]any)
			typeName, _ := block["type"].(string)
			text, _ := block["text"].(string)
			if (typeName == "output_text" || typeName == "text" || typeName == "") && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "")
}

func responseError(response map[string]any) string {
	raw, _ := response["error"].(map[string]any)
	message, _ := raw["message"].(string)
	return strings.TrimSpace(message)
}
