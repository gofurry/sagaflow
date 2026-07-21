package comfyui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestExecuteBindsUploadsAndStreamsOutput(t *testing.T) {
	const promptID = "7b929cf9-3e54-4f1a-b58f-5fcf299344ae"
	var submitted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/history/"+promptID:
			if !submitted.Load() {
				writeJSON(w, map[string]any{})
				return
			}
			writeJSON(w, map[string]any{promptID: map[string]any{
				"status":  map[string]any{"status_str": "success", "completed": true},
				"outputs": map[string]any{"9": map[string]any{"images": []any{map[string]any{"filename": "result.png", "subfolder": "SagaFlow", "type": "output"}}}},
			}})
		case r.URL.Path == "/queue":
			writeJSON(w, map[string]any{"queue_running": []any{}, "queue_pending": []any{}})
		case r.URL.Path == "/upload/image":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("subfolder") != "sagaflow/"+promptID {
				t.Fatalf("unexpected upload subfolder %q", r.FormValue("subfolder"))
			}
			writeJSON(w, map[string]any{"name": "01-reference.png", "subfolder": "sagaflow/" + promptID, "type": "input"})
		case r.URL.Path == "/prompt":
			var payload struct {
				Prompt   map[string]WorkflowNode `json:"prompt"`
				PromptID string                  `json:"prompt_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.PromptID != promptID || payload.Prompt["6"].Inputs["text"] != "雪豹信使" || payload.Prompt["10"].Inputs["image"] != "sagaflow/"+promptID+"/01-reference.png" {
				t.Fatalf("workflow bindings were not applied: %#v", payload)
			}
			submitted.Store(true)
			writeJSON(w, map[string]any{"prompt_id": promptID, "number": 1, "node_errors": map[string]any{}})
		case r.URL.Path == "/view":
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "generated-image")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	inputIndex := 0
	spec := WorkflowSpec{
		Workflow: map[string]WorkflowNode{
			"6":  {ClassType: "CLIPTextEncode", Inputs: map[string]any{"text": ""}},
			"9":  {ClassType: "SaveImage", Inputs: map[string]any{"filename_prefix": "SagaFlow"}},
			"10": {ClassType: "LoadImage", Inputs: map[string]any{"image": ""}},
		},
		Bindings: map[string]Binding{
			"prompt":    {NodeID: "6", Input: "text", Source: "prompt", Required: true},
			"reference": {NodeID: "10", Input: "image", Source: "input", InputIndex: &inputIndex, Required: true},
		},
		Outputs: []OutputSelector{{NodeID: "9", Key: "images", MediaType: "image"}},
		Version: 1, Checksum: strings.Repeat("a", 64),
	}
	driver := New(Config{HTTPClient: server.Client(), PollInterval: time.Millisecond})
	result, err := driver.Execute(context.Background(), inference.Request{
		ID: promptID, Runtime: inference.Runtime{ProviderCode: "comfy-local", AdapterCode: "comfyui", Endpoint: server.URL},
		Target: inference.Target{Kind: inference.TargetWorkflow, ID: "portrait@1", Capability: inference.CapabilityImage, Spec: mustJSON(spec)},
		Prompt: "雪豹信使", Inputs: []inference.Input{{ID: "asset-1", Name: "reference.png", MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent([]byte("reference"), "image/png")}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].MediaType != "image" {
		t.Fatalf("unexpected artifacts: %#v", result.Artifacts)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "generated-image" {
		t.Fatalf("unexpected output %q", data)
	}
}

func TestDiscoveryAndCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/system_stats":
			writeJSON(w, map[string]any{"system": map[string]any{"comfyui_version": "0.9.2"}, "devices": []any{map[string]any{"name": "GPU", "type": "cuda", "vram_total": 8 << 30}}})
		case "/features":
			writeJSON(w, map[string]any{"jobs_api": true})
		case "/object_info":
			writeJSON(w, map[string]any{"KSampler": map[string]any{}, "SaveImage": map[string]any{}})
		case "/models":
			writeJSON(w, []string{"checkpoints"})
		case "/models/checkpoints":
			writeJSON(w, []string{"anything.safetensors"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	driver := New(Config{HTTPClient: server.Client()})
	discovery, err := driver.Discover(context.Background(), inference.Runtime{Endpoint: server.URL, ProviderCode: "comfy-local"})
	if err != nil {
		t.Fatal(err)
	}
	status, report := EvaluateCompatibility(WorkflowSpec{
		Workflow:     map[string]WorkflowNode{"1": {ClassType: "KSampler", Inputs: map[string]any{}}},
		Requirements: Requirements{Models: []ModelRequirement{{Folder: "checkpoints", Name: "anything.safetensors"}}},
	}, discovery)
	if status != "ready" || len(report.MissingNodes) != 0 || discovery.Server.Version != "0.9.2" {
		t.Fatalf("unexpected discovery result: %s %#v %#v", status, report, discovery)
	}
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
