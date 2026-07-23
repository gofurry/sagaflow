// Package moonshot implements Kimi's OpenAI-compatible Chat Completions API,
// including local-first image and video references and live model discovery.
package moonshot

import (
	"context"
	"net/http"
	"strings"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapterutil"
)

const providerCode = "moonshot"

type Driver struct {
	http *adapterutil.HTTPClient
}

func New(client *http.Client) *Driver {
	return &Driver{http: adapterutil.NewHTTPClient(client)}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	if request.Target.Kind != inference.TargetModel || request.Target.Capability != inference.CapabilityText {
		return inference.Result{}, inference.NewError(inference.ErrorInvalidRequest, provider(request), "Moonshot adapter only accepts text model targets", false, nil)
	}
	return d.generateText(ctx, request, events)
}

func provider(request inference.Request) string {
	if value := strings.TrimSpace(request.Runtime.ProviderCode); value != "" {
		return value
	}
	return providerCode
}

func endpoint(baseURL, path string) string {
	return adapterutil.JoinURL(baseURL, path)
}
