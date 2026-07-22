// Package bailian implements Alibaba Cloud Model Studio (DashScope) model
// execution across its OpenAI-compatible text and native media APIs.
package bailian

import (
	"context"
	"net/http"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const providerCode = "aliyun_bailian"

type Config struct {
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Driver struct {
	http         *adapterutil.HTTPClient
	text         *openaicompat.Driver
	pollInterval time.Duration
}

func New(config Config) *Driver {
	interval := config.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Driver{
		http:         adapterutil.NewHTTPClient(config.HTTPClient),
		text:         openaicompat.NewChatAt(config.HTTPClient, "/compatible-mode/v1/chat/completions"),
		pollInterval: interval,
	}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if request.Target.Kind != inference.TargetModel {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerCode, "Bailian adapter only accepts model targets", false, nil)
	}
	switch request.Target.Capability {
	case inference.CapabilityText:
		return d.text.Execute(ctx, request, events)
	case inference.CapabilityImage:
		return d.generateImage(ctx, request, events)
	case inference.CapabilityAudio:
		return d.generateAudio(ctx, request, events)
	case inference.CapabilityVideo:
		return d.generateVideo(ctx, request, events)
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, providerCode, "unsupported Bailian output capability", false, nil)
	}
}

func endpoint(baseURL, path string) string {
	return adapterutil.JoinURL(baseURL, path)
}
