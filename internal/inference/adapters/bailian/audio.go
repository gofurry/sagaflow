package bailian

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

func (d *Driver) generateAudio(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	provider := request.Runtime.ProviderCode
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	if len(request.Inputs) != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "Bailian speech generation does not accept reference assets", false, nil)
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	format := adapterutil.StringParam(p, "format", "mp3")
	input := map[string]any{
		"text":        request.Prompt,
		"voice":       adapterutil.StringParam(p, "voice", "longanhuan_v3.6"),
		"format":      format,
		"sample_rate": int(adapterutil.NumberParam(p, "sample_rate", 24000)),
		"volume":      adapterutil.NumberParam(p, "volume", 50),
		"rate":        adapterutil.NumberParam(p, "rate", 1),
		"pitch":       adapterutil.NumberParam(p, "pitch", 1),
	}
	if instruction := adapterutil.StringParam(p, "instruction", ""); instruction != "" {
		input["instruction"] = instruction
	}
	payload := map[string]any{"model": request.Target.ID, "input": input}
	url := endpoint(request.Runtime.Endpoint, "/api/v1/services/audio/tts/SpeechSynthesizer")
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Bailian speech request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": url, "payload": payload},
	}); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Output    struct {
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
		Usage map[string]any `json:"usage"`
	}
	raw, err := d.http.DoJSON(ctx, provider, http.MethodPost, url, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if err := bailianError(provider, response.Code, response.Message); err != nil {
		return inference.Result{}, err
	}
	audioURL := strings.TrimSpace(response.Output.Audio.URL)
	if audioURL == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "Bailian speech response has no audio URL", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching"}); err != nil {
		return inference.Result{}, err
	}
	return inference.Result{Artifacts: []inference.Artifact{{
		MediaType: "audio", MIMEType: audioMIME(format), SourceURL: audioURL,
		Metadata: adapterutil.JSONSummary(raw, "request_id", "usage"), Content: d.http.URLContent(provider, audioURL),
	}}, Usage: response.Usage}, nil
}

func audioMIME(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/L16"
	case "opus":
		return "audio/opus"
	default:
		return fmt.Sprintf("audio/%s", strings.ToLower(format))
	}
}
