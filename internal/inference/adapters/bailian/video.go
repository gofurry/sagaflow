package bailian

import (
	"context"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateVideo(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	input, err := videoInput(provider, request)
	if err != nil {
		return inference.Result{}, err
	}
	parameters := map[string]any{
		"resolution": adapterutil.StringParam(p, "resolution", "720P"),
		"duration":   int(adapterutil.NumberParam(p, "duration", 5)),
		"watermark":  adapterutil.BoolParam(p, "watermark", false),
	}
	for _, key := range []string{"ratio", "seed", "prompt_extend", "shot_type"} {
		adapterutil.CopyParam(parameters, p, key)
	}
	if negativePrompt := strings.TrimSpace(adapterutil.StringParam(p, "negative_prompt", "")); negativePrompt != "" {
		input["negative_prompt"] = negativePrompt
	}
	payload := map[string]any{"model": request.Target.ID, "input": input, "parameters": parameters}
	taskID, err := d.submitTask(ctx, request, events, "/api/v1/services/aigc/video-generation/video-synthesis", payload)
	if err != nil {
		return inference.Result{}, err
	}
	response, raw, err := d.waitTask(ctx, request, events, taskID)
	if err != nil {
		return inference.Result{}, err
	}
	videoURL := adapterutil.FindMediaURL(response)
	if videoURL == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "completed Bailian video task has no video URL", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching", ProviderRunID: taskID}); err != nil {
		return inference.Result{}, err
	}
	return inference.Result{Artifacts: []inference.Artifact{{
		MediaType: "video", MIMEType: "video/mp4", SourceURL: videoURL,
		Metadata: adapterutil.JSONSummary(raw, "request_id", "usage"), Content: d.http.URLContent(provider, videoURL),
	}}, Usage: usage(response)}, nil
}

func videoInput(provider string, request inference.Request) (map[string]any, error) {
	modelID := strings.ToLower(request.Target.ID)
	input := map[string]any{"prompt": request.Prompt}
	if strings.Contains(modelID, "t2v") {
		if modelID == "wan2.7-t2v" {
			if len(request.Inputs) > 1 || (len(request.Inputs) == 1 && request.Inputs[0].MediaType != "audio") {
				return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "Wan 2.7 text-to-video accepts at most one audio reference", false, nil)
			}
			if len(request.Inputs) == 1 {
				if err := adapterutil.Required(request.Inputs[0].URL, "provider-reachable audio reference", provider); err != nil {
					return nil, err
				}
				input["audio_url"] = request.Inputs[0].URL
			}
			return input, nil
		}
		if len(request.Inputs) != 0 {
			return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "selected text-to-video model does not accept references", false, nil)
		}
		return input, nil
	}
	media := make([]map[string]any, 0, len(request.Inputs))
	for index, reference := range request.Inputs {
		if err := adapterutil.Required(reference.URL, "provider-reachable video reference", provider); err != nil {
			return nil, err
		}
		typeName := "reference_" + reference.MediaType
		if strings.Contains(modelID, "i2v") && reference.MediaType == "image" {
			if index == 0 {
				typeName = "first_frame"
			} else if index == 1 {
				typeName = "last_frame"
			}
		}
		if reference.MediaType == "audio" {
			typeName = "driving_audio"
		}
		media = append(media, map[string]any{"type": typeName, "url": reference.URL})
	}
	if len(media) == 0 {
		return nil, inference.NewError(inference.ErrorInvalidRequest, provider, "selected Bailian video model requires reference assets", false, nil)
	}
	input["media"] = media
	return input, nil
}
