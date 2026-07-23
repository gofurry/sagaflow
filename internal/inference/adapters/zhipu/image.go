package zhipu

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", providerName); err != nil {
		return inference.Result{}, err
	}
	if len(request.Inputs) != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Zhipu image generation does not accept reference assets", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"quality", "size", "user_id", "watermark_enabled"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	url := endpoint(request.Runtime.Endpoint, "/images/generations")
	if err := emitRequest(ctx, events, url, payload); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		Data []struct {
			URL string `json:"url"`
			B64 string `json:"b64_json"`
		} `json:"data"`
		Created any `json:"created"`
	}
	raw, err := d.http.DoJSON(ctx, providerName, http.MethodPost, url, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if len(response.Data) == 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Zhipu image response has no image", false, nil)
	}
	artifacts := make([]inference.Artifact, 0, len(response.Data))
	for _, item := range response.Data {
		artifact := inference.Artifact{
			MediaType: "image", MIMEType: "image/png",
			Metadata: adapterutil.JSONSummary(raw, "created", "content_filter"),
		}
		switch {
		case strings.TrimSpace(item.URL) != "":
			artifact.SourceURL = item.URL
			artifact.Content = d.http.URLContent(providerName, item.URL)
		case strings.TrimSpace(item.B64) != "":
			data, decodeErr := base64.StdEncoding.DecodeString(item.B64)
			if decodeErr != nil {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "decode Zhipu image", false, decodeErr)
			}
			artifact.Content = inference.BytesContent(data, "image/png")
		default:
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Zhipu image item is empty", false, nil)
		}
		artifacts = append(artifacts, artifact)
	}
	return inference.Result{Artifacts: artifacts, Usage: map[string]any{"created": response.Created}}, nil
}
