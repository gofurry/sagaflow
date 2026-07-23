package zhipu

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestLiveProductionChain(t *testing.T) {
	if os.Getenv("ZHIPU_LIVE") != "1" {
		t.Skip("set ZHIPU_LIVE=1 to run real Zhipu requests")
	}
	key := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY"))
	if key == "" {
		t.Fatal("ZHIPU_API_KEY is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	driver := New(Config{PollInterval: 4 * time.Second})
	runtime := inference.Runtime{
		ProviderCode: providerCode, AdapterCode: providerCode,
		Endpoint: "https://open.bigmodel.cn/api/paas/v4", APIKey: key,
	}

	text, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("glm-4.7-flash", inference.CapabilityText, "chat"),
		Prompt:     "为一部东方奇幻漫剧写一句不超过三十字的开场旁白：海上灯塔在百年风暴中重新亮起。",
		Parameters: map[string]any{"max_tokens": 128, "temperature": 0.7, "thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value := artifactData(t, ctx, text.Artifacts[0]); len(value) == 0 {
		t.Fatal("text output is empty")
	}

	if os.Getenv("ZHIPU_LIVE_MEDIA") != "1" {
		return
	}
	image, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("glm-image", inference.CapabilityImage, "image_generation"),
		Prompt:     "东方奇幻动画电影概念图，暴雨夜的黑色海面中央矗立一座古老青铜灯塔，橙金灯火穿透青蓝风暴云，远处鲸群剪影，宽阔电影构图，无文字。",
		Parameters: map[string]any{"quality": "hd", "size": "1280x1280", "watermark_enabled": true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	imageData := artifactData(t, ctx, image.Artifacts[0])
	if len(imageData) == 0 {
		t.Fatal("image output is empty")
	}
	imageInput := inference.Input{
		Name: "storm-lighthouse.png", MediaType: "image", MIMEType: image.Artifacts[0].MIMEType,
		Content: inference.BytesContent(imageData, image.Artifacts[0].MIMEType),
	}

	vision, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("glm-5v-turbo", inference.CapabilityText, "chat"),
		Prompt: "分析画面的主体、光线、色彩和镜头构图，并给出三条可直接用于后续分镜的连续性约束。",
		Inputs: []inference.Input{imageInput}, Parameters: map[string]any{"max_tokens": 512, "thinking": "disabled"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value := artifactData(t, ctx, vision.Artifacts[0]); len(value) == 0 {
		t.Fatal("vision output is empty")
	}

	speech, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("glm-tts", inference.CapabilityAudio, "speech_generation"),
		Prompt:     "灯火穿过百年风暴，沉睡的航路终于再次苏醒。",
		Parameters: map[string]any{"voice": "tongtong", "response_format": "wav", "speed": 1.0, "volume": 1.0},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	audioData := artifactData(t, ctx, speech.Artifacts[0])
	if len(audioData) == 0 {
		t.Fatal("speech output is empty")
	}

	transcription, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("glm-asr-2512", inference.CapabilityText, "speech_recognition"),
		Inputs: []inference.Input{{
			Name: "narration.wav", MediaType: "audio", MIMEType: "audio/wav",
			Content: inference.BytesContent(audioData, "audio/wav"),
		}},
		Parameters: map[string]any{"prompt": "东方奇幻漫剧旁白"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value := artifactData(t, ctx, transcription.Artifacts[0]); len(value) == 0 {
		t.Fatal("transcription output is empty")
	}

	if os.Getenv("ZHIPU_LIVE_VIDEO") != "1" {
		return
	}
	video, err := driver.Execute(ctx, inference.Request{
		Runtime: runtime, Target: target("cogvideox-3", inference.CapabilityVideo, ""),
		Prompt: "镜头缓慢推向暴雨中的古老青铜灯塔，橙金灯火扫过海面，风暴云翻涌，鲸群剪影从远处掠过，保持主体造型和配色一致。",
		Inputs: []inference.Input{imageInput},
		Parameters: map[string]any{
			"quality": "speed", "with_audio": false, "watermark_enabled": true,
			"size": "1280x720", "fps": 30, "duration": 5,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value := artifactData(t, ctx, video.Artifacts[0]); len(value) == 0 {
		t.Fatal("video output is empty")
	}
}

func artifactData(t *testing.T, ctx context.Context, artifact inference.Artifact) []byte {
	t.Helper()
	if artifact.Content == nil {
		t.Fatal("artifact content is unavailable")
	}
	reader, _, err := artifact.Content.Open(ctx)
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
