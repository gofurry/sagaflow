package tencenttokenhub

import (
	"context"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateVideo(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	providerName := provider(request)
	inputs, err := materializeInputs(ctx, request, "image")
	if err != nil {
		return inference.Result{}, err
	}
	isYT := strings.EqualFold(request.Target.ID, "yt-video-2.0")
	if isYT && len(inputs) != 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "YT Video 2 requires exactly one image reference", false, nil)
	}
	if isYT && inputs[0].URL == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "TokenHub YT Video 2 references must be manually published to S3 first", false, nil)
	}
	if !isYT && len(inputs) > 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Hunyuan Video accepts at most one image reference", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"resolution", "fps"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	trace := cloneMap(payload)
	if len(inputs) == 1 {
		image := map[string]any{}
		traceImage := map[string]any{}
		if inputs[0].URL != "" {
			image["url"] = inputs[0].URL
		} else {
			image["base64"] = inputs[0].Base64
		}
		traceImage["url"] = inputs[0].traceValue()
		payload["image"] = image
		trace["image"] = traceImage
	}
	runID := strings.TrimSpace(request.ProviderRunID)
	if runID == "" {
		submitURL := endpoint(request.Runtime.Endpoint, "/api/video/submit")
		if err := emitRequest(ctx, events, submitURL, trace); err != nil {
			return inference.Result{}, err
		}
		var submitted map[string]any
		if _, err := d.http.DoJSON(ctx, providerName, http.MethodPost, submitURL, request.Runtime.APIKey, payload, &submitted); err != nil {
			return inference.Result{}, err
		}
		runID = firstString(submitted, "id", "task_id", "request_id")
		if runID == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "TokenHub video response has no task ID", false, nil)
		}
		if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .3, ProviderRunID: runID, Message: "TokenHub video queued"}); err != nil {
			return inference.Result{}, err
		}
	}
	queryURL := endpoint(request.Runtime.Endpoint, "/api/video/query")
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
			videoURL := firstString(response, "url", "video_url", "result_video")
			if videoURL == "" {
				for _, item := range artifactItems(response) {
					if videoURL = firstString(item, "url", "video_url", "result_video"); videoURL != "" {
						break
					}
				}
			}
			if videoURL == "" {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "completed TokenHub video has no URL", false, nil)
			}
			if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", Progress: .9, ProviderRunID: runID}); err != nil {
				return inference.Result{}, err
			}
			return inference.Result{Artifacts: []inference.Artifact{{
				MediaType: "video", MIMEType: "video/mp4", SourceURL: videoURL,
				Metadata: adapterutil.JSONSummary(raw, "id", "task_id", "status", "data"),
				Content:  d.http.URLContent(providerName, videoURL),
			}}}, nil
		case "failed", "fail", "cancelled":
			message := responseMessage(response, "TokenHub video generation failed")
			if strings.Contains(message, "任务不存在") && notFoundRetries < 3 {
				notFoundRetries++
				continue
			}
			return inference.Result{}, inference.NewError(inference.ErrorProvider, providerName, message, false, nil)
		default:
			if err := inference.Emit(ctx, events, inference.Event{Stage: "running", Progress: .55, ProviderRunID: runID, Message: "TokenHub video generating"}); err != nil {
				return inference.Result{}, err
			}
		}
	}
}
