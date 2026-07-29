package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestLiveConfiguredVoiceDesignProviders(t *testing.T) {
	if os.Getenv("SAGAFLOW_LIVE_VOICE_DESIGN") != "1" {
		t.Skip("set SAGAFLOW_LIVE_VOICE_DESIGN=1 to test configured MiniMax and Aliyun voice design")
	}
	dataDir := strings.TrimSpace(os.Getenv("SAGAFLOW_LIVE_DATA_DIR"))
	if dataDir == "" {
		t.Fatal("SAGAFLOW_LIVE_DATA_DIR is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	service := &VoiceService{httpClient: &http.Client{Timeout: 3 * time.Minute}}

	t.Run("MiniMaxSpeech28Design", func(t *testing.T) {
		secret := configuredVoiceSecret(t, ctx, store, credentials, ProviderMiniMax)
		adapter := miniMaxVoiceAdapter{service: service}
		result, err := adapter.Create(ctx, voiceAdapterInput{
			Profile: db.VoiceProfile{Kind: "design", DesignPrompt: "年轻、温暖、清晰的中文女声，语速自然，适合漫剧旁白。"},
			Model:   db.Model{ModelID: "speech-2.8-hd"}, Secret: secret,
			VoiceID: fmt.Sprintf("SagaDev%d", time.Now().Unix()), PreviewText: "欢迎使用 SagaFlow 音色测试。",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Preview == nil || len(result.Preview.Data) == 0 {
			t.Fatal("MiniMax returned no preview audio")
		}
		if err := adapter.Delete(context.Background(), secret, db.VoiceBinding{
			VoiceID: result.VoiceID, Operation: "design",
		}); err != nil {
			t.Errorf("delete MiniMax test voice: %v", err)
		}
	})

	t.Run("AliyunCosyVoice35Design", func(t *testing.T) {
		secret := configuredVoiceSecret(t, ctx, store, credentials, ProviderAliyunBailian)
		adapter := aliyunVoiceAdapter{service: service}
		result, err := adapter.Create(ctx, voiceAdapterInput{
			Profile: db.VoiceProfile{Kind: "design", DesignPrompt: "年轻、温暖、清晰的中文女声，语速自然，适合漫剧旁白。"},
			Model:   db.Model{ModelID: "cosyvoice-v3.5-plus"}, Secret: secret,
			VoiceID: "sagadev", PreviewText: "欢迎使用 SagaFlow 音色测试。",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Preview == nil || len(result.Preview.Data) == 0 {
			t.Fatal("Aliyun returned no preview audio")
		}
		if err := adapter.Delete(context.Background(), secret, db.VoiceBinding{VoiceID: result.VoiceID, Operation: "design"}); err != nil {
			t.Errorf("delete Aliyun test voice: %v", err)
		}
	})

	t.Run("SiliconFlowAudioCatalog", func(t *testing.T) {
		probeConfiguredModelCatalog(t, ctx, service.httpClient, configuredVoiceSecret(t, ctx, store, credentials, ProviderSiliconFlow), "cosyvoice")
	})

	t.Run("ZhipuAudioCatalog", func(t *testing.T) {
		// Zhipu's authenticated catalog currently omits GLM-TTS even though its
		// dedicated clone and speech endpoints remain documented and supported.
		probeConfiguredModelCatalog(t, ctx, service.httpClient, configuredVoiceSecret(t, ctx, store, credentials, ProviderZhipu), "")
	})
}

func configuredVoiceSecret(t *testing.T, ctx context.Context, store *db.Store, credentials *CredentialService, providerCode string) CredentialSecret {
	t.Helper()
	provider, err := store.GetModelProviderByCode(ctx, providerCode)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := credentials.SecretForProvider(ctx, provider)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func probeConfiguredModelCatalog(t *testing.T, ctx context.Context, client *http.Client, secret CredentialSecret, expectedModel string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(secret.BaseURL, "/")+"/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret.APIKey)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("catalog probe status=%d", response.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil || len(payload.Data) == 0 {
		t.Fatalf("catalog probe returned no models")
	}
	if expectedModel == "" {
		return
	}
	for _, model := range payload.Data {
		if strings.Contains(strings.ToLower(model.ID), expectedModel) {
			return
		}
	}
	t.Fatalf("catalog probe did not expose expected audio model %q", expectedModel)
}
