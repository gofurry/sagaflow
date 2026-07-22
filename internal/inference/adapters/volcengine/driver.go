package volcengine

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type Config struct {
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Driver struct {
	http         *adapterutil.HTTPClient
	text         *openaicompat.Driver
	chat         *openaicompat.Driver
	pollInterval time.Duration
}

func New(config Config) *Driver {
	interval := config.PollInterval
	if interval <= 0 {
		interval = 4 * time.Second
	}
	return &Driver{
		http:         adapterutil.NewHTTPClient(config.HTTPClient),
		text:         openaicompat.NewResponses(config.HTTPClient),
		chat:         openaicompat.NewChat(config.HTTPClient),
		pollInterval: interval,
	}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if request.Target.Kind != inference.TargetModel {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, request.Runtime.ProviderCode, "Volcengine adapter only accepts model targets", false, nil)
	}
	switch request.Target.Capability {
	case inference.CapabilityText:
		if request.Target.ID == "doubao-seed-evolving" {
			return d.chat.Execute(ctx, request, events)
		}
		return d.text.Execute(ctx, request, events)
	case inference.CapabilityImage:
		return d.generateImage(ctx, request, events)
	case inference.CapabilityVideo:
		return d.generateVideo(ctx, request, events)
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, request.Runtime.ProviderCode, "driver only supports text, image and video generation", false, nil)
	}
}

func (d *Driver) generateImage(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt, "response_format": "url", "stream": false, "sequential_image_generation": "disabled"}
	for _, key := range []string{"size", "seed", "guidance_scale", "watermark", "response_format", "sequential_image_generation"} {
		adapterutil.CopyParam(payload, p, key)
	}
	if maxImages := adapterutil.NumberParam(p, "max_images", 1); adapterutil.StringParam(p, "sequential_image_generation", "disabled") == "auto" {
		payload["sequential_image_generation_options"] = map[string]any{"max_images": maxImages}
	}
	urls := make([]string, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		if input.MediaType != "image" {
			continue
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return inference.Result{}, err
		}
		urls = append(urls, input.URL)
	}
	if len(urls) == 1 {
		payload["image"] = urls[0]
	} else if len(urls) > 1 {
		payload["image"] = urls
	}
	endpoint := strings.TrimRight(request.Runtime.Endpoint, "/") + "/images/generations"
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Volcengine image request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	}); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		Data []struct {
			URL           string `json:"url"`
			B64           string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
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
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching"}); err != nil {
		return inference.Result{}, err
	}
	artifacts := make([]inference.Artifact, 0, len(response.Data))
	for _, item := range response.Data {
		artifact := inference.Artifact{
			MediaType: "image", MIMEType: "image/png", SourceURL: item.URL,
			Metadata: adapterutil.JSON(map[string]any{"revised_prompt": item.RevisedPrompt, "provider_response": adapterutil.JSONSummary(raw)}),
		}
		switch {
		case item.B64 != "":
			data, decodeErr := base64.StdEncoding.DecodeString(item.B64)
			if decodeErr != nil {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "decode image response", false, decodeErr)
			}
			artifact.Content = inference.BytesContent(data, artifact.MIMEType)
		case item.URL != "":
			artifact.Content = d.http.URLContent(provider, item.URL)
		default:
			continue
		}
		artifacts = append(artifacts, artifact)
	}
	return inference.Result{Artifacts: artifacts}, nil
}

func (d *Driver) generateVideo(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	content := []map[string]any{{"type": "text", "text": request.Prompt}}
	for _, input := range request.Inputs {
		if err := adapterutil.Required(input.URL, "provider-reachable reference", provider); err != nil {
			return inference.Result{}, err
		}
		switch input.MediaType {
		case "image":
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": input.URL}, "role": "reference_image"})
		case "video":
			content = append(content, map[string]any{"type": "video_url", "video_url": map[string]string{"url": input.URL}, "role": "reference_video"})
		case "audio":
			content = append(content, map[string]any{"type": "audio_url", "audio_url": map[string]string{"url": input.URL}, "role": "reference_audio"})
		}
	}
	payload := map[string]any{
		"model": request.Target.ID, "content": content,
		"generate_audio": adapterutil.BoolParam(p, "generate_audio", true),
		"ratio":          adapterutil.StringParam(p, "ratio", "16:9"),
		"duration":       adapterutil.NumberParam(p, "duration", 5),
		"watermark":      adapterutil.BoolParam(p, "watermark", false),
	}
	adapterutil.CopyParam(payload, p, "resolution")
	endpoint := strings.TrimRight(request.Runtime.Endpoint, "/") + "/contents/generations/tasks"
	externalID := strings.TrimSpace(request.ProviderRunID)
	if externalID == "" {
		if err := inference.Emit(ctx, events, inference.Event{
			Stage: "provider_request", Progress: .2, Message: "Volcengine video request payload prepared",
			Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
		}); err != nil {
			return inference.Result{}, err
		}
		var submitted map[string]any
		if _, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &submitted); err != nil {
			return inference.Result{}, err
		}
		externalID = adapterutil.FindString(submitted, "id", "task_id")
		if externalID == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "video response has no task id", false, nil)
		}
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", ProviderRunID: externalID}); err != nil {
		return inference.Result{}, err
	}
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return inference.Result{}, ctx.Err()
		case <-ticker.C:
			var polled map[string]any
			raw, err := d.http.DoJSON(ctx, provider, http.MethodGet, endpoint+"/"+externalID, request.Runtime.APIKey, nil, &polled)
			if err != nil {
				return inference.Result{}, err
			}
			status := strings.ToLower(adapterutil.FindString(polled, "status"))
			switch status {
			case "failed", "error":
				return inference.Result{}, inference.NewError(inference.ErrorProvider, provider, "video task failed", false, nil)
			case "succeeded", "completed", "success":
				url := adapterutil.FindMediaURL(polled)
				if url == "" {
					return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "completed video has no URL", false, nil)
				}
				if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", ProviderRunID: externalID}); err != nil {
					return inference.Result{}, err
				}
				return inference.Result{Artifacts: []inference.Artifact{{
					MediaType: "video", MIMEType: "video/mp4", SourceURL: url,
					Metadata: adapterutil.JSONSummary(raw), Content: d.http.URLContent(provider, url),
				}}}, nil
			default:
				if err := inference.Emit(ctx, events, inference.Event{Stage: "running", ProviderRunID: externalID}); err != nil {
					return inference.Result{}, err
				}
			}
		}
	}
}
