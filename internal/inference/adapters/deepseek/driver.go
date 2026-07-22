package deepseek

import (
	"context"
	"net/http"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
)

// Driver is retained as the DeepSeek compatibility name while execution is
// delegated to the reusable OpenAI Chat Completions protocol implementation.
type Driver struct{ inner *openaicompat.Driver }

func New(client *http.Client) *Driver {
	return &Driver{inner: openaicompat.NewChat(client)}
}

func (d *Driver) Execute(ctx context.Context, request inference.Request, events inference.EventSink) (inference.Result, error) {
	return d.inner.Execute(ctx, request, events)
}
