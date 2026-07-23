package zhipu

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
	updated, err := d.materializeReferences(ctx, request, "image")
	if err != nil {
		return inference.Result{}, err
	}
	task := modelTask(request)
	if err := validateVideoReferences(task, request.Target.ID, updated.Inputs); err != nil {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, err.Error(), false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	for _, key := range []string{"quality", "with_audio", "watermark_enabled", "size", "fps", "duration", "aspect_ratio", "movement_amplitude", "request_id", "user_id"} {
		adapterutil.CopyParam(payload, request.Parameters, key)
	}
	if len(updated.Inputs) > 0 {
		values := make([]string, 0, len(updated.Inputs))
		for _, input := range updated.Inputs {
			values = append(values, input.URL)
		}
		if len(values) == 1 {
			payload["image_url"] = values[0]
		} else {
			payload["image_url"] = values
		}
	}
	trace := cloneMap(payload)
	if values, ok := payload["image_url"].([]string); ok {
		traced := make([]string, len(values))
		for index, value := range values {
			traced[index] = traceReference(value)
		}
		trace["image_url"] = traced
	} else if value, ok := payload["image_url"].(string); ok {
		trace["image_url"] = traceReference(value)
	}
	runID := strings.TrimSpace(request.ProviderRunID)
	if runID == "" {
		submitURL := endpoint(request.Runtime.Endpoint, "/videos/generations")
		if err := emitRequest(ctx, events, submitURL, trace); err != nil {
			return inference.Result{}, err
		}
		var submitted struct {
			ID     string `json:"id"`
			Status string `json:"task_status"`
		}
		if _, err := d.http.DoJSON(ctx, providerName, http.MethodPost, submitURL, request.Runtime.APIKey, payload, &submitted); err != nil {
			return inference.Result{}, err
		}
		runID = strings.TrimSpace(submitted.ID)
		if runID == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "Zhipu video response has no task ID", false, nil)
		}
		if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", Progress: .3, ProviderRunID: runID, Message: "Zhipu video queued"}); err != nil {
			return inference.Result{}, err
		}
	}
	statusURL := endpoint(request.Runtime.Endpoint, "/async-result/"+runID)
	for {
		if err := wait(ctx, d.pollInterval); err != nil {
			return inference.Result{}, err
		}
		var status struct {
			ID          string `json:"id"`
			Status      string `json:"task_status"`
			Error       any    `json:"error"`
			VideoResult []struct {
				URL      string `json:"url"`
				CoverURL string `json:"cover_image_url"`
			} `json:"video_result"`
		}
		raw, err := d.http.DoJSON(ctx, providerName, http.MethodGet, statusURL, request.Runtime.APIKey, nil, &status)
		if err != nil {
			return inference.Result{}, err
		}
		switch strings.ToLower(status.Status) {
		case "success", "succeeded":
			if len(status.VideoResult) == 0 || strings.TrimSpace(status.VideoResult[0].URL) == "" {
				return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, providerName, "completed Zhipu video has no URL", false, nil)
			}
			videoURL := status.VideoResult[0].URL
			if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", Progress: .9, ProviderRunID: runID}); err != nil {
				return inference.Result{}, err
			}
			return inference.Result{Artifacts: []inference.Artifact{{
				MediaType: "video", MIMEType: "video/mp4", SourceURL: videoURL,
				Metadata: adapterutil.JSONSummary(raw, "id", "task_status", "video_result"),
				Content:  d.http.URLContent(providerName, videoURL),
			}}}, nil
		case "fail", "failed":
			return inference.Result{}, inference.NewError(inference.ErrorProvider, providerName, "Zhipu video generation failed", false, nil)
		default:
			if err := inference.Emit(ctx, events, inference.Event{Stage: "running", Progress: .55, ProviderRunID: runID, Message: "Zhipu video generating"}); err != nil {
				return inference.Result{}, err
			}
		}
	}
}

func validateVideoReferences(task, modelID string, inputs []inference.Input) error {
	switch task {
	case "text_to_video":
		if len(inputs) != 0 {
			return fmtError("text-to-video does not accept image references")
		}
	case "image_to_video":
		if len(inputs) != 1 {
			return fmtError("image-to-video requires one image reference")
		}
	case "start_end_video":
		if len(inputs) != 2 {
			return fmtError("start/end video requires exactly two image references")
		}
	case "reference_to_video":
		if len(inputs) < 1 || len(inputs) > 3 {
			return fmtError("reference video requires one to three image references")
		}
		for _, input := range inputs {
			if strings.HasPrefix(input.URL, "data:") {
				return fmtError("Vidu reference video requires S3-published image URLs")
			}
		}
	default:
		if strings.HasPrefix(strings.ToLower(modelID), "cogvideox") && len(inputs) > 2 {
			return fmtError("CogVideoX accepts at most two image references")
		}
	}
	return nil
}

type validationError string

func (e validationError) Error() string { return string(e) }
func fmtError(message string) error     { return validationError("Zhipu " + message) }

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
