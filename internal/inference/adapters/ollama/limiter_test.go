package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestMaxConcurrencyDefaultsAndBounds(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want int
	}{
		{name: "missing", raw: nil, want: 1},
		{name: "configured", raw: json.RawMessage(`{"max_concurrency":3}`), want: 3},
		{name: "too large", raw: json.RawMessage(`{"max_concurrency":9}`), want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := maxConcurrency(test.raw); got != test.want {
				t.Fatalf("maxConcurrency() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestAcquireSerializesConnectionByDefault(t *testing.T) {
	driver := New(nil)
	runtime := inference.Runtime{ProviderCode: "ollama-local", Endpoint: "http://127.0.0.1:11434"}
	release, err := driver.acquire(context.Background(), runtime)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := driver.acquire(waitCtx, runtime); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second acquire error = %v, want deadline exceeded", err)
	}
	release()

	releaseAgain, err := driver.acquire(context.Background(), runtime)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseAgain()
}
