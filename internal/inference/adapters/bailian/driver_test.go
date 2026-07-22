package bailian_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/bailian"
)

func TestTextUsesCompatibleChatEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compatible-mode/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		writeJSON(t, w, map[string]any{"id": "chat-1", "choices": []any{map[string]any{"message": map[string]any{"content": "洞穴入口泛着微光。"}}}})
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "qwen3.7-plus", Capability: inference.CapabilityText}, Prompt: "写一句场景描述",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactText(t, result.Artifacts[0], "洞穴入口")
}

func TestImageSubmitsPollsAndDownloads(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/image-generation/generation":
			if r.Header.Get("X-DashScope-Async") != "enable" {
				t.Fatal("missing asynchronous header")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["model"] != "wan2.7-image-pro" {
				t.Fatalf("unexpected payload %#v", body)
			}
			writeJSON(t, w, map[string]any{"output": map[string]any{"task_id": "image-task", "task_status": "PENDING"}})
		case "/api/v1/tasks/image-task":
			writeJSON(t, w, map[string]any{"output": map[string]any{"task_id": "image-task", "task_status": "SUCCEEDED", "choices": []any{map[string]any{"message": map[string]any{"content": []any{map[string]any{"image": server.URL + "/image.png"}}}}}}})
		case "/image.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "wan2.7-image-pro", Capability: inference.CapabilityImage}, Prompt: "狼兽人探险者",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactBytes(t, result.Artifacts[0], "png")
}

func TestAudioDownloadsResult(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/audio/tts/SpeechSynthesizer":
			writeJSON(t, w, map[string]any{"request_id": "audio-1", "output": map[string]any{"audio": map[string]any{"url": server.URL + "/voice.mp3"}}, "usage": map[string]any{"characters": 8}})
		case "/voice.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "qwen-audio-3.0-tts-plus", Capability: inference.CapabilityAudio}, Prompt: "前方有风声。",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactBytes(t, result.Artifacts[0], "mp3")
}

func TestCosyVoiceRequiresCustomVoice(t *testing.T) {
	t.Parallel()
	_, err := bailian.New(bailian.Config{}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: "https://example.test", APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "cosyvoice-v3.5-plus", Capability: inference.CapabilityAudio}, Prompt: "你好",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "requires a cloned or designed voice ID") {
		t.Fatalf("expected custom voice validation, got %v", err)
	}
}

func TestVideoResumesExistingTaskWithoutSubmitting(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/tasks/existing-task":
			writeJSON(t, w, map[string]any{"output": map[string]any{"task_status": "SUCCEEDED", "video_url": server.URL + "/clip.mp4"}})
		case "/clip.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("mp4"))
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "happyhorse-1.1-t2v", Capability: inference.CapabilityVideo},
		Prompt:  "穿过遗迹", ProviderRunID: "existing-task",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactBytes(t, result.Artifacts[0], "mp4")
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func assertArtifactBytes(t *testing.T, artifact inference.Artifact, expected string) {
	t.Helper()
	reader, _, err := artifact.Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, data)
	}
}

func assertArtifactText(t *testing.T, artifact inference.Artifact, expected string) {
	t.Helper()
	reader, _, err := artifact.Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), expected) {
		t.Fatalf("expected %q in %q", expected, data)
	}
}
