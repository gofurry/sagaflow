package bailian

import (
	"context"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	if request.Target.ID == "wanx2.1-imageedit" {
		return d.editImage(ctx, request, events)
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	content := make([]map[string]any, 0, len(request.Inputs)+1)
	for _, input := range request.Inputs {
		if input.MediaType != "image" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "Bailian image references must be images", false, nil)
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return inference.Result{}, err
		}
		content = append(content, map[string]any{"image": input.URL})
	}
	if len(request.Inputs) > 9 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "Bailian image generation accepts at most 9 references", false, nil)
	}
	content = append(content, map[string]any{"text": request.Prompt})
	parameters := map[string]any{
		"size":          adapterutil.StringParam(p, "size", "2K"),
		"n":             int(adapterutil.NumberParam(p, "n", 1)),
		"watermark":     adapterutil.BoolParam(p, "watermark", false),
		"thinking_mode": adapterutil.BoolParam(p, "thinking_mode", true),
	}
	for _, key := range []string{"seed"} {
		adapterutil.CopyParam(parameters, p, key)
	}
	payload := map[string]any{
		"model":      request.Target.ID,
		"input":      map[string]any{"messages": []map[string]any{{"role": "user", "content": content}}},
		"parameters": parameters,
	}
	taskID, err := d.submitTask(ctx, request, events, "/api/v1/services/aigc/image-generation/generation", payload)
	if err != nil {
		return inference.Result{}, err
	}
	response, raw, err := d.waitTask(ctx, request, events, taskID)
	if err != nil {
		return inference.Result{}, err
	}
	urls := imageURLs(response)
	if len(urls) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "completed Bailian image task has no image URL", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", ProviderRunID: taskID}); err != nil {
		return inference.Result{}, err
	}
	artifacts := make([]inference.Artifact, 0, len(urls))
	for _, url := range urls {
		artifacts = append(artifacts, inference.Artifact{
			MediaType: "image", MIMEType: "image/png", SourceURL: url,
			Metadata: adapterutil.JSONSummary(raw, "request_id", "usage"), Content: d.http.URLContent(provider, url),
		})
	}
	return inference.Result{Artifacts: artifacts, Usage: usage(response)}, nil
}

func imageURLs(value any) []string {
	urls := make([]string, 0)
	seen := make(map[string]struct{})
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for _, key := range []string{"image", "url", "image_url"} {
				if raw, ok := typed[key].(string); ok && (strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
					if _, exists := seen[raw]; !exists {
						seen[raw] = struct{}{}
						urls = append(urls, raw)
					}
				}
			}
			for key, child := range typed {
				if key != "image" && key != "url" && key != "image_url" {
					walk(child)
				}
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return urls
}

func usage(response map[string]any) map[string]any {
	value, _ := response["usage"].(map[string]any)
	if value == nil {
		if output, ok := response["output"].(map[string]any); ok {
			value, _ = output["usage"].(map[string]any)
		}
	}
	return value
}
