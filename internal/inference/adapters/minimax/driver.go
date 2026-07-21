package minimax

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type Driver struct{ http *adapterutil.HTTPClient }

func New(client *http.Client) *Driver {
	return &Driver{http: adapterutil.NewHTTPClient(client)}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	const provider = "minimax"
	if request.Target.Kind != inference.TargetModel || request.Target.Capability != inference.CapabilityAudio {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "driver only supports audio generation", false, nil)
	}
	if err := adapterutil.Required(request.Runtime.Endpoint, "runtime endpoint", provider); err != nil {
		return inference.Result{}, err
	}
	p := request.Parameters
	if p == nil {
		p = map[string]any{}
	}
	format := adapterutil.StringParam(p, "format", "mp3")
	outputFormat := adapterutil.StringParam(p, "output_format", "hex")
	payload := map[string]any{
		"model": request.Target.ID, "text": request.Prompt, "stream": false,
		"language_boost": adapterutil.StringParam(p, "language_boost", "auto"),
		"output_format":  outputFormat, "subtitle_enable": adapterutil.BoolParam(p, "subtitle_enable", false),
		"subtitle_type":         adapterutil.StringParam(p, "subtitle_type", "sentence"),
		"aigc_watermark":        adapterutil.BoolParam(p, "aigc_watermark", false),
		"english_normalization": adapterutil.BoolParam(p, "english_normalization", false),
		"voice_setting": map[string]any{
			"voice_id": adapterutil.StringParam(p, "voice_id", "Chinese (Mandarin)_Lyrical_Voice"),
			"speed":    adapterutil.NumberParam(p, "speed", 1), "vol": adapterutil.NumberParam(p, "volume", 1),
			"pitch": adapterutil.NumberParam(p, "pitch", 0), "emotion": adapterutil.StringParam(p, "emotion", "calm"),
		},
		"audio_setting": map[string]any{
			"sample_rate": adapterutil.NumberParam(p, "sample_rate", 32000),
			"bitrate":     adapterutil.NumberParam(p, "bitrate", 128000), "format": format,
			"channel": adapterutil.NumberParam(p, "channel", 1),
		},
	}
	for _, key := range []string{"pronunciation_dict", "timbre_weights", "voice_modify"} {
		adapterutil.CopyParam(payload, p, key)
	}
	endpoint := strings.TrimRight(request.Runtime.Endpoint, "/") + "/v1/t2a_v2"
	if err := inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "MiniMax request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	}); err != nil {
		return inference.Result{}, err
	}
	var response struct {
		Data *struct {
			Audio string `json:"audio"`
		} `json:"data"`
		ExtraInfo any    `json:"extra_info"`
		TraceID   string `json:"trace_id"`
		BaseResp  struct {
			StatusCode int64  `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	raw, err := d.http.DoJSON(ctx, provider, http.MethodPost, endpoint, request.Runtime.APIKey, payload, &response)
	if err != nil {
		return inference.Result{}, err
	}
	if response.BaseResp.StatusCode != 0 {
		return inference.Result{}, inference.NewError(inference.ErrorProvider, provider, response.BaseResp.StatusMsg, false, nil)
	}
	if response.Data == nil || response.Data.Audio == "" {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "audio response is empty", false, nil)
	}
	if err := inference.Emit(ctx, events, inference.Event{Stage: "fetching"}); err != nil {
		return inference.Result{}, err
	}
	mimeType := audioMIMEType(format)
	artifact := inference.Artifact{MediaType: "audio", MIMEType: mimeType, Metadata: adapterutil.JSONSummary(raw, "trace_id", "extra_info")}
	if outputFormat == "url" {
		artifact.SourceURL = response.Data.Audio
		artifact.Content = d.http.URLContent(provider, response.Data.Audio)
	} else {
		data, decodeErr := hex.DecodeString(response.Data.Audio)
		if decodeErr != nil {
			return inference.Result{}, inference.NewError(inference.ErrorInvalidOutput, provider, "decode audio response", false, decodeErr)
		}
		artifact.Content = inference.BytesContent(data, mimeType)
	}
	return inference.Result{Artifacts: []inference.Artifact{artifact}}, nil
}

func audioMIMEType(format string) string {
	switch strings.ToLower(format) {
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/L16"
	case "mp3":
		return "audio/mpeg"
	default:
		return fmt.Sprintf("audio/%s", strings.ToLower(format))
	}
}
