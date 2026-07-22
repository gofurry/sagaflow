package deepseek_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/deepseek"
)

func TestDriverGeneratesText(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "reply-1", "choices": []any{map[string]any{"message": map[string]any{"content": "  生成正文  "}}},
			"usage": map[string]any{"prompt_tokens": 12},
		})
	}))
	defer server.Close()

	var requestEvent inference.Event
	result, err := deepseek.New(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "deepseek", AdapterCode: "deepseek", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "deepseek-v4-flash", Capability: inference.CapabilityText},
		Prompt:  "创作正文", Parameters: map[string]any{
			"temperature": .7, "thinking": "disabled", "reasoning_effort": "high",
		},
	}, func(_ context.Context, event inference.Event) error {
		if event.Stage == "provider_request" {
			requestEvent = event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestBody["model"] != "deepseek-v4-flash" || requestBody["temperature"] != .7 || requestBody["reasoning_effort"] != "high" {
		t.Fatalf("unexpected payload: %#v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking parameter was not normalized: %#v", requestBody)
	}
	payload, ok := requestEvent.Details["payload"].(map[string]any)
	if !ok || payload["model"] != "deepseek-v4-flash" || payload["temperature"] != .7 {
		t.Fatalf("provider request event is incomplete: %#v", requestEvent)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "生成正文" {
		t.Fatalf("unexpected content %q", data)
	}
}
