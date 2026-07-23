// Package siliconflow implements SiliconFlow's OpenAI-compatible text API and
// native image, audio, video and model discovery APIs.
package siliconflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const providerCode = "siliconflow"

type Config struct {
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Driver struct {
	client       *http.Client
	http         *adapterutil.HTTPClient
	text         *openaicompat.Driver
	pollInterval time.Duration
}

func New(config Config) *Driver {
	client := config.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	interval := config.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Driver{
		client: client, http: adapterutil.NewHTTPClient(client),
		text: openaicompat.NewChat(client), pollInterval: interval,
	}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if request.Target.Kind != inference.TargetModel {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "SiliconFlow adapter only accepts model targets", false, nil)
	}
	switch modelTask(request) {
	case "speech_recognition":
		return d.transcribe(ctx, request, events)
	}
	switch request.Target.Capability {
	case inference.CapabilityText:
		updated, err := d.materializeReferences(ctx, request, "image")
		if err != nil {
			return inference.Result{}, err
		}
		return d.text.Execute(ctx, updated, events)
	case inference.CapabilityImage:
		return d.generateImage(ctx, request, events)
	case inference.CapabilityAudio:
		return d.generateSpeech(ctx, request, events)
	case inference.CapabilityVideo:
		return d.generateVideo(ctx, request, events)
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "unsupported SiliconFlow output capability", false, nil)
	}
}

func provider(request inference.Request) string {
	if value := strings.TrimSpace(request.Runtime.ProviderCode); value != "" {
		return value
	}
	return providerCode
}

func modelTask(request inference.Request) string {
	if len(request.Target.Spec) > 0 {
		var metadata struct {
			Task string `json:"task"`
		}
		if json.Unmarshal(request.Target.Spec, &metadata) == nil && metadata.Task != "" {
			return metadata.Task
		}
	}
	modelID := strings.ToLower(request.Target.ID)
	switch {
	case strings.Contains(modelID, "sensevoice") || strings.Contains(modelID, "speechasr"):
		return "speech_recognition"
	case strings.Contains(modelID, "moss-ttsd") || strings.Contains(modelID, "cosyvoice"):
		return "speech_generation"
	case strings.Contains(modelID, "image-edit"):
		return "image_edit"
	case strings.Contains(modelID, "i2v"):
		return "image_to_video"
	case strings.Contains(modelID, "t2v"):
		return "text_to_video"
	}
	return ""
}

func endpoint(baseURL, path string) string {
	return adapterutil.JoinURL(baseURL, path)
}
