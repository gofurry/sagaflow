package openaicompat_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
)

func TestChatDriverGeneratesTextWithImageInput(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "reply-1", "choices": []any{map[string]any{"message": map[string]any{"content": "  角色描述  "}}},
			"usage": map[string]any{"prompt_tokens": 12},
		})
	}))
	defer server.Close()

	result, err := openaicompat.NewChatAt(server.Client(), "/v1/chat/completions").Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "compatible", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "vision-model", Capability: inference.CapabilityText},
		Prompt:  "描述角色", Inputs: []inference.Input{{MediaType: "image", URL: "https://assets.example/role.png"}},
		Parameters: map[string]any{"response_format": "json_object"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := requestBody["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 2 || requestBody["stream"] != false {
		t.Fatalf("unexpected payload: %#v", requestBody)
	}
	if got := artifactText(t, result); got != "角色描述" {
		t.Fatalf("unexpected content %q", got)
	}
}

func TestResponsesDriverExtractsOutputText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "response-1", "status": "completed",
			"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "剧情大纲"}}}},
			"usage":  map[string]any{"input_tokens": 4, "output_tokens": 8},
		})
	}))
	defer server.Close()

	result, err := openaicompat.NewResponses(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "ark", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "doubao", Capability: inference.CapabilityText},
		Prompt:  "写大纲", Parameters: map[string]any{"thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := artifactText(t, result); got != "剧情大纲" {
		t.Fatalf("unexpected content %q", got)
	}
}

func artifactText(t *testing.T, result inference.Result) string {
	t.Helper()
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
