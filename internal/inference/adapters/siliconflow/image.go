package siliconflow

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", providerName); err != nil {
		return inference.Result{}, err
	}
	updated, err := d.materializeReferences(ctx, request, "image")
	if err != nil {
		return inference.Result{}, err
	}
	if len(updated.Inputs) > 3 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "SiliconFlow image editing accepts at most 3 references", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"image_size", "negative_prompt", "batch_size", "seed", "num_inference_steps", "guidance_scale"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	trace := cloneMap(payload)
	for index, input := range updated.Inputs {
		key := "image"
		if index > 0 {
			key += string(rune('1' + index))
		}
		payload[key] = input.URL
		trace[key] = traceReference(input.URL)
	}
	url := endpoint(request.Runtime.Endpoint, "/images/generations")
	if err := emitRequest(ctx, events, url, trace); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		Images []struct {
			URL string `json:"url"`
			B64 string `json:"b64_json"`
		} `json:"images"`
		Timings map[string]any `json:"timings"`
		Seed    any            `json:"seed"`
	}
	raw, err := d.http.DoJSON(ctx, providerName, "POST", url, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if len(response.Images) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "SiliconFlow image response has no images", false, nil)
	}
	artifacts := make([]inference.Artifact, 0, len(response.Images))
	for _, image := range response.Images {
		artifact := inference.Artifact{
			MediaType: "image", MIMEType: "image/png",
			Metadata: adapterutil.JSONSummary(raw, "timings", "seed"),
		}
		switch {
		case strings.TrimSpace(image.URL) != "":
			artifact.SourceURL = image.URL
			artifact.Content = d.http.URLContent(providerName, image.URL)
		case strings.TrimSpace(image.B64) != "":
			data, decodeErr := base64.StdEncoding.DecodeString(image.B64)
			if decodeErr != nil {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "decode SiliconFlow image", false, decodeErr)
			}
			artifact.Content = inference.BytesContent(data, "image/png")
		default:
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "SiliconFlow image item is empty", false, nil)
		}
		artifacts = append(artifacts, artifact)
	}
	return inference.Result{Artifacts: artifacts, Usage: map[string]any{"timings": response.Timings, "seed": response.Seed}}, nil
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
