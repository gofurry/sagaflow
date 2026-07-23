package siliconflow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

// TestLiveProductionChain is intentionally skipped during normal test runs.
// It exercises the public SiliconFlow service through the same Adapter used by
// SagaFlow. Run it with SILICONFLOW_API_KEY set; video is additionally guarded
// by SILICONFLOW_RUN_VIDEO=1 because it is slower and billable.
func TestLiveProductionChain(t *testing.T) {
	key := os.Getenv("SILICONFLOW_API_KEY")
	if key == "" {
		t.Skip("SILICONFLOW_API_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	driver := New(Config{
		HTTPClient:   &http.Client{Timeout: 10 * time.Minute},
		PollInterval: 5 * time.Second,
	})
	runtime := inference.Runtime{
		ProviderCode: providerCode,
		AdapterCode:  providerCode,
		Endpoint:     "https://api.siliconflow.cn/v1",
		APIKey:       key,
	}
	discovery, err := driver.Discover(ctx, runtime)
	if err != nil {
		t.Fatal(err)
	}
	visible := make(map[string]bool, len(discovery.Models))
	for _, model := range discovery.Models {
		visible[model.Name] = true
	}
	for _, modelID := range []string{
		"deepseek-ai/DeepSeek-V4-Flash",
		"Tongyi-MAI/Z-Image-Turbo",
		"Qwen/Qwen-Image-Edit-2509",
		"FunAudioLLM/CosyVoice2-0.5B",
		"FunAudioLLM/SenseVoiceSmall",
		"Wan-AI/Wan2.2-I2V-A14B",
	} {
		if !visible[modelID] {
			t.Fatalf("expected %s in live discovery", modelID)
		}
	}
	if os.Getenv("SILICONFLOW_DISCOVERY_ONLY") == "1" {
		return
	}
	videoOnly := os.Getenv("SILICONFLOW_VIDEO_ONLY") == "1"

	if !videoOnly {
		text := executeLive(t, ctx, driver, runtime, inference.Request{
			Target: liveTarget("deepseek-ai/DeepSeek-V4-Flash", inference.CapabilityText, "chat"),
			Prompt: "用一句简洁的中文描写清晨海雾中的灯塔。",
			Parameters: map[string]any{
				"max_tokens": 64, "temperature": 0.5, "enable_thinking": false,
			},
		})
		if len(text) < 3 {
			t.Fatalf("text response is unexpectedly short: %d bytes", len(text))
		}
	}

	image := executeLive(t, ctx, driver, runtime, inference.Request{
		Target: liveTarget("Tongyi-MAI/Z-Image-Turbo", inference.CapabilityImage, "image_generation"),
		Prompt: "电影概念设计，清晨海雾中的古老灯塔，暖色灯光穿透青灰色雾气，远处礁石与海鸟，宽阔构图，无文字",
		Parameters: map[string]any{
			"image_size": "1024x1024", "seed": 20260723,
		},
	})
	if len(image) < 1024 {
		t.Fatalf("image response is unexpectedly small: %d bytes", len(image))
	}

	videoImage := image
	if !videoOnly {
		edited := executeLive(t, ctx, driver, runtime, inference.Request{
			Target: liveTarget("Qwen/Qwen-Image-Edit-2509", inference.CapabilityImage, "image_edit"),
			Prompt: "保留原始构图，将时间改为日落，灯塔亮起橘色灯光，增加轻微电影胶片质感，不添加文字",
			Inputs: []inference.Input{{
				Name: "lighthouse.png", MediaType: "image", MIMEType: "image/png",
				Content: inference.BytesContent(image, "image/png"),
			}},
			Parameters: map[string]any{"seed": 20260724},
		})
		if len(edited) < 1024 {
			t.Fatalf("edited image response is unexpectedly small: %d bytes", len(edited))
		}
		videoImage = edited

		const speechText = "潮声渐近，远方的灯塔终于穿透了清晨的海雾。"
		audio := executeLive(t, ctx, driver, runtime, inference.Request{
			Target: liveTarget("FunAudioLLM/CosyVoice2-0.5B", inference.CapabilityAudio, "speech_generation"),
			Prompt: speechText,
			Parameters: map[string]any{
				"voice": "FunAudioLLM/CosyVoice2-0.5B:alex", "response_format": "mp3",
				"sample_rate": 44100, "speed": 1, "gain": 0,
			},
		})
		if len(audio) < 512 {
			t.Fatalf("audio response is unexpectedly small: %d bytes", len(audio))
		}

		transcript := executeLive(t, ctx, driver, runtime, inference.Request{
			Target: liveTarget("FunAudioLLM/SenseVoiceSmall", inference.CapabilityText, "speech_recognition"),
			Inputs: []inference.Input{{
				Name: "narration.mp3", MediaType: "audio", MIMEType: "audio/mpeg",
				Content: inference.BytesContent(audio, "audio/mpeg"),
			}},
		})
		if len(transcript) < 3 {
			t.Fatalf("transcription response is unexpectedly short: %d bytes", len(transcript))
		}
	}

	if os.Getenv("SILICONFLOW_RUN_VIDEO") == "1" {
		video := executeLive(t, ctx, driver, runtime, inference.Request{
			Target: liveTarget("Wan-AI/Wan2.2-I2V-A14B", inference.CapabilityVideo, "image_to_video"),
			Prompt: "海雾缓慢流动，灯塔光束平稳扫过海面，镜头轻微向前推进，电影质感，动作自然稳定",
			Inputs: []inference.Input{{
				Name: "lighthouse-video-reference.png", MediaType: "image", MIMEType: "image/png",
				Content: inference.BytesContent(videoImage, "image/png"),
			}},
			Parameters: map[string]any{
				"image_size": "960x960", "seed": 20260725,
			},
		})
		if len(video) < 4096 {
			t.Fatalf("video response is unexpectedly small: %d bytes", len(video))
		}
	}
}

func executeLive(t *testing.T, ctx context.Context, driver *Driver, runtime inference.Runtime, request inference.Request) []byte {
	t.Helper()
	request.Runtime = runtime
	result, err := driver.Execute(ctx, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Content == nil {
		t.Fatalf("unexpected artifact result: %+v", result.Artifacts)
	}
	reader, _, err := result.Artifacts[0].Content.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func liveTarget(id string, capability inference.Capability, task string) inference.Target {
	return inference.Target{
		Kind: inference.TargetModel, ID: id, Capability: capability,
		Spec: json.RawMessage(`{"task":"` + task + `"}`),
	}
}
