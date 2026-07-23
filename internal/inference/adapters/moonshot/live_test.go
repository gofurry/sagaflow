package moonshot

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestLiveK3Text(t *testing.T) {
	if os.Getenv("MOONSHOT_LIVE") != "1" {
		t.Skip("set MOONSHOT_LIVE=1 to run")
	}
	driver, runtime := liveDriver(t)
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "kimi-k3", Capability: inference.CapabilityText},
		Prompt:  "为一部海港奇幻漫剧写一句不超过二十字的开场旁白。",
		Parameters: map[string]any{
			"max_completion_tokens": 256, "reasoning_effort": "low",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("K3 response: %s", readText(t, result))
}

func TestLiveK26Image(t *testing.T) {
	if os.Getenv("MOONSHOT_MEDIA_LIVE") != "1" {
		t.Skip("set MOONSHOT_MEDIA_LIVE=1 to run")
	}
	driver, runtime := liveDriver(t)
	imagePath := strings.TrimSpace(os.Getenv("MOONSHOT_IMAGE_PATH"))
	if imagePath == "" {
		t.Fatal("MOONSHOT_IMAGE_PATH is required")
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "kimi-k2.6", Capability: inference.CapabilityText},
		Prompt:  "用一句话描述画面主体、环境和色调。",
		Inputs: []inference.Input{{
			MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(data, "image/png"),
		}},
		Parameters: map[string]any{"max_tokens": 256, "thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("K2.6 image response: %s", readText(t, result))
}

func TestLiveK26Video(t *testing.T) {
	if os.Getenv("MOONSHOT_VIDEO_LIVE") != "1" {
		t.Skip("set MOONSHOT_VIDEO_LIVE=1 to run")
	}
	driver, runtime := liveDriver(t)
	videoPath := strings.TrimSpace(os.Getenv("MOONSHOT_VIDEO_PATH"))
	if videoPath == "" {
		t.Fatal("MOONSHOT_VIDEO_PATH is required")
	}
	data, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "kimi-k2.6", Capability: inference.CapabilityText},
		Prompt:  "简要说明视频中发生的动作。",
		Inputs: []inference.Input{{
			MediaType: "video", MIMEType: "video/mp4", Content: inference.BytesContent(data, "video/mp4"),
		}},
		Parameters: map[string]any{"max_tokens": 256, "thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("K2.6 video response: %s", readText(t, result))
}

func liveDriver(t *testing.T) (*Driver, inference.Runtime) {
	t.Helper()
	key := strings.TrimSpace(os.Getenv("MOONSHOT_API_KEY"))
	if key == "" {
		t.Fatal("MOONSHOT_API_KEY is required")
	}
	return New(&http.Client{Timeout: 10 * time.Minute}), inference.Runtime{
		ProviderCode: providerCode, Endpoint: "https://api.moonshot.cn/v1", APIKey: key,
	}
}

func readText(t *testing.T, result inference.Result) string {
	t.Helper()
	if len(result.Artifacts) == 0 {
		t.Fatal("response has no artifacts")
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		t.Fatal("response text is empty")
	}
	return text
}
