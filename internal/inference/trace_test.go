package inference

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSnapshotRequestRedactsSecretsAndTransportURLs(t *testing.T) {
	request := Request{
		ID:      "job-1",
		Runtime: Runtime{ProviderCode: "seedream", AdapterCode: "volcengine", Endpoint: "https://user:pass@example.test/v1?token=secret", APIKey: "must-not-leak"},
		Target:  Target{Kind: TargetModel, ID: "image-model", Capability: CapabilityImage},
		Prompt:  "一只雪豹",
		Parameters: map[string]any{
			"seed":    42,
			"api_key": "hidden",
			"nested":  map[string]any{"access_token": "hidden", "callback": "https://example.test/result?signature=hidden"},
		},
		Inputs: []Input{{ID: "asset-1", Name: "设定图", MediaType: "image", MIMEType: "image/png", URL: "https://signed.test/file?token=hidden", Content: BytesContent([]byte("image"), "image/png")}},
	}

	snapshot := SnapshotRequest(request)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"must-not-leak", "user:pass", "token=secret", "signature=hidden"} {
		if strings.Contains(text, secret) {
			t.Fatalf("snapshot leaked %q: %s", secret, text)
		}
	}
	if snapshot.Parameters["seed"] != 42 {
		t.Fatalf("safe parameters were not preserved: %#v", snapshot.Parameters)
	}
	if snapshot.Parameters["api_key"] != "[REDACTED]" {
		t.Fatalf("api key was not redacted: %#v", snapshot.Parameters)
	}
	if len(snapshot.Inputs) != 1 || !snapshot.Inputs[0].ProviderURL || !snapshot.Inputs[0].ContentStream {
		t.Fatalf("input transport summary is incomplete: %#v", snapshot.Inputs)
	}
}

func TestSnapshotResultAndError(t *testing.T) {
	result := SnapshotResult(Result{
		Artifacts: []Artifact{{
			MediaType: "image", MIMEType: "image/png", SourceURL: "https://cdn.test/a.png?token=hidden",
			Metadata: json.RawMessage(`{"trace_id":"trace-1","authorization":"hidden"}`),
			Content:  BytesContent([]byte("image"), "image/png"),
		}},
		Usage: map[string]any{"total_tokens": 12, "token": "hidden"},
	})
	if result.Artifacts[0].SourceURL != "https://cdn.test/a.png" {
		t.Fatalf("source URL was not sanitized: %q", result.Artifacts[0].SourceURL)
	}
	if result.Artifacts[0].Metadata["authorization"] != "[REDACTED]" || result.Usage["token"] != "[REDACTED]" {
		t.Fatalf("result secrets were not redacted: %#v %#v", result.Artifacts[0].Metadata, result.Usage)
	}

	failure := SnapshotError(NewError(ErrorRateLimited, "deepseek", "too many requests", true, errors.New("cause")))
	if failure.Kind != ErrorRateLimited || failure.Provider != "deepseek" || !failure.Retryable || failure.Message != "too many requests" {
		t.Fatalf("unexpected failure snapshot: %#v", failure)
	}
}

func TestSnapshotDetailsRedactsActualProviderPayload(t *testing.T) {
	details := SnapshotDetails(map[string]any{
		"endpoint": "https://api.test/v1?signature=hidden",
		"payload": map[string]any{
			"model":         "model-1",
			"image":         []string{"https://assets.test/reference.png?token=hidden"},
			"content":       []map[string]any{{"type": "image_url", "image_url": map[string]string{"url": "https://assets.test/reference.png?credential=hidden"}}},
			"authorization": "Bearer hidden",
		},
	})
	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "hidden") || !strings.Contains(text, `"model":"model-1"`) {
		t.Fatalf("unexpected provider detail snapshot: %s", text)
	}
}
