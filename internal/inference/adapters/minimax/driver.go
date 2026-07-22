package minimax

import (
	"context"
	"net/http"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

type Config struct {
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Driver struct {
	http         *adapterutil.HTTPClient
	text         *openaicompat.Driver
	pollInterval time.Duration
}

func New(client *http.Client) *Driver {
	return NewWithConfig(Config{HTTPClient: client})
}

func NewWithConfig(config Config) *Driver {
	interval := config.PollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &Driver{
		http:         adapterutil.NewHTTPClient(config.HTTPClient),
		text:         openaicompat.NewChatAt(config.HTTPClient, "/v1/chat/completions"),
		pollInterval: interval,
	}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	const provider = "minimax"
	if request.Target.Kind != inference.TargetModel {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "MiniMax adapter only accepts model targets", false, nil)
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
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider, "unsupported MiniMax output capability", false, nil)
	}
}
