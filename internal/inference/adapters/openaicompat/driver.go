// Package openaicompat implements the protocol-level OpenAI Chat Completions
// and Responses APIs used by multiple cloud model providers.
package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type Protocol string

const (
	ProtocolChat      Protocol = "chat_completions"
	ProtocolResponses Protocol = "responses"
)

type Driver struct {
	http         *adapterutil.HTTPClient
	protocol     Protocol
	endpointPath string
}

func NewChat(client *http.Client) *Driver {
	return NewChatAt(client, "/chat/completions")
}

func NewChatAt(client *http.Client, endpointPath string) *Driver {
	return &Driver{http: adapterutil.NewHTTPClient(client), protocol: ProtocolChat, endpointPath: normalizePath(endpointPath, "/chat/completions")}
}

func NewResponses(client *http.Client) *Driver {
	return NewResponsesAt(client, "/responses")
}

func NewResponsesAt(client *http.Client, endpointPath string) *Driver {
	return &Driver{http: adapterutil.NewHTTPClient(client), protocol: ProtocolResponses, endpointPath: normalizePath(endpointPath, "/responses")}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := strings.TrimSpace(request.Runtime.ProviderCode)
	if provider == "" {
		provider = string(d.protocol)
	}
	if request.Target.Kind != inference.TargetModel || request.Target.Capability != inference.CapabilityText {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "OpenAI-compatible driver only supports text model targets", false, nil)
	}
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	switch d.protocol {
	case ProtocolChat:
		return d.executeChat(ctx, provider, request, events)
	case ProtocolResponses:
		return d.executeResponses(ctx, provider, request, events)
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "unsupported OpenAI-compatible protocol", false, nil)
	}
}

func (d *Driver) executeChat(ctx context.Context, provider string, request inference.Request, events inference.EventSink) (inference.Result, error) {
	messages, err := chatMessages(provider, request)
	if err != nil {
		return inference.Result{}, err
	}
	payload := map[string]any{"model": request.Target.ID, "messages": messages, "stream": false}
	copyChatParameters(payload, request.Parameters)
	endpoint := endpointURL(request.Runtime.Endpoint, d.endpointPath)
	if err := emitRequest(ctx, events, endpoint, payload); err != nil {
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
	raw, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if response.Error != nil {
		return inference.Result{}, inference.NewError(inference.ErrorProvider, provider, response.Error.Message, false, nil)
	}
	if len(response.Choices) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "text response has no choices", false, nil)
	}
	content := strings.TrimSpace(chatContent(response.Choices[0].Message.Content))
	if content == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "text response content is empty", false, nil)
	}
	return textResult(content, response.Usage, adapterutil.JSONSummary(raw, "id", "model", "usage")), nil
}

func (d *Driver) executeResponses(ctx context.Context, provider string, request inference.Request, events inference.EventSink) (inference.Result, error) {
	input, err := responsesInput(provider, request)
	if err != nil {
		return inference.Result{}, err
	}
	payload := map[string]any{"model": request.Target.ID, "input": input, "stream": false}
	copyResponsesParameters(payload, request.Parameters)
	endpoint := endpointURL(request.Runtime.Endpoint, d.endpointPath)
	if err := emitRequest(ctx, events, endpoint, payload); err != nil {
		return inference.Result{}, err
	}
	var response map[string]any
	raw, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if message := responseError(response); message != "" {
		return inference.Result{}, inference.NewError(inference.ErrorProvider, provider, message, false, nil)
	}
	content := strings.TrimSpace(responsesText(response))
	if content == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Responses API returned no output text", false, nil)
	}
	usage, _ := response["usage"].(map[string]any)
	return textResult(content, usage, adapterutil.JSONSummary(raw, "id", "model", "usage", "status")), nil
}

func textResult(content string, usage map[string]any, metadata json.RawMessage) inference.Result {
	if usage == nil {
		usage = map[string]any{}
	}
	return inference.Result{
		Artifacts: []inference.Artifact{{
			MediaType: "text", MIMEType: "text/markdown; charset=utf-8", Metadata: metadata,
			Content: inference.BytesContent([]byte(content), "text/markdown; charset=utf-8"),
		}},
		Usage: usage,
	}
}

func emitRequest(ctx context.Context, events inference.EventSink, endpoint string, payload map[string]any) error {
	return inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "OpenAI-compatible request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	})
}

func normalizePath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return "/" + strings.Trim(value, "/")
}

func endpointURL(baseURL, path string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
}
