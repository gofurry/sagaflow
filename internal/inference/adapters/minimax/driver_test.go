package minimax_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/minimax"
)

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
