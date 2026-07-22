package minimax_test

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
	"github.com/gofurry/sagaflow/internal/inference/adapters/minimax"
)

func TestDriverGeneratesTextWithOpenAIChat(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "# 设定\n\n正文"}}},
			"usage":   map[string]any{"total_tokens": 12},
		})
	}))
	defer server.Close()

	result, err := minimax.New(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "minimax", AdapterCode: "minimax", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "MiniMax-M2.7", Capability: inference.CapabilityText},
		Prompt:  "输出角色设定", Parameters: map[string]any{"temperature": .3},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requestBody["model"] != "MiniMax-M2.7" || requestBody["temperature"] != .3 {
		t.Fatalf("text parameters were not forwarded: %#v", requestBody)
	}
	reader, _, _ := result.Artifacts[0].Content.Open(context.Background())
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "# 设定\n\n正文" {
		t.Fatalf("unexpected text %q", data)
	}
}

func TestDriverGeneratesImageWithCharacterReference(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image_generation" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":      map[string]any{"image_urls": []string{base64.StdEncoding.EncodeToString([]byte("png"))}},
			"base_resp": map[string]any{"status_code": 0},
		})
	}))
	defer server.Close()

	result, err := minimax.New(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "minimax", AdapterCode: "minimax", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "image-01", Capability: inference.CapabilityImage},
		Prompt:  "角色立绘", Inputs: []inference.Input{{MediaType: "image", URL: "https://assets.example/character.png"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	references, ok := requestBody["subject_reference"].([]any)
	if !ok || len(references) != 1 {
		t.Fatalf("character reference was not forwarded: %#v", requestBody)
	}
	reader, _, _ := result.Artifacts[0].Content.Open(context.Background())
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "png" {
		t.Fatalf("unexpected image %q", data)
	}
}

func TestDriverGeneratesHexAudio(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":     map[string]any{"audio": "494433"},
			"trace_id": "trace-1", "base_resp": map[string]any{"status_code": 0},
		})
	}))
	defer server.Close()

	result, err := minimax.New(server.Client()).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "minimax", AdapterCode: "minimax", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "speech", Capability: inference.CapabilityAudio},
		Prompt:  "旁白", Parameters: map[string]any{"voice_id": "voice-1", "format": "mp3"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	voice := requestBody["voice_setting"].(map[string]any)
	if voice["voice_id"] != "voice-1" {
		t.Fatalf("voice parameter was not forwarded: %#v", voice)
	}
	reader, _, _ := result.Artifacts[0].Content.Open(context.Background())
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "ID3" {
		t.Fatalf("unexpected audio %q", data)
	}
}

func TestDriverPollsAndRetrievesVideo(t *testing.T) {
	var server *httptest.Server
	var mu sync.Mutex
	polls := 0
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/video_generation":
			_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "task-1", "base_resp": map[string]any{"status_code": 0}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/query/video_generation":
			mu.Lock()
			polls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "Success", "file_id": "file-1", "base_resp": map[string]any{"status_code": 0}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/files/retrieve":
			_ = json.NewEncoder(w).Encode(map[string]any{"file": map[string]any{"download_url": server.URL + "/result.mp4"}, "base_resp": map[string]any{"status_code": 0}})
		case r.URL.Path == "/result.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var runID string
	result, err := minimax.NewWithConfig(minimax.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "minimax", AdapterCode: "minimax", Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "MiniMax-Hailuo-2.3", Capability: inference.CapabilityVideo},
		Prompt:  "镜头平稳推进", Inputs: []inference.Input{{MediaType: "image", URL: "https://assets.example/first.png"}},
	}, func(_ context.Context, event inference.Event) error {
		if event.ProviderRunID != "" {
			runID = event.ProviderRunID
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if runID != "task-1" || polls != 1 {
		t.Fatalf("unexpected async state run=%q polls=%d", runID, polls)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "video" {
		t.Fatalf("unexpected video %q", data)
	}
}
