package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/bailian"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

// TestLiveConfiguredPreciseImageEditing is opt-in because it calls a paid
// provider. It deliberately reads an encrypted development credential from the
// selected data directory instead of accepting or printing a plaintext key.
func TestLiveConfiguredPreciseImageEditing(t *testing.T) {
	if os.Getenv("SAGAFLOW_LIVE_IMAGE_EDIT") != "1" {
		t.Skip("set SAGAFLOW_LIVE_IMAGE_EDIT=1 to test configured Aliyun image editing")
	}
	dataDir := strings.TrimSpace(os.Getenv("SAGAFLOW_LIVE_DATA_DIR"))
	sourcePath := strings.TrimSpace(os.Getenv("SAGAFLOW_LIVE_SOURCE"))
	outputDir := strings.TrimSpace(os.Getenv("SAGAFLOW_LIVE_OUTPUT_DIR"))
	if dataDir == "" || sourcePath == "" || outputDir == "" {
		t.Fatal("SAGAFLOW_LIVE_DATA_DIR, SAGAFLOW_LIVE_SOURCE, and SAGAFLOW_LIVE_OUTPUT_DIR are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	database, err := sqlite.Open(ctx, filepath.Join(dataDir, "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := db.New(database)
	credentials, err := NewCredentialService(store, filepath.Join(dataDir, "secrets", "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	secret := configuredImageEditSecret(t, ctx, store, credentials)
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" && format != "jpeg" {
		t.Fatalf("live source must be PNG or JPEG, got %s", format)
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	mimeType := "image/" + format
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}

	driver := bailian.New(bailian.Config{
		HTTPClient:   &http.Client{Timeout: 8 * time.Minute},
		PollInterval: 2 * time.Second,
	})
	runtime := inference.Runtime{
		ProviderCode: ProviderAliyunBailian,
		AdapterCode:  ProviderAliyunBailian,
		Endpoint:     secret.BaseURL,
		APIKey:       secret.APIKey,
	}
	target := inference.Target{Kind: inference.TargetModel, ID: "wanx2.1-imageedit", Capability: inference.CapabilityImage}
	baseInput := inference.Input{Name: filepath.Base(sourcePath), MediaType: "image", MIMEType: mimeType, Content: inference.BytesContent(source, mimeType)}

	t.Run("Outpaint", func(t *testing.T) {
		control := mustJSON(t, map[string]any{
			"type": "outpaint", "source_width": width, "source_height": height,
			"top_scale": 1.1, "bottom_scale": 1.1, "left_scale": 1.15, "right_scale": 1.15,
		})
		result, err := driver.Execute(ctx, inference.Request{
			Runtime: runtime, Target: target, Operation: "outpaint", Control: control,
			Prompt: "保持现有 SagaFlow 标志、文字、配色和构图完整不变，仅向四周自然延续浅暖白背景，扩展区域干净协调。",
			Inputs: []inference.Input{baseInput}, Parameters: map[string]any{"n": 1, "watermark": false},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		writeLiveArtifact(t, ctx, result, filepath.Join(outputDir, "outpaint.png"))
	})

	t.Run("Inpaint", func(t *testing.T) {
		mask := binaryMaskPNG(t, width, height)
		control := mustJSON(t, map[string]any{"type": "inpaint", "source_width": width, "source_height": height})
		result, err := driver.Execute(ctx, inference.Request{
			Runtime: runtime, Target: target, Operation: "inpaint", Control: control,
			Prompt: "将遮罩区域自然修复为干净的浅暖白背景，严格保持遮罩外的 SagaFlow 标志、橙色胶片图形、颜色和构图不变。",
			Inputs: []inference.Input{baseInput, {
				Name: "mask.png", MediaType: "image", MIMEType: "image/png", Content: inference.BytesContent(mask, "image/png"),
			}}, Parameters: map[string]any{"n": 1, "watermark": false},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		writeLiveArtifact(t, ctx, result, filepath.Join(outputDir, "inpaint.png"))
	})
}

func configuredImageEditSecret(t *testing.T, ctx context.Context, store *db.Store, credentials *CredentialService) CredentialSecret {
	t.Helper()
	provider, err := store.GetModelProviderByCode(ctx, ProviderAliyunBailian)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := credentials.SecretForProvider(ctx, provider)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func binaryMaskPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	mask := image.NewGray(image.Rect(0, 0, width, height))
	for y := height * 2 / 3; y < height-12; y++ {
		for x := width / 5; x < width*4/5; x++ {
			mask.SetGray(x, y, color.Gray{Y: 0xff})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, mask); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeLiveArtifact(t *testing.T, ctx context.Context, result inference.Result, path string) {
	t.Helper()
	if len(result.Artifacts) != 1 || result.Artifacts[0].Content == nil {
		t.Fatalf("provider returned %d usable artifacts, want 1", len(result.Artifacts))
	}
	reader, info, err := result.Artifacts[0].Content.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if written == 0 {
		t.Fatal("provider returned an empty image")
	}
	t.Logf("saved %s (%s, %s)", path, info.MIMEType, fmt.Sprint(written)+" bytes")
}
