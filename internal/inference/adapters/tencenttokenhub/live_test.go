package tencenttokenhub

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func TestLiveText(t *testing.T) {
	if os.Getenv("TENCENT_TOKENHUB_LIVE") != "1" {
		t.Skip("set TENCENT_TOKENHUB_LIVE=1 to run")
	}
	key := strings.TrimSpace(os.Getenv("TENCENT_TOKENHUB_API_KEY"))
	if key == "" {
		t.Fatal("TENCENT_TOKENHUB_API_KEY is required")
	}
	driver := New(Config{HTTPClient: &http.Client{Timeout: 2 * time.Minute}})
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: "https://tokenhub.tencentmaas.com/v1", APIKey: key},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy3", Capability: inference.CapabilityText},
		Prompt:  "为一部都市奇幻漫剧写一句不超过二十字的片头旁白。",
		Parameters: map[string]any{
			"max_tokens": 64, "temperature": .3, "thinking": "disabled",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, _, err := result.Artifacts[0].Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	text, _ := io.ReadAll(reader)
	if strings.TrimSpace(string(text)) == "" {
		t.Fatal("empty live response")
	}
	t.Logf("hy3 response: %s", strings.TrimSpace(string(text)))
}

func TestLiveImageVisionAndVideo(t *testing.T) {
	if os.Getenv("TENCENT_TOKENHUB_MEDIA_LIVE") != "1" {
		t.Skip("set TENCENT_TOKENHUB_MEDIA_LIVE=1 to run")
	}
	key := strings.TrimSpace(os.Getenv("TENCENT_TOKENHUB_API_KEY"))
	if key == "" {
		t.Fatal("TENCENT_TOKENHUB_API_KEY is required")
	}
	driver := New(Config{
		HTTPClient: &http.Client{Timeout: 10 * time.Minute}, PollInterval: 3 * time.Second,
	})
	runtime := inference.Runtime{Endpoint: "https://tokenhub.tencentmaas.com/v1", APIKey: key}
	image, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-image-lite", Capability: inference.CapabilityImage},
		Prompt:  "雨后旧码头的暖色电影概念图，远处灯塔，前景木船，横向构图",
		Parameters: map[string]any{
			"resolution": "1280:720", "watermark": false, "response_format": "url",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	imageData, imageMIME := readArtifact(t, image.Artifacts[0])
	if !strings.HasPrefix(imageMIME, "image/") {
		t.Fatalf("unexpected image MIME %q", imageMIME)
	}
	t.Logf("image lite returned %d bytes", len(imageData))

	vision, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-vision-2.0-instruct", Capability: inference.CapabilityText},
		Prompt:  "用一句话概括画面的主体、环境与色调。",
		Inputs: []inference.Input{{
			MediaType: "image", MIMEType: imageMIME, Content: inference.BytesContent(imageData, imageMIME),
		}},
		Parameters: map[string]any{"max_tokens": 128, "temperature": .2},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	visionText, _ := readArtifact(t, vision.Artifacts[0])
	t.Logf("vision response: %s", strings.TrimSpace(string(visionText)))

	if os.Getenv("TENCENT_TOKENHUB_VIDEO_LIVE") != "1" {
		return
	}
	video, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-video-1.5", Capability: inference.CapabilityVideo},
		Prompt:  "镜头缓慢推向远处灯塔，水面倒影轻微晃动，电影感运镜",
		Inputs: []inference.Input{{
			MediaType: "image", MIMEType: imageMIME, Content: inference.BytesContent(imageData, imageMIME),
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	videoData, videoMIME := readArtifact(t, video.Artifacts[0])
	t.Logf("video returned %d bytes as %s", len(videoData), videoMIME)
}

func TestLiveImageV3(t *testing.T) {
	if os.Getenv("TENCENT_TOKENHUB_IMAGE_V3_LIVE") != "1" {
		t.Skip("set TENCENT_TOKENHUB_IMAGE_V3_LIVE=1 to run")
	}
	key := strings.TrimSpace(os.Getenv("TENCENT_TOKENHUB_API_KEY"))
	if key == "" {
		t.Fatal("TENCENT_TOKENHUB_API_KEY is required")
	}
	driver := New(Config{HTTPClient: &http.Client{Timeout: 10 * time.Minute}, PollInterval: 3 * time.Second})
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: "https://tokenhub.tencentmaas.com/v1", APIKey: key},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-image-v3.0", Capability: inference.CapabilityImage},
		Prompt:  "暖色剪纸风格的山海城市天际线，简洁构图",
		Parameters: map[string]any{
			"resolution": "1024:1024", "watermark": false, "revise": true,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, mimeType := readArtifact(t, result.Artifacts[0])
	t.Logf("image v3 returned %d bytes as %s", len(data), mimeType)
}

func TestLiveRoleAndVITA(t *testing.T) {
	if os.Getenv("TENCENT_TOKENHUB_MULTIMODAL_LIVE") != "1" {
		t.Skip("set TENCENT_TOKENHUB_MULTIMODAL_LIVE=1 to run")
	}
	key := strings.TrimSpace(os.Getenv("TENCENT_TOKENHUB_API_KEY"))
	if key == "" {
		t.Fatal("TENCENT_TOKENHUB_API_KEY is required")
	}
	driver := New(Config{HTTPClient: &http.Client{Timeout: 3 * time.Minute}})
	runtime := inference.Runtime{Endpoint: "https://tokenhub.tencentmaas.com/v1", APIKey: key}
	role, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hunyuan-role-latest", Capability: inference.CapabilityText},
		Prompt:  "你是守护旧灯塔的狼兽人，请用一句简短对白欢迎远航归来的朋友。",
		Parameters: map[string]any{
			"max_tokens": 64, "temperature": .7,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	roleText, _ := readArtifact(t, role.Artifacts[0])
	t.Logf("role response: %s", strings.TrimSpace(string(roleText)))

	vita, err := driver.Execute(context.Background(), inference.Request{
		Runtime: runtime,
		Target:  inference.Target{Kind: inference.TargetModel, ID: "youtu-vita", Capability: inference.CapabilityText},
		Prompt:  "指出图片的主要颜色。",
		Inputs: []inference.Input{{
			MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(samplePNG(t), "image/png"),
		}},
		Parameters: map[string]any{"max_tokens": 64, "temperature": .2},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	vitaText, _ := readArtifact(t, vita.Artifacts[0])
	t.Logf("VITA response: %s", strings.TrimSpace(string(vitaText)))
}

func TestLiveVideo(t *testing.T) {
	if os.Getenv("TENCENT_TOKENHUB_VIDEO_LIVE") != "1" {
		t.Skip("set TENCENT_TOKENHUB_VIDEO_LIVE=1 to run")
	}
	key := strings.TrimSpace(os.Getenv("TENCENT_TOKENHUB_API_KEY"))
	if key == "" {
		t.Fatal("TENCENT_TOKENHUB_API_KEY is required")
	}
	driver := New(Config{HTTPClient: &http.Client{Timeout: 15 * time.Minute}, PollInterval: 5 * time.Second})
	result, err := driver.Execute(context.Background(), inference.Request{
		Runtime: inference.Runtime{Endpoint: "https://tokenhub.tencentmaas.com/v1", APIKey: key},
		Target:  inference.Target{Kind: inference.TargetModel, ID: "hy-video-1.5", Capability: inference.CapabilityVideo},
		Prompt:  "黄昏海面上的小船缓慢驶向灯塔，镜头平稳向前推进，暖色电影光影",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, mimeType := readArtifact(t, result.Artifacts[0])
	t.Logf("video returned %d bytes as %s", len(data), mimeType)
}

func readArtifact(t *testing.T, artifact inference.Artifact) ([]byte, string) {
	t.Helper()
	reader, info, err := artifact.Content.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	mimeType := strings.TrimSpace(strings.Split(info.MIMEType, ";")[0])
	if mimeType == "" {
		mimeType = strings.TrimSpace(strings.Split(artifact.MIMEType, ";")[0])
	}
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = parsed
	}
	return data, mimeType
}

func samplePNG(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			canvas.Set(x, y, color.RGBA{R: 230, G: 120, B: 45, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
