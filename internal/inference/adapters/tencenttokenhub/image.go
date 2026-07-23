package tencenttokenhub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if strings.EqualFold(request.Target.ID, "hy-image-lite") {
		return d.generateLiteImage(ctx, request, events)
	}
	return d.generateV3Image(ctx, request, events)
}

func (d *Driver) generateLiteImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if len(request.Inputs) != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Hunyuan Image Lite does not accept image references", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"negative_prompt", "resolution", "seed", "logo_add", "rsp_img_type"} {
		copyAliasedParameter(payload, request.Parameters, key)
	}
	target := endpoint(request.Runtime.Endpoint, "/api/image/lite")
	if err := emitRequest(ctx, events, target, payload); err != nil {
		return inference.Result{}, err
	}
	var response map[string]any
	raw, err := d.http.DoJSON(ctx, providerName, http.MethodPost, target, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	return d.imageResult(providerName, raw, response)
}

func (d *Driver) generateV3Image(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	inputs, err := materializeInputs(ctx, request, "image")
	if err != nil {
		return inference.Result{}, err
	}
	if len(inputs) > 3 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Hunyuan Image 3 accepts at most three image references", false, nil)
	}
	for _, input := range inputs {
		if input.URL == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "TokenHub Hunyuan Image 3 references must be manually published to S3 first", false, nil)
		}
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"resolution", "seed", "logo_add", "revise"} {
		copyAliasedParameter(payload, request.Parameters, key)
	}
	trace := cloneMap(payload)
	if len(inputs) > 0 {
		images := make([]string, 0, len(inputs))
		traced := make([]string, 0, len(inputs))
		for _, input := range inputs {
			images = append(images, input.URL)
			traced = append(traced, input.traceValue())
		}
		payload["images"] = images
		trace["images"] = traced
	}
	runID := strings.TrimSpace(request.ProviderRunID)
	if runID == "" {
		submitURL := endpoint(request.Runtime.Endpoint, "/api/image/submit")
		if err := emitRequest(ctx, events, submitURL, trace); err != nil {
			return inference.Result{}, err
		}
		var submitted map[string]any
		if _, err := d.http.DoJSON(ctx, providerName, http.MethodPost, submitURL, request.Runtime.APIKey, payload, &submitted); err != nil {
			return inference.Result{}, err
		}
		runID = firstString(submitted, "id", "task_id", "request_id")
		if runID == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub image response has no task ID", false, nil)
		}
		if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .3, ProviderRunID: runID, Message: "TokenHub image queued"}); err != nil {
			return inference.Result{}, err
		}
	}
	queryURL := endpoint(request.Runtime.Endpoint, "/api/image/query")
	notFoundRetries := 0
	for {
		if err := wait(ctx, d.pollInterval); err != nil {
			return inference.Result{}, err
		}
		var response map[string]any
		raw, err := d.http.DoJSON(ctx, providerName, http.MethodPost, queryURL, request.Runtime.APIKey, map[string]any{"model": request.Target.ID, "id": runID}, &response)
		if err != nil {
			return inference.Result{}, err
		}
		switch normalizeStatus(response) {
		case "completed", "success", "succeeded":
			return d.imageResult(providerName, raw, response)
		case "failed", "fail", "cancelled":
			message := responseMessage(response, "TokenHub image generation failed")
			if strings.Contains(message, "任务不存在") && notFoundRetries < 3 {
				notFoundRetries++
				continue
			}
			return inference.Result{}, inference.NewError(inference.ErrorProvider, providerName, message, false, nil)
		default:
			if err := inference.Emit(ctx, events, inference.Event{Stage: "running", Progress: .55, ProviderRunID: runID, Message: "TokenHub image generating"}); err != nil {
				return inference.Result{}, err
			}
		}
	}
}

func (d *Driver) imageResult(providerName string, raw json.RawMessage, response map[string]any) (inference.Result, error) {
	items := artifactItems(response)
	if len(items) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub image response has no image", false, nil)
	}
	artifacts := make([]inference.Artifact, 0, len(items))
	for _, item := range items {
		url := firstString(item, "url", "image_url", "result_image")
		b64 := firstString(item, "b64_json", "base64", "image_base64")
		artifact := inference.Artifact{
			MediaType: "image", MIMEType: "image/png",
			Metadata: adapterutil.JSONSummary(raw, "id", "task_id", "status", "seed", "data"),
		}
		switch {
		case url != "" && strings.HasPrefix(url, "http"):
			artifact.SourceURL = url
			artifact.Content = d.http.URLContent(providerName, url)
		case url != "":
			b64 = strings.TrimPrefix(url, "data:image/png;base64,")
			fallthrough
		case b64 != "":
			data, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "decode TokenHub image", false, err)
			}
			artifact.Content = inference.BytesContent(data, "image/png")
		default:
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub image items are empty", false, nil)
	}
	return inference.Result{Artifacts: artifacts}, nil
}

func copyAliasedParameter(payload, parameters map[string]any, key string) {
	aliases := map[string][]string{
		"logo_add":     {"logo_add", "watermark"},
		"rsp_img_type": {"rsp_img_type", "response_format"},
	}
	for _, alias := range append([]string{key}, aliases[key]...) {
		if value, ok := parameters[alias]; ok {
			if key == "logo_add" || key == "revise" {
				if enabled, isBool := value.(bool); isBool {
					if enabled {
						value = 1
					} else {
						value = 0
					}
				}
			}
			payload[key] = value
			return
		}
	}
}

func artifactItems(response map[string]any) []map[string]any {
	if data, ok := response["data"].([]any); ok {
		result := make([]map[string]any, 0, len(data))
		for _, value := range data {
			if item, ok := value.(map[string]any); ok {
				result = append(result, item)
			}
		}
		return result
	}
	if data, ok := response["data"].(map[string]any); ok {
		return []map[string]any{data}
	}
	return []map[string]any{response}
}

func firstString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func normalizeStatus(response map[string]any) string {
	return strings.ToLower(firstString(response, "status", "task_status", "state"))
}

func responseMessage(response map[string]any, fallback string) string {
	if value := firstString(response, "message", "error_msg", "error_message"); value != "" {
		return value
	}
	if nested, ok := response["error"].(map[string]any); ok {
		if value := firstString(nested, "message", "message_zh", "code"); value != "" {
			return value
		}
	}
	if data, err := json.Marshal(response); err == nil && len(data) > 2 {
		return fallback + ": " + adapterutil.Truncate(string(data), 600)
	}
	return fallback
}
