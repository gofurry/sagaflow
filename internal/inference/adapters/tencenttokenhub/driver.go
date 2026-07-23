// Package tencenttokenhub implements Tencent Cloud TokenHub's native text,
// image and asynchronous video APIs.
package tencenttokenhub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const providerCode = "tencent_tokenhub"

type Config struct {
	HTTPClient   *http.Client
	PollInterval time.Duration
}

type Driver struct {
	http         *adapterutil.HTTPClient
	pollInterval time.Duration
}

func New(config Config) *Driver {
	interval := config.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Driver{
		http:         adapterutil.NewHTTPClient(config.HTTPClient),
		pollInterval: interval,
	}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if request.Target.Kind != inference.TargetModel {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "TokenHub adapter only accepts model targets", false, nil)
	}
	switch request.Target.Capability {
	case inference.CapabilityText:
		return d.generateText(ctx, request, events)
	case inference.CapabilityImage:
		return d.generateImage(ctx, request, events)
	case inference.CapabilityVideo:
		return d.generateVideo(ctx, request, events)
	default:
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "unsupported TokenHub output capability", false, nil)
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
	switch strings.ToLower(request.Target.ID) {
	case "hy-image-lite":
		return "image_generation_lite"
	case "hy-image-v3.0":
		return "image_generation"
	case "yt-video-2.0":
		return "image_to_video"
	case "hy-video-1.5":
		return "text_to_video"
	default:
		return "chat"
	}
}

func endpoint(baseURL, path string) string {
	return adapterutil.JoinURL(baseURL, path)
}

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
