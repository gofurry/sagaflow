package minimax

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	const provider = "minimax"
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	payload := map[string]any{
		"model": request.Target.ID, "prompt": request.Prompt,
		"response_format": adapterutil.StringParam(p, "response_format", "url"),
		"n":               adapterutil.NumberParam(p, "n", 1),
	}
	_, hasWidth := p["width"]
	_, hasHeight := p["height"]
	if hasWidth != hasHeight {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax custom image size requires both width and height", false, nil)
	}
	if !hasWidth {
		payload["aspect_ratio"] = adapterutil.StringParam(p, "aspect_ratio", "16:9")
	}
	for _, key := range []string{"width", "height", "style", "seed", "prompt_optimizer", "aigc_watermark"} {
		adapterutil.CopyParam(payload, p, key)
	}
	if len(request.Inputs) > 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax image generation accepts one character reference", false, nil)
	}
	if len(request.Inputs) == 1 {
		input := request.Inputs[0]
		if input.MediaType != "image" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax image reference must be an image", false, nil)
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return inference.Result{}, err
		}
		payload["subject_reference"] = []map[string]any{{"type": "character", "image_file": input.URL}}
	}
	endpoint := strings.TrimRight(request.Runtime.Endpoint, "/") + "/v1/image_generation"
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "MiniMax image request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	}); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		ID   string `json:"id"`
		Data struct {
			ImageURLs []string `json:"image_urls"`
		} `json:"data"`
		Metadata any          `json:"metadata"`
		BaseResp baseResponse `json:"base_resp"`
	}
	raw, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if err := response.BaseResp.Error(provider); err != nil {
		return inference.Result{}, err
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching"}); err != nil {
		return inference.Result{}, err
	}
	artifacts := make([]inference.Artifact, 0, len(response.Data.ImageURLs))
	for _, value := range response.Data.ImageURLs {
		artifact, ok, artifactErr := d.imageArtifact(value, adapterutil.JSONSummary(raw, "id", "metadata"))
		if artifactErr != nil {
			return inference.Result{}, artifactErr
		}
		if ok {
			artifacts = append(artifacts, artifact)
		}
	}
	if len(artifacts) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "image response is empty", false, nil)
	}
	return inference.Result{Artifacts: artifacts}, nil
}

func (d *Driver) imageArtifact(value string, metadata []byte) (inference.Artifact, bool, error) {
	const provider = "minimax"
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return inference.Artifact{
			MediaType: "image", MIMEType: "image/png", SourceURL: value, Metadata: metadata,
			Content: d.http.URLContent(provider, value),
		}, true, nil
	}
	if index := strings.Index(value, ","); strings.HasPrefix(value, "data:") && index >= 0 {
		value = value[index+1:]
	}
	if value == "" {
		return inference.Artifact{}, false, nil
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return inference.Artifact{}, false, inference.NewError(inference.ErrorInvalidOutput, provider, "decode image response", false, err)
	}
	return inference.Artifact{
		MediaType: "image", MIMEType: "image/png", Metadata: metadata,
		Content: inference.BytesContent(data, "image/png"),
	}, true, nil
}
