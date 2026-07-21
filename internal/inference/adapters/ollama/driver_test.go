package ollama_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/ollama"
)

func TestDriverExecuteStreamsTextAndImage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Images []string `json:"images"`
			} `json:"messages"`
			Options map[string]any `json:"options"`
			Think   bool           `json:"think"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "qwen3.5:4b" || len(payload.Messages) != 1 || len(payload.Messages[0].Images) != 1 {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		if decoded, err := base64.StdEncoding.DecodeString(payload.Messages[0].Images[0]); err != nil || string(decoded) != "image" {
			t.Fatalf("unexpected encoded image: %q, %v", decoded, err)
		}
		if payload.Options["num_ctx"] != float64(4096) || payload.Think {
			t.Fatalf("unexpected options: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"model":"qwen3.5:4b","message":{"content":"你好"},"done":false}`+"\n")
		_, _ = io.WriteString(w, `{"model":"qwen3.5:4b","message":{"content":"，世界"},"done":true,"done_reason":"stop","total_duration":12,"load_duration":2,"prompt_eval_count":8,"eval_count":4,"eval_duration":6}`+"\n")
	}))
	defer server.Close()

	driver := ollama.New(server.Client())
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime:    inference.Runtime{ProviderCode: "ollama-local", AdapterCode: "ollama", Endpoint: server.URL},
		Target:     inference.Target{Kind: inference.TargetModel, ID: "qwen3.5:4b", Capability: inference.CapabilityText},
		Prompt:     "描述这张图片",
		Parameters: map[string]any{"num_ctx": 4096, "think": false},
		Inputs:     []inference.Input{{MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent([]byte("image"), "image/png")}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "你好，世界" {
		t.Fatalf("unexpected content: %s", content)
	}
	if result.Usage["eval_count"] != int64(4) {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}
}

func TestDriverDiscoversModelCapabilities(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/version":
			_, _ = io.WriteString(w, `{"version":"0.32.1"}`)
		case "/api/tags":
			_, _ = io.WriteString(w, `{"models":[{"name":"qwen3.5:4b","model":"qwen3.5:4b","size":3389983735,"digest":"abc","modified_at":"2026-07-19T00:00:00Z","details":{"parameter_size":"4.7B","quantization_level":"Q4_K_M"}}]}`)
		case "/api/ps":
			_, _ = io.WriteString(w, `{"models":[]}`)
		case "/api/show":
			_, _ = io.WriteString(w, `{"parameters":"temperature 1","capabilities":["completion","vision","tools","thinking"],"model_info":{"qwen35.context_length":262144}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	discovery, err := ollama.New(server.Client()).Discover(context.Background(), inference.Runtime{
		ProviderCode: "ollama-local", AdapterCode: "ollama", Endpoint: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.Server.Version != "0.32.1" || len(discovery.Models) != 1 {
		t.Fatalf("unexpected discovery: %#v", discovery)
	}
	model := discovery.Models[0]
	if !model.SupportsTextOutput || model.ContextLength != 262144 || len(model.InputModalities) != 2 {
		t.Fatalf("unexpected model: %#v", model)
	}
}
