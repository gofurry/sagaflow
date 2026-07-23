package tencenttokenhub

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
)

func TestDiscoverFiltersTokenHubProxyModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing bearer key")
		}
		_, _ = io.WriteString(w, `{"data":[
			{"id":"hy3","status":"online"},
			{"id":"hy-image-v3.0","status":"pre-offline"},
			{"id":"third-party/model","status":"online"}
		]}`)
	}))
	defer server.Close()

	discovery, err := New(Config{HTTPClient: server.Client()}).Discover(context.Background(), inference.Runtime{
		Endpoint: server.URL + "/v1", APIKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Models) != 2 {
		t.Fatalf("models = %#v", discovery.Models)
	}
	for _, model := range discovery.Models {
		if model.Name == "hy-image-v3.0" && (model.RemoteStatus != "pre-offline" || model.LifecycleStatus != "deprecated") {
			t.Fatalf("pre-offline lifecycle = %#v", model)
		}
	}
}

func TestTextSupportsLocalImageWithoutTracingPayload(t *testing.T) {
	var eventPayload string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(payload)
		if !strings.Contains(string(encoded), "data:image/png;base64,") {
			t.Fatalf("payload = %s", encoded)
		}
		_, _ = io.WriteString(w, `{"id":"chat-1","choices":[{"message":{"content":"画面描述"}}],"usage":{"total_tokens":12}}`)
	}))
	defer server.Close()

	result, err := New(Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-vision-2.0-instruct", Capability: inference.CapabilityText},
		Prompt:  "描述画面",
		Inputs: []inference.Input{{
			MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent([]byte("png"), "image/png"),
		}},
	}, func(_ context.Context, event inference.Event) error {
		if event.Stage == "provider_request" {
			data, _ := json.Marshal(event.Details["payload"])
			eventPayload = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(eventPayload, "cG5n") || !strings.Contains(eventPayload, "[LOCAL_DATA]") {
		t.Fatalf("trace leaked local data: %s", eventPayload)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	if string(body) != "画面描述" {
		t.Fatalf("text = %q", body)
	}
}

func TestImageLiteAndAsyncVideoContracts(t *testing.T) {
	var queryCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/image/lite":
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}],"seed":7}`)
		case "/api/video/submit":
			_, _ = io.WriteString(w, `{"id":"video-1","status":"pending"}`)
		case "/api/video/query":
			queryCount++
			_, _ = io.WriteString(w, `{"id":"video-1","status":"completed","data":[{"url":"`+serverURL(r)+`/artifact.mp4"}]}`)
		case "/artifact.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "video")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	driver := New(Config{HTTPClient: server.Client(), PollInterval: time.Millisecond})

	image, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-image-lite", Capability: inference.CapabilityImage},
		Prompt:  "暖色调远景",
	}, nil)
	if err != nil || len(image.Artifacts) != 1 {
		t.Fatalf("image result = %#v, err = %v", image, err)
	}
	video, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: server.URL},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-video-1.5", Capability: inference.CapabilityVideo},
		Prompt:  "镜头缓慢推进",
	}, nil)
	if err != nil || len(video.Artifacts) != 1 || queryCount != 1 {
		t.Fatalf("video result = %#v, queries = %d, err = %v", video, queryCount, err)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
