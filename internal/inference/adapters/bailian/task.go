package bailian

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type taskResponse struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Output    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
	} `json:"output"`
	Usage map[string]any `json:"usage"`
}

func (d *Driver) submitTask(ctx context.Context, request inference.Request, events inference.EventSink, path string, payload map[string]any) (string, error) {
	provider := request.Runtime.ProviderCode
	taskID := strings.TrimSpace(request.ProviderRunID)
	if taskID != "" {
		return taskID, nil
	}
	url := endpoint(request.Runtime.Endpoint, path)
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Bailian asynchronous request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": url, "payload": payload},
	}); err != nil {
		return "", err
	}
	var submitted taskResponse
	_, err := d.http.DoJSONWithHeaders(ctx, provider, http.MethodPost, url, request.Runtime.APIKey, map[string]string{"X-DashScope-Async": "enable"}, payload, &submitted)
	if err != nil {
		return "", err
	}
	if err := bailianError(provider, submitted.Code, submitted.Message); err != nil {
		return "", err
	}
	taskID = strings.TrimSpace(submitted.Output.TaskID)
	if taskID == "" {
		return "", inference.NewError(inference.ErrorInvalidOutput, provider, "Bailian response has no task id", false, nil)
	}
	return taskID, nil
}

func (d *Driver) waitTask(ctx context.Context, request inference.Request, events inference.EventSink, taskID string) (map[string]any, json.RawMessage, error) {
	provider := request.Runtime.ProviderCode
	if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", ProviderRunID: taskID}); err != nil {
		return nil, nil, err
	}
	url := endpoint(request.Runtime.Endpoint, "/api/v1/tasks/"+taskID)
	for {
		var response map[string]any
		raw, err := d.http.DoJSON(ctx, provider, http.MethodGet, url, request.Runtime.APIKey, nil, &response)
		if err != nil {
			return nil, nil, err
		}
		code := adapterutil.FindString(response, "code")
		message := adapterutil.FindString(response, "message")
		if err := bailianError(provider, code, message); err != nil {
			return nil, nil, err
		}
		status := strings.ToUpper(adapterutil.FindString(response, "task_status"))
		switch status {
		case "SUCCEEDED", "SUCCESS", "COMPLETED":
			return response, raw, nil
		case "FAILED", "CANCELED", "CANCELLED", "UNKNOWN":
			if message == "" {
				message = "Bailian task " + strings.ToLower(status)
			}
			return nil, raw, inference.NewError(inference.ErrorProvider, provider, message, false, nil)
		default:
			if err := inference.Emit(ctx, events, inference.Event{Stage: "running", ProviderRunID: taskID}); err != nil {
				return nil, nil, err
			}
		}
		timer := time.NewTimer(d.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func bailianError(provider, code, message string) error {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	if strings.TrimSpace(message) == "" {
		message = code
	}
	return inference.NewError(inference.ErrorProvider, provider, message, false, nil)
}
