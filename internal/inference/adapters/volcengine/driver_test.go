package volcengine_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/volcengine"
)

func TestDriverForwardsImageReferences(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("png"))}}})
	}))
	defer server.Close()

	result, err := volcengine.New(volcengine.Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "seedream", AdapterCode: "volcengine", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "image-model", Capability: inference.CapabilityImage}, Prompt: "角色设定",
		Inputs: []inference.Input{{MediaType: "image", URL: "https://assets.example/reference.png"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestBody["image"] != "https://assets.example/reference.png" {
		t.Fatalf("reference was not forwarded: %#v", requestBody)
	}
	reader, _, _ := result.Artifacts[0].Content.Open(context.Background())
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "png" {
		t.Fatalf("unexpected image %q", data)
	}
}

func TestDriverUsesResponsesForText(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "response-1", "status": "completed",
			"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "# 场景设定"}}}},
			"usage":  map[string]any{"total_tokens": 18},
		})
	}))
	defer server.Close()

	result, err := volcengine.New(volcengine.Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "volcengine", AdapterCode: "volcengine", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "doubao-seed-2-0-lite-260215", Capability: inference.CapabilityText},
		Prompt:  "编写场景设定", Parameters: map[string]any{"max_output_tokens": 1024, "thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestBody["model"] != "doubao-seed-2-0-lite-260215" || requestBody["max_output_tokens"] != float64(1024) {
		t.Fatalf("text parameters were not forwarded: %#v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking parameter was not normalized: %#v", requestBody)
	}
	reader, _, _ := result.Artifacts[0].Content.Open(context.Background())
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "# 场景设定" {
		t.Fatalf("unexpected text %q", data)
	}
}

func TestDriverPollsVideoAndStreamsArtifact(t *testing.T) {
	var server *httptest.Server
	var mu sync.Mutex
	polls := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/contents/generations/tasks":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		case r.Method == http.MethodGet && r.URL.Path == "/contents/generations/tasks/run-1":
			mu.Lock()
			polls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "video_url": server.URL + "/result.mp4"})
		case r.URL.Path == "/result.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var runID string
	result, err := volcengine.New(volcengine.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "seedance", AdapterCode: "volcengine", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "video-model", Capability: inference.CapabilityVideo}, Prompt: "镜头",
		Inputs: []inference.Input{{MediaType: "image", URL: "https://assets.example/character.png"}},
	}, func(_ context.Context, event inference.Event) error {
		if event.ProviderRunID != "" {
			runID = event.ProviderRunID
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run-1" || polls != 1 {
		t.Fatalf("unexpected async state run=%q polls=%d", runID, polls)
	}
	reader, info, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "video" || info.MIMEType != "video/mp4" {
		t.Fatalf("unexpected video data=%q info=%#v", data, info)
	}
}

func TestDriverResumesExistingVideoTask(t *testing.T) {
	var server *httptest.Server
	posts := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			posts++
			http.Error(w, "unexpected submit", http.StatusBadRequest)
		case r.URL.Path == "/contents/generations/tasks/run-existing":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "succeeded", "video_url": server.URL + "/result.mp4"})
		case r.URL.Path == "/result.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("resumed"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := volcengine.New(volcengine.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime:       inference.Runtime{ProviderCode: "volcengine", AdapterCode: "volcengine", Endpoint: server.URL},
		Target:        inference.Target{Kind: inference.TargetModel, ID: "video-model", Capability: inference.CapabilityVideo},
		ProviderRunID: "run-existing",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if posts != 0 || result.Artifacts[0].SourceURL == "" {
		t.Fatalf("existing task was not resumed: posts=%d result=%#v", posts, result)
	}
}
