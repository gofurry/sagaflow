package moonshot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestExecuteMaterializesLocalImageAndVideo(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing bearer token")
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"chat-1","model":"kimi-k2.6","choices":[{"message":{"content":"完成"}}],"usage":{"total_tokens":12}}`)
	}))
	defer server.Close()

	var traced map[string]any
	result, err := New(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: providerCode, Endpoint: server.URL + "/v1", APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "kimi-k2.6", Capability: inference.CapabilityText},
		Prompt:  "概括参考内容",
		Inputs: []inference.Input{
			{MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent([]byte("png"), "image/png")},
			{MediaType: "video", MIMEType: "video/mp4", Content: inference.BytesContent([]byte("mp4"), "video/mp4")},
		},
		Parameters: map[string]any{"max_tokens": 512, "thinking": "disabled"},
	}, func(_ context.Context, event inference.Event) error {
		traced, _ = event.Details["payload"].(map[string]any)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %d", len(result.Artifacts))
	}
	raw, _ := json.Marshal(received)
	if !strings.Contains(string(raw), "data:image/png;base64,") || !strings.Contains(string(raw), "data:video/mp4;base64,") {
		t.Fatalf("expected local references in payload: %s", raw)
	}
	if !strings.Contains(string(raw), `"thinking":{"type":"disabled"}`) {
		t.Fatalf("expected structured thinking parameter: %s", raw)
	}
	traceRaw, _ := json.Marshal(traced)
	if strings.Contains(string(traceRaw), "cG5n") || strings.Contains(string(traceRaw), "bXA0") || strings.Count(string(traceRaw), "[LOCAL_DATA]") != 2 {
		t.Fatalf("trace leaked local content: %s", traceRaw)
	}
}

func TestDiscoverFiltersToCatalogModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":[
			{"id":"kimi-k3","context_length":1048576,"supports_image_in":true,"supports_video_in":true,"supports_reasoning":true},
			{"id":"unknown-model","context_length":1}
		]}`)
	}))
	defer server.Close()

	discovery, err := New(server.Client()).Discover(context.Background(), inference.Runtime{Endpoint: server.URL + "/v1", APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.Server.ModelCount != 1 || len(discovery.Models) != 1 {
		t.Fatalf("unexpected discovery: %#v", discovery)
	}
	if discovery.Models[0].Name != "kimi-k3" || discovery.Models[0].ContextLength != 1048576 {
		t.Fatalf("unexpected model: %#v", discovery.Models[0])
	}
}
