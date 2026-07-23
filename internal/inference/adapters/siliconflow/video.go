package siliconflow

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateVideo(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", providerName); err != nil {
		return inference.Result{}, err
	}
	updated, err := d.materializeReferences(ctx, request, "image")
	if err != nil {
		return inference.Result{}, err
	}
	task := modelTask(request)
	if task == "text_to_video" && len(updated.Inputs) != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "SiliconFlow text-to-video does not accept references", false, nil)
	}
	if task == "image_to_video" && len(updated.Inputs) != 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "SiliconFlow image-to-video requires one image reference", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"image_size", "negative_prompt", "seed"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	trace := cloneMap(payload)
	if len(updated.Inputs) == 1 {
		payload["image"] = updated.Inputs[0].URL
		trace["image"] = traceReference(updated.Inputs[0].URL)
	}
	submitURL := endpoint(request.Runtime.Endpoint, "/video/submit")
	if err := emitRequest(ctx, events, submitURL, trace); err != nil {
		return inference.Result{}, err
	}
	var submitted struct {
		RequestID string `json:"requestId"`
	}
	if _, err := d.http.DoJSON(ctx, providerName, http.MethodPost, submitURL, request.Runtime.APIKey, payload, &submitted); err != nil {
		return inference.Result{}, err
	}
	if strings.TrimSpace(submitted.RequestID) == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "SiliconFlow video response has no requestId", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .3, ProviderRunID: submitted.RequestID, Message: "SiliconFlow video queued"}); err != nil {
		return inference.Result{}, err
	}
	statusURL := endpoint(request.Runtime.Endpoint, "/video/status")
	for {
		if err := wait(ctx, d.pollInterval); err != nil {
			return inference.Result{}, err
		}
		var status struct {
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Results struct {
				Videos []struct {
					URL string `json:"url"`
				} `json:"videos"`
				Timings map[string]any `json:"timings"`
				Seed    any            `json:"seed"`
			} `json:"results"`
		}
		raw, err := d.http.DoJSON(ctx, providerName, http.MethodPost, statusURL, request.Runtime.APIKey, map[string]any{"requestId": submitted.RequestID}, &status)
		if err != nil {
			return inference.Result{}, err
		}
		switch strings.ToLower(status.Status) {
		case "succeed":
			if len(status.Results.Videos) == 0 || strings.TrimSpace(status.Results.Videos[0].URL) == "" {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "completed SiliconFlow video has no URL", false, nil)
			}
			videoURL := status.Results.Videos[0].URL
			if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", Progress: .9, ProviderRunID: submitted.RequestID}); err != nil {
				return inference.Result{}, err
			}
			return inference.Result{
				Artifacts: []inference.Artifact{{
					MediaType: "video", MIMEType: "video/mp4", SourceURL: videoURL,
					Metadata: adapterutil.JSONSummary(raw, "status", "reason", "results"),
					Content:  d.http.URLContent(providerName, videoURL),
				}},
				Usage: map[string]any{"timings": status.Results.Timings, "seed": status.Results.Seed},
			}, nil
		case "failed":
			message := strings.TrimSpace(status.Reason)
			if message == "" {
				message = "SiliconFlow video generation failed"
			}
			return inference.Result{}, inference.NewError(inference.ErrorProvider, providerName, message, false, nil)
		default:
			if err := inference.Emit(ctx, events, inference.Event{
				Stage: "running", Progress: .55, ProviderRunID: submitted.RequestID, Message: "SiliconFlow video generating",
			}); err != nil {
				return inference.Result{}, err
			}
		}
	}
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
