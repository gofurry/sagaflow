package tencenttokenhub

import (
	"context"
	"net/http"
	"strconv"
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
	isKling := strings.HasPrefix(strings.ToLower(request.Target.ID), "kl-video-")
	isVidu := strings.HasPrefix(strings.ToLower(request.Target.ID), "vd-video-")
	if isYT && len(inputs) != 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "YT Video 2 requires exactly one image reference", false, nil)
	}
	if isYT && inputs[0].URL == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "TokenHub YT Video 2 references must be manually published to S3 first", false, nil)
	}
	if !isYT && !isKling && !isVidu && len(inputs) > 1 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "Hunyuan Video accepts at most one image reference", false, nil)
	}
	if (isKling || isVidu) && len(inputs) > 2 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerName, "selected video model accepts at most two image references", false, nil)
	}
	payload := map[string]any{"model": request.Target.ID, "prompt": request.Prompt}
	switch {
	case isKling:
		for _, key := range []string{"negative_prompt", "mode", "cfg_scale"} {
			adapterutil.CopyParam(payload, request.Parameters, key)
		}
		if duration := int(adapterutil.NumberParam(request.Parameters, "duration", 5)); duration > 0 {
			payload["duration"] = strconv.Itoa(duration)
		}
		if len(inputs) == 0 {
			adapterutil.CopyParam(payload, request.Parameters, "aspect_ratio")
		} else {
			adapterutil.CopyParam(payload, request.Parameters, "sound")
		}
	case isVidu:
		for _, key := range []string{"duration", "resolution", "seed", "logo_add", "off_peak"} {
			adapterutil.CopyParam(payload, request.Parameters, key)
		}
		if len(inputs) == 0 {
			for _, key := range []string{"aspect_ratio", "bgm"} {
				adapterutil.CopyParam(payload, request.Parameters, key)
			}
		} else {
			for _, key := range []string{"audio", "is_rec"} {
				adapterutil.CopyParam(payload, request.Parameters, key)
			}
			if len(inputs) == 1 {
				adapterutil.CopyParam(payload, request.Parameters, "voice_id")
			}
		}
	default:
		for _, key := range []string{"resolution", "fps"} {
			adapterutil.CopyParam(payload, request.Parameters, key)
		}
	}
	trace := cloneMap(payload)
	if isKling && len(inputs) > 0 {
		payload["image"] = inputValue(inputs[0])
		trace["image"] = inputs[0].traceValue()
		if len(inputs) == 2 {
			payload["image_tail"] = inputValue(inputs[1])
			trace["image_tail"] = inputs[1].traceValue()
		}
	} else if isVidu && len(inputs) > 0 {
		images := make([]string, 0, len(inputs))
		traceImages := make([]string, 0, len(inputs))
		for _, input := range inputs {
			images = append(images, inputValue(input))
			traceImages = append(traceImages, input.traceValue())
		}
		payload["images"] = images
		trace["images"] = traceImages
	} else if len(inputs) == 1 {
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

func inputValue(input materializedInput) string {
	if input.URL != "" {
		return input.URL
	}
	return input.Base64
}
