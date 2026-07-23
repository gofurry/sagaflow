package service

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLiveZhipuVoiceLifecycle(t *testing.T) {
	if os.Getenv("ZHIPU_LIVE_VOICE") != "1" {
		t.Skip("set ZHIPU_LIVE_VOICE=1 to run the real Zhipu voice lifecycle")
	}
	key := strings.TrimSpace(os.Getenv("ZHIPU_API_KEY"))
	if key == "" {
		t.Fatal("ZHIPU_API_KEY is required")
	}
	sourcePath := strings.TrimSpace(os.Getenv("ZHIPU_VOICE_SOURCE_PATH"))
	sourceText := strings.TrimSpace(os.Getenv("ZHIPU_VOICE_SOURCE_TEXT"))
	if sourcePath == "" || sourceText == "" {
		t.Skip("set ZHIPU_VOICE_SOURCE_PATH and ZHIPU_VOICE_SOURCE_TEXT to a consented human voice sample")
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	baseURL := "https://open.bigmodel.cn/api/paas/v4"
	service := &VoiceService{httpClient: &http.Client{Timeout: 5 * time.Minute}}
	extension := strings.ToLower(filepath.Ext(sourcePath))
	mimeType := "audio/mpeg"
	if extension == ".wav" {
		mimeType = "audio/wav"
	}
	fileID, err := service.uploadZhipuVoiceFile(ctx, baseURL, key, VoiceFile{
		Name: filepath.Base(sourcePath), MIMEType: mimeType, Data: source,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.deleteZhipuFile(context.Background(), baseURL, key, fileID)
	voiceName := "SagaFlowVoice" + strconv.FormatInt(time.Now().Unix(), 10)
	voiceID, _, _, err := service.cloneZhipuVoice(
		ctx, baseURL, key, voiceName, sourceText,
		"你好，欢迎回来，愿你拥有愉快的一天。", fileID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.deleteZhipuVoice(context.Background(), baseURL, key, voiceID); err != nil {
			t.Errorf("delete cloned voice: %v", err)
		}
	}()
	preview, _, err := service.previewZhipuVoice(
		ctx, baseURL, key, voiceID, "你好，欢迎回来，愿你拥有愉快的一天。",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) == 0 {
		t.Fatal("cloned voice preview is empty")
	}
}
