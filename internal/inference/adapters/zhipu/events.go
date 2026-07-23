package zhipu

import (
	"context"
	"net/http"

	"github.com/gofurry/sagaflow/internal/inference"
)

func emitRequest(ctx context.Context, events inference.EventSink, endpoint string, payload map[string]any) error {
	return inference.Emit(ctx, events, inference.Event{
		Stage: "provider_request", Progress: .2, Message: "Zhipu request payload prepared",
		Details: map[string]any{"method": http.MethodPost, "endpoint": endpoint, "payload": payload},
	})
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
