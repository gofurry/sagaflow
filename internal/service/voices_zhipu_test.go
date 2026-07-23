package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestZhipuVoiceLifecycleRequests(t *testing.T) {
	const voiceID = "voice_clone_remote_001"
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		switch r.URL.Path {
		case "/v4/files":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("purpose") != "voice-clone-input" {
				t.Fatalf("unexpected purpose: %q", r.FormValue("purpose"))
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(file)
			file.Close()
			if string(data) != "audio" {
				t.Fatalf("unexpected upload data: %q", data)
			}
			_, _ = w.Write([]byte(`{"id":"file-input-1"}`))
		case "/v4/voice/clone":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != "glm-tts-clone" || payload["voice_name"] != "SagaVoice01" ||
				payload["file_id"] != "file-input-1" || payload["text"] != "海风穿过灯塔。" {
				t.Fatalf("unexpected clone payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"voice":"` + voiceID + `","file_id":"file-preview-1","request_id":"request-1"}`))
		case "/v4/audio/speech":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != "glm-tts" || payload["voice"] != voiceID || payload["input"] != "灯塔重新亮起。" {
				t.Fatalf("unexpected preview payload: %#v", payload)
			}
			if payload["response_format"] != "wav" {
				t.Fatalf("unexpected preview format: %#v", payload)
			}
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("wav"))
		case "/v4/voice/delete":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["voice"] != voiceID {
				t.Fatalf("unexpected deletion voice: %#v", payload)
			}
			deleted = true
			_, _ = w.Write([]byte(`{"voice":"` + voiceID + `"}`))
		case "/v4/files/file-input-1":
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected file deletion method: %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &VoiceService{httpClient: server.Client()}
	fileID, err := service.uploadZhipuVoiceFile(context.Background(), server.URL+"/v4", "secret", VoiceFile{
		Name: "voice.wav", MIMEType: "audio/wav", Data: []byte("audio"),
	})
	if err != nil {
		t.Fatal(err)
	}
	voice, previewFileID, metadata, err := service.cloneZhipuVoice(
		context.Background(), server.URL+"/v4", "secret", "SagaVoice01",
		"海风穿过灯塔。", "灯塔重新亮起。", fileID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if voice != voiceID || previewFileID != "file-preview-1" || !json.Valid(metadata) {
		t.Fatalf("unexpected clone result: voice=%q preview=%q metadata=%s", voice, previewFileID, metadata)
	}
	preview, previewMetadata, err := service.previewZhipuVoice(context.Background(), server.URL+"/v4", "secret", voice, "灯塔重新亮起。")
	if err != nil {
		t.Fatal(err)
	}
	if string(preview) != "wav" || !json.Valid(previewMetadata) {
		t.Fatalf("unexpected preview: %q metadata=%s", preview, previewMetadata)
	}
	if err := service.deleteZhipuVoice(context.Background(), server.URL+"/v4", "secret", voice); err != nil {
		t.Fatal(err)
	}
	service.deleteZhipuFile(context.Background(), server.URL+"/v4", "secret", fileID)
	if !deleted {
		t.Fatal("expected remote voice deletion")
	}
}
