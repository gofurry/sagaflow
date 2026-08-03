package minimax

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateVideo(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	const provider = "minimax"
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	baseURL := strings.TrimRight(request.Runtime.Endpoint, "/")
	taskID := strings.TrimSpace(request.ProviderRunID)
	if taskID == "" {
		payload, err := miniMaxVideoPayload(request)
		if err != nil {
			return inference.Result{}, err
		}
		endpoint := baseURL + "/v1/video_generation"
		if err := inference.Emit(ctx, events, inference.Event{
			Stage: "provider_request", Progress: .2, Message: "MiniMax video request payload prepared",
			Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
		}); err != nil {
			return inference.Result{}, err
		}
		var submitted struct {
			TaskID   string       `json:"task_id"`
			BaseResp baseResponse `json:"base_resp"`
		}
		if _, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &submitted); err != nil {
			return inference.Result{}, err
		}
		if err := submitted.BaseResp.Error(provider); err != nil {
			return inference.Result{}, err
		}
		taskID = strings.TrimSpace(submitted.TaskID)
		if taskID == "" {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "video response has no task id", false, nil)
		}
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "queued", ProviderRunID: taskID}); err != nil {
		return inference.Result{}, err
	}
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return inference.Result{}, ctx.Err()
		case <-ticker.C:
			fileID, done, err := d.pollVideo(ctx, request.Runtime.APIKey, baseURL, taskID)
			if err != nil {
				return inference.Result{}, err
			}
			if !done {
				if err := inference.Emit(ctx, events, inference.Event{Stage: "running", ProviderRunID: taskID}); err != nil {
					return inference.Result{}, err
				}
				continue
			}
			return d.retrieveVideo(ctx, request.Runtime.APIKey, baseURL, taskID, fileID, events)
		}
	}
}

func miniMaxVideoPayload(request inference.Request) (map[string]any, error) {
	const provider = "minimax"
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	payload := map[string]any{
		"model": request.Target.ID, "prompt": request.Prompt,
		"duration":   adapterutil.NumberParam(p, "duration", 6),
		"resolution": adapterutil.StringParam(p, "resolution", "1080P"),
	}
	if payload["resolution"] == "1080P" && payload["duration"].(float64) != 6 {
		return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax 1080P video only supports 6 seconds", false, nil)
	}
	if strings.Contains(strings.ToLower(request.Target.ID), "fast") && adapterutil.StringParam(p, "reference_mode", "first_frame") != "first_frame" {
		return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax Fast only supports first-frame image-to-video", false, nil)
	}
	adapterutil.CopyParam(payload, p, "prompt_optimizer")
	images := make([]string, 0, len(request.Inputs))
	for _, input := range request.Inputs {
		if input.MediaType != "image" {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax video references must be images", false, nil)
		}
		if err := adapterutil.Required(input.URL, "provider-reachable image reference", provider); err != nil {
			return nil, err
		}
		images = append(images, input.URL)
	}
	mode := adapterutil.StringParam(p, "reference_mode", "first_frame")
	switch mode {
	case "first_frame":
		if len(images) > 1 {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "first-frame mode accepts one image", false, nil)
		}
		if len(images) == 1 {
			payload["first_frame_image"] = images[0]
		}
	case "first_last_frame":
		if len(images) != 2 {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "first/last-frame mode requires two images", false, nil)
		}
		payload["first_frame_image"], payload["last_frame_image"] = images[0], images[1]
	case "subject":
		if len(images) == 0 {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "subject-reference mode requires at least one image", false, nil)
		}
		payload["subject_reference"] = []map[string]any{{"type": "character", "image": images}}
	default:
		return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "unknown MiniMax video reference mode", false, nil)
	}
	return payload, nil
}

func (d *Driver) pollVideo(ctx context.Context, key, baseURL, taskID string) (string, bool, error) {
	const provider = "minimax"
	endpoint := baseURL + "/v1/query/video_generation?task_id=" + url.QueryEscape(taskID)
	var response struct {
		Status   string       `json:"status"`
		FileID   string       `json:"file_id"`
		BaseResp baseResponse `json:"base_resp"`
	}
	if _, err := d.http.DoJSON(ctx, provider, http.MethodGet, endpoint, key, nil, &response); err != nil {
		return "", false, err
	}
	if err := response.BaseResp.Error(provider); err != nil {
		return "", false, err
	}
	switch strings.ToLower(response.Status) {
	case "success", "succeeded", "completed":
		if strings.TrimSpace(response.FileID) == "" {
			return "", false, inference.NewError(inference.ErrorInvalidOutput, provider, "completed video has no file id", false, nil)
		}
		return response.FileID, true, nil
	case "fail", "failed", "error":
		return "", false, inference.NewError(inference.ErrorProvider, provider, "video task failed", false, nil)
	default:
		return "", false, nil
	}
}

func (d *Driver) retrieveVideo(ctx context.Context, key, baseURL, taskID, fileID string, events inference.EventSink) (inference.Result, error) {
	const provider = "minimax"
	endpoint := baseURL + "/v1/files/retrieve?file_id=" + url.QueryEscape(fileID)
	var response struct {
		File struct {
			DownloadURL string `json:"download_url"`
			Filename    string `json:"filename"`
		} `json:"file"`
		BaseResp baseResponse `json:"base_resp"`
	}
	raw, err := d.http.DoJSON(ctx, provider, http.MethodGet, endpoint, key, nil, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if err := response.BaseResp.Error(provider); err != nil {
		return inference.Result{}, err
	}
	downloadURL := strings.TrimSpace(response.File.DownloadURL)
	if downloadURL == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "video file has no download URL", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", ProviderRunID: taskID}); err != nil {
		return inference.Result{}, err
	}
	return inference.Result{Artifacts: []inference.Artifact{{
		MediaType: "video", MIMEType: "video/mp4", SourceURL: downloadURL,
		Metadata: adapterutil.JSONSummary(raw, "file", "base_resp"), Content: d.http.URLContent(provider, downloadURL),
	}}}, nil
}
