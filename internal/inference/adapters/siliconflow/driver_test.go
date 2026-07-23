package siliconflow

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

func TestDriverExecutesTextImageAudioAndVideo(t *testing.T) {
	var videoPolls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			messages := payload["messages"].([]any)
			content := messages[0].(map[string]any)["content"].([]any)
			imageURL := content[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
			if !strings.HasPrefix(imageURL, "data:image/png;base64,") {
				t.Fatalf("expected local image data URL, got %q", imageURL)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"完成"}}],"usage":{"total_tokens":2}}`))
		case "/v1/images/generations":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["image"] != "data:image/png;base64,aW1hZ2U=" {
				t.Fatalf("unexpected image reference: %#v", payload["image"])
			}
			_, _ = w.Write([]byte(`{"images":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png")) + `"}],"seed":1}`))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("mp3"))
		case "/v1/video/submit":
			_, _ = w.Write([]byte(`{"requestId":"video-1"}`))
		case "/v1/video/status":
			if videoPolls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"status":"InProgress"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"Succeed","results":{"videos":[{"url":"` + serverURL(r) + `/artifact.mp4"}]}}`))
		case "/artifact.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	driver := New(Config{HTTPClient: server.Client(), PollInterval: time.Millisecond})
	runtime := inference.Runtime{ProviderCode: providerCode, AdapterCode: providerCode, Endpoint: server.URL + "/v1", APIKey: "secret"}
	imageInput := inference.Input{
		Name: "reference.png", MediaType: "image", MIMEType: "image/png",
		Content: inference.BytesContent([]byte("image"), "image/png"),
	}
	cases := []struct {
		name       string
		target     inference.Target
		inputs     []inference.Input
		parameters map[string]any
		want       string
	}{
		{name: "text", target: target("chat-model", inference.CapabilityText, "chat"), inputs: []inference.Input{imageInput}, want: "完成"},
		{name: "image", target: target("edit-model", inference.CapabilityImage, "image_edit"), inputs: []inference.Input{imageInput}, want: "png"},
		{name: "audio", target: target("speech-model", inference.CapabilityAudio, "speech_generation"), parameters: map[string]any{"voice": "voice", "response_format": "mp3"}, want: "mp3"},
		{name: "video", target: target("video-model", inference.CapabilityVideo, "text_to_video"), parameters: map[string]any{"image_size": "1280x720"}, want: "video"},
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

func TestDriverDiscoversGenerationModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("sub_type") == "chat":
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen/Qwen3.6-35B-A3B"}]}`))
		case r.URL.Query().Get("sub_type") == "text-to-image":
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen/Qwen-Image"}]}`))
		case r.URL.Query().Get("sub_type") == "image-to-image":
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen/Qwen-Image-Edit-2509"}]}`))
		case r.URL.Query().Get("type") == "audio":
			_, _ = w.Write([]byte(`{"data":[{"id":"FunAudioLLM/CosyVoice2-0.5B"},{"id":"FunAudioLLM/SenseVoiceSmall"}]}`))
		case r.URL.Query().Get("type") == "video":
			_, _ = w.Write([]byte(`{"data":[{"id":"Wan-AI/Wan2.2-T2V-A14B"},{"id":"Wan-AI/Wan2.2-I2V-A14B"}]}`))
		default:
			http.Error(w, "bad query", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	driver := New(Config{HTTPClient: server.Client()})
	discovery, err := driver.Discover(context.Background(), inference.Runtime{Endpoint: server.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if discovery.Server.ModelCount != 7 || len(discovery.Models) != 7 {
		t.Fatalf("unexpected discovery result: %+v", discovery)
	}
	tasks := map[string]bool{}
	for _, model := range discovery.Models {
		tasks[model.Task] = true
	}
	for _, task := range []string{"chat", "image_generation", "image_edit", "speech_generation", "speech_recognition", "text_to_video", "image_to_video"} {
		if !tasks[task] {
			t.Fatalf("expected task %s in discovery: %+v", task, discovery.Models)
		}
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
