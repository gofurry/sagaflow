package bailian_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func TestQwenImageUsesSynchronousMultimodalContract(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			if r.Header.Get("X-DashScope-Async") != "" {
				t.Fatal("Qwen Image must use the synchronous endpoint")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			parameters := body["parameters"].(map[string]any)
			if parameters["size"] != "2688*1536" || parameters["negative_prompt"] != "模糊" || parameters["thinking_mode"] != nil {
				t.Fatalf("unexpected Qwen Image parameters %#v", parameters)
			}
			writeJSON(t, w, map[string]any{"request_id": "qwen-image-1", "output": map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": []any{map[string]any{"image": server.URL + "/qwen.png"}}}}}}})
		case "/qwen.png":
			_, _ = w.Write([]byte("qwen"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client()}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "qwen-image-2.0-pro", Capability: inference.CapabilityImage},
		Prompt:  "带有中文招牌的街道", Parameters: map[string]any{"size": "2688*1536", "negative_prompt": "模糊", "prompt_extend": true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactBytes(t, result.Artifacts[0], "qwen")
}

func TestPreciseImageEditSubmitsNativeOutpaintAndMaskPayloads(t *testing.T) {
	t.Parallel()
	source := testPNG(t, false)
	mask := testPNG(t, true)
	for _, test := range []struct {
		name      string
		operation string
		inputs    []inference.Input
		control   map[string]any
		function  string
	}{
		{
			name: "outpaint", operation: "outpaint", function: "expand",
			inputs:  []inference.Input{{Name: "source.png", MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(source, "image/png")}},
			control: map[string]any{"type": "outpaint", "source_width": 512, "source_height": 512, "top_scale": 1.25, "bottom_scale": 1.25, "left_scale": 1.5, "right_scale": 1.5},
		},
		{
			name: "inpaint", operation: "inpaint", function: "description_edit_with_mask",
			inputs: []inference.Input{
				{Name: "source.png", MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(source, "image/png")},
				{Name: "mask.png", MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(mask, "image/png")},
			},
			control: map[string]any{"type": "inpaint", "source_width": 512, "source_height": 512},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/services/aigc/image2image/image-synthesis":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					input := body["input"].(map[string]any)
					if input["function"] != test.function || !strings.HasPrefix(input["base_image_url"].(string), "data:image/png;base64,") {
						t.Fatalf("unexpected image edit input %#v", input)
					}
					if test.operation == "inpaint" && !strings.HasPrefix(input["mask_image_url"].(string), "data:image/png;base64,") {
						t.Fatalf("missing native mask input %#v", input)
					}
					if test.operation == "outpaint" {
						parameters := body["parameters"].(map[string]any)
						if parameters["left_scale"] != 1.5 || parameters["right_scale"] != 1.5 {
							t.Fatalf("missing outpaint geometry %#v", parameters)
						}
					}
					writeJSON(t, w, map[string]any{"output": map[string]any{"task_id": test.operation + "-task", "task_status": "PENDING"}})
				case "/api/v1/tasks/" + test.operation + "-task":
					writeJSON(t, w, map[string]any{"output": map[string]any{"task_status": "SUCCEEDED", "results": []any{map[string]any{"url": server.URL + "/edited.png"}}}})
				case "/edited.png":
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write([]byte("edited"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			control, _ := json.Marshal(test.control)
			result, err := bailian.New(bailian.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
				Runtime:   inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
				Target:    inference.Target{Kind: inference.TargetModel, ID: "wanx2.1-imageedit", Capability: inference.CapabilityImage},
				Operation: test.operation, Control: control, Prompt: "edit", Parameters: map[string]any{"n": 1}, Inputs: test.inputs,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			assertArtifactBytes(t, result.Artifacts[0], "edited")
		})
	}
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

func TestWanTextToVideoAcceptsOneAudioReference(t *testing.T) {
	t.Parallel()
	var submitted map[string]any
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/video-generation/video-synthesis":
			if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
				t.Fatal(err)
			}
			writeJSON(t, w, map[string]any{"output": map[string]any{"task_id": "wan-t2v-task"}})
		case "/api/v1/tasks/wan-t2v-task":
			writeJSON(t, w, map[string]any{"output": map[string]any{"task_status": "SUCCEEDED", "video_url": server.URL + "/wan.mp4"}})
		case "/wan.mp4":
			_, _ = w.Write([]byte("wan"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := bailian.New(bailian.Config{HTTPClient: server.Client(), PollInterval: time.Millisecond}).Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{ProviderCode: "aliyun_bailian", Endpoint: server.URL, APIKey: "secret"},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "wan2.7-t2v", Capability: inference.CapabilityVideo},
		Prompt:  "狼兽人推开异世界之门", Parameters: map[string]any{"ratio": "16:9", "duration": 6, "negative_prompt": "模糊"},
		Inputs: []inference.Input{{MediaType: "audio", URL: "https://assets.example/voice.mp3"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := submitted["input"].(map[string]any)
	if input["audio_url"] != "https://assets.example/voice.mp3" || input["negative_prompt"] != "模糊" {
		t.Fatalf("unexpected Wan text-to-video input %#v", input)
	}
	assertArtifactBytes(t, result.Artifacts[0], "wan")
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

func testPNG(t *testing.T, mask bool) []byte {
	t.Helper()
	background := color.RGBA{R: 230, G: 180, B: 120, A: 255}
	if mask {
		background = color.RGBA{A: 255}
	}
	value := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			pixel := background
			if mask && x >= 160 && x < 352 && y >= 160 && y < 352 {
				pixel = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			value.SetRGBA(x, y, pixel)
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
