package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type Driver struct{ http *adapterutil.HTTPClient }

func New(client *http.Client) *Driver {
	return &Driver{http: adapterutil.NewHTTPClient(client)}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	const provider = "deepseek"
	if request.Target.Kind != inference.TargetModel || request.Target.Capability != inference.CapabilityText {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "driver only supports text generation", false, nil)
	}
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	params := request.Parameters
	if params == nil {
		params = map[string]any{}
	}
	payload := map[string]any{"model": request.Target.ID, "messages": []map[string]string{{"role": "user", "content": request.Prompt}}}
	for _, key := range []string{"reasoning_effort", "max_tokens", "temperature", "top_p", "stop", "logprobs", "top_logprobs", "user_id"} {
		adapterutil.CopyParam(payload, params, key)
	}
	if thinking := adapterutil.StringParam(params, "thinking", "enabled"); thinking != "" {
		payload["thinking"] = map[string]string{"type": thinking}
	}
	if responseFormat := adapterutil.StringParam(params, "response_format", "text"); responseFormat != "" {
		payload["response_format"] = map[string]string{"type": responseFormat}
	}
	endpoint := strings.TrimRight(request.Runtime.Endpoint, "/") + "/chat/completions"
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "DeepSeek request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	}); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage any `json:"usage"`
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
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	usage := map[string]any{}
	if encoded, marshalErr := json.Marshal(response.Usage); marshalErr == nil && string(encoded) != "null" {
		_ = json.Unmarshal(encoded, &usage)
	}
	return inference.Result{
		Artifacts: []inference.Artifact{{
			MediaType: "text", MIMEType: "text/plain; charset=utf-8",
			Metadata: adapterutil.JSONSummary(raw, "id", "usage"),
			Content:  inference.BytesContent([]byte(content), "text/plain; charset=utf-8"),
		}},
		Usage: usage,
	}, nil
}
