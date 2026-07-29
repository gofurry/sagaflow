package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestMiniMaxVoiceDesignLifecycleRequests(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		switch r.URL.Path {
		case "/v1/voice_design":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["prompt"] != "温暖、清晰的青年男声" || payload["preview_text"] != "欢迎来到灯塔。" || payload["voice_id"] != "SagaVoice01" {
				t.Fatalf("unexpected design payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"voice_id":"SagaVoice01","trial_audio":"6d7033","base_resp":{"status_code":0}}`))
		case "/v1/t2a_v2":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != "speech-2.8-hd" || payload["text"] != "欢迎来到灯塔。" {
				t.Fatalf("unexpected activation payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":{"audio":"6d7033"},"base_resp":{"status_code":0}}`))
		case "/v1/delete_voice":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["voice_id"] != "SagaVoice01" || payload["voice_type"] != "voice_generation" {
				t.Fatalf("unexpected delete payload: %#v", payload)
			}
			deleted = true
			_, _ = w.Write([]byte(`{"base_resp":{"status_code":0}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &VoiceService{httpClient: server.Client()}
	adapter := miniMaxVoiceAdapter{service: service}
	result, err := adapter.Create(context.Background(), voiceAdapterInput{
		Profile: db.VoiceProfile{Kind: "design", DesignPrompt: "温暖、清晰的青年男声"},
		Model:   db.Model{ModelID: "speech-2.8-hd"},
		Secret:  CredentialSecret{BaseURL: server.URL, APIKey: "secret"},
		VoiceID: "SagaVoice01", PreviewText: "欢迎来到灯塔。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.VoiceID != "SagaVoice01" || result.Preview == nil || string(result.Preview.Data) != "mp3" {
		t.Fatalf("unexpected MiniMax result: %#v", result)
	}
	if err := adapter.Delete(context.Background(), CredentialSecret{BaseURL: server.URL, APIKey: "secret"}, db.VoiceBinding{
		VoiceID: result.VoiceID, Operation: "design",
	}); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected MiniMax voice deletion")
	}
}

func TestAliyunVoiceDesignLifecycleRequests(t *testing.T) {
	deleted := false
	preview := base64.StdEncoding.EncodeToString([]byte("wav"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		if r.URL.Path != "/api/v1/services/audio/tts/customization" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Model string         `json:"model"`
			Input map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		switch payload.Input["action"] {
		case "create_voice":
			if payload.Model != "voice-enrollment" || payload.Input["target_model"] != "cosyvoice-v3.5-plus" ||
				payload.Input["prefix"] != "saga" || payload.Input["voice_prompt"] != "沉稳的中年女声" {
				t.Fatalf("unexpected Aliyun create payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"request_id":"request-1","output":{"voice_id":"cosyvoice-v3.5-plus-Saga-001","preview_audio":{"data":"` + preview + `"}}}`))
		case "delete_voice":
			if payload.Input["voice_id"] != "cosyvoice-v3.5-plus-Saga-001" {
				t.Fatalf("unexpected Aliyun delete payload: %#v", payload)
			}
			deleted = true
			_, _ = w.Write([]byte(`{"request_id":"request-2","output":{}}`))
		default:
			t.Fatalf("unexpected action: %#v", payload.Input)
		}
	}))
	defer server.Close()

	service := &VoiceService{httpClient: server.Client()}
	adapter := aliyunVoiceAdapter{service: service}
	result, err := adapter.Create(context.Background(), voiceAdapterInput{
		Profile: db.VoiceProfile{Kind: "design", DesignPrompt: "沉稳的中年女声"},
		Model:   db.Model{ModelID: "cosyvoice-v3.5-plus"},
		Secret:  CredentialSecret{BaseURL: server.URL, APIKey: "secret"},
		VoiceID: "saga", PreviewText: "欢迎来到灯塔。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.VoiceID != "cosyvoice-v3.5-plus-Saga-001" || result.Preview == nil || string(result.Preview.Data) != "wav" {
		t.Fatalf("unexpected Aliyun result: %#v", result)
	}
	if err := adapter.Delete(context.Background(), CredentialSecret{BaseURL: server.URL, APIKey: "secret"}, db.VoiceBinding{VoiceID: result.VoiceID}); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected Aliyun voice deletion")
	}
}

func TestBailianEndpointAvoidsDuplicateAPIVersion(t *testing.T) {
	if got := bailianEndpoint("https://dashscope.aliyuncs.com/api/v1", "/api/v1/services/audio/tts/customization"); got != "https://dashscope.aliyuncs.com/api/v1/services/audio/tts/customization" {
		t.Fatalf("unexpected endpoint: %s", got)
	}
}
