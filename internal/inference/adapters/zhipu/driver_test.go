package zhipu

import (
	"context"
	"encoding/base64"
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

func TestDriverExecutesTextImageAudioVideoAndTranscription(t *testing.T) {
	var videoPolls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v4/chat/completions":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			messages := payload["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			imageURL := content[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
			if !strings.HasPrefix(imageURL, "data:image/png;base64,") {
				t.Fatalf("expected local image data URL, got %q", imageURL)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"完成"}}],"usage":{"total_tokens":2}}`))
		case "/v4/images/generations":
			_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png")) + `"}],"created":1}`))
		case "/v4/audio/speech":
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("wav"))
		case "/v4/audio/transcriptions":
			if err := r.ParseMultipartForm(30 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("model") != "glm-asr-2512" {
				t.Fatalf("unexpected ASR model: %q", r.FormValue("model"))
			}
			_, _ = w.Write([]byte(`{"text":"潮汐灯塔"}`))
		case "/v4/videos/generations":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if value, ok := payload["image_url"].(string); !ok || !strings.HasPrefix(value, "data:image/png;base64,") {
				t.Fatalf("expected local video reference, got %#v", payload["image_url"])
			}
			_, _ = w.Write([]byte(`{"id":"video-1","task_status":"PROCESSING"}`))
		case "/v4/async-result/video-1":
			if videoPolls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"id":"video-1","task_status":"PROCESSING"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"video-1","task_status":"SUCCESS","video_result":[{"url":"` + serverURL(r) + `/artifact.mp4"}]}`))
		case "/artifact.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	driver := New(Config{HTTPClient: server.Client(), PollInterval: time.Millisecond})
	runtime := inference.Runtime{ProviderCode: providerCode, AdapterCode: providerCode, Endpoint: server.URL + "/v4", APIKey: "secret"}
	imageInput := inference.Input{
		Name: "reference.png", MediaType: "image", MIMEType: "image/png",
		Content: inference.BytesContent([]byte("image"), "image/png"),
	}
	audioInput := inference.Input{
		Name: "speech.wav", MediaType: "audio", MIMEType: "audio/wav",
		Content: inference.BytesContent([]byte("audio"), "audio/wav"),
	}
	cases := []struct {
		name       string
		target     inference.Target
		inputs     []inference.Input
		parameters map[string]any
		want       string
	}{
		{name: "text", target: target("glm-5v-turbo", inference.CapabilityText, "chat"), inputs: []inference.Input{imageInput}, want: "完成"},
		{name: "image", target: target("glm-image", inference.CapabilityImage, "image_generation"), want: "png"},
		{name: "audio", target: target("glm-tts", inference.CapabilityAudio, "speech_generation"), parameters: map[string]any{"voice": "tongtong", "response_format": "wav"}, want: "wav"},
		{name: "video", target: target("cogvideox-3", inference.CapabilityVideo, ""), inputs: []inference.Input{imageInput}, want: "video"},
		{name: "asr", target: target("glm-asr-2512", inference.CapabilityText, "speech_recognition"), inputs: []inference.Input{audioInput}, want: "潮汐灯塔"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			result, err := driver.Execute(context.Background(), inference.Request{
				Runtime: runtime, Target: item.target, Prompt: "prompt",
				Inputs: item.inputs, Parameters: item.parameters,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			reader, _, err := result.Artifacts[0].Content.Open(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			if string(data) != item.want {
				t.Fatalf("expected %q, got %q", item.want, data)
			}
		})
	}
}

func TestViduReferenceRequiresRemoteURLs(t *testing.T) {
	driver := New(Config{})
	_, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: providerCode, Endpoint: "https://example.test/v4"},
		Target:  target("vidu2-reference", inference.CapabilityVideo, "reference_to_video"),
		Prompt:  "prompt",
		Inputs: []inference.Input{{
			Name: "reference.png", MediaType: "image", MIMEType: "image/png",
			Content: inference.BytesContent([]byte("image"), "image/png"),
		}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "S3-published") {
		t.Fatalf("expected remote URL error, got %v", err)
	}
}

func target(id string, capability inference.Capability, task string) inference.Target {
	return inference.Target{
		Kind: inference.TargetModel, ID: id, Capability: capability,
		Spec: json.RawMessage(`{"task":"` + task + `"}`),
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
