package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiliconFlowVoiceLifecycleRequests(t *testing.T) {
	const voiceURI = "speech:SagaVoice01:remote-id"
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		switch r.URL.Path {
		case "/v1/uploads/audio/voice":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("model") != "FunAudioLLM/CosyVoice2-0.5B" ||
				r.FormValue("customName") != "SagaVoice01" ||
				r.FormValue("text") != "海风穿过灯塔。" {
				t.Fatalf("unexpected upload fields: %#v", r.Form)
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
			_ = json.NewEncoder(w).Encode(map[string]any{"uri": voiceURI})
		case "/v1/audio/speech":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["voice"] != voiceURI || payload["input"] != "灯塔重新亮起。" {
				t.Fatalf("unexpected preview payload: %#v", payload)
			}
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("x-siliconcloud-trace-id", "trace-1")
			_, _ = w.Write([]byte("mp3"))
		case "/v1/audio/voice/deletions":
			var payload struct {
				URI string `json:"uri"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.URI != voiceURI {
				t.Fatalf("unexpected deletion uri: %q", payload.URI)
			}
			deleted = true
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &VoiceService{httpClient: server.Client()}
	uri, metadata, err := service.uploadSiliconFlowVoice(
		context.Background(), server.URL+"/v1", "secret",
		"FunAudioLLM/CosyVoice2-0.5B", "SagaVoice01", "海风穿过灯塔。",
		VoiceFile{Name: "voice.mp3", MIMEType: "audio/mpeg", Data: []byte("audio")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if uri != voiceURI || !json.Valid(metadata) {
		t.Fatalf("unexpected upload result: uri=%q metadata=%s", uri, metadata)
	}
	preview, previewMetadata, err := service.previewSiliconFlowVoice(
		context.Background(), server.URL+"/v1", "secret",
		"FunAudioLLM/CosyVoice2-0.5B", uri, "灯塔重新亮起。",
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(preview) != "mp3" || !json.Valid(previewMetadata) {
		t.Fatalf("unexpected preview: %q metadata=%s", preview, previewMetadata)
	}
	if err := service.deleteSiliconFlowVoice(context.Background(), server.URL+"/v1", "secret", uri); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected remote voice deletion")
	}
}
