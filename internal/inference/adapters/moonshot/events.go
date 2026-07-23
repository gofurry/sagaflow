package moonshot

import (
	"context"
	"net/http"

	"github.com/gofurry/sagaflow/internal/inference"
)

func emitRequest(ctx context.Context, events inference.EventSink, endpoint string, payload map[string]any) error {
	return inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Kimi request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	})
}
