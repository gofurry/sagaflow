package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var voiceIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$`)

type VoiceService struct {
	store       *db.Store
	credentials *CredentialService
	storage     *storage.Manager
	httpClient  *http.Client
	log         *zap.Logger
	adapters    map[string]voiceAdapter
}

type VoiceFile struct {
	Name     string
	MIMEType string
	Data     []byte
}

func (s *VoiceService) uploadProviderFile(ctx context.Context, baseURL, key, purpose string, file VoiceFile) (int64, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", purpose); err != nil {
		return 0, err
	}
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return 0, err
	}
	if _, err := part.Write(file.Data); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/files/upload", &body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(data), 600))
	}
	var response struct {
		File struct {
			FileID int64 `json:"file_id"`
		} `json:"file"`
		BaseResp miniMaxBaseResponse `json:"base_resp"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return 0, fmt.Errorf("%w: decode upload response: %v", ErrProviderRequest, err)
	}
	if response.BaseResp.StatusCode != 0 || response.File.FileID == 0 {
		return 0, fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
	}
	return response.File.FileID, nil
}

func (s *VoiceService) activateVoice(ctx context.Context, baseURL, key, modelID, voiceID, text string) ([]byte, json.RawMessage, error) {
	payload := map[string]any{
		"model": modelID, "text": text, "stream": false, "output_format": "hex",
		"voice_setting":  map[string]any{"voice_id": voiceID, "speed": 1, "vol": 1, "pitch": 0},
		"audio_setting":  map[string]any{"sample_rate": 32000, "bitrate": 128000, "format": "mp3", "channel": 1},
		"aigc_watermark": false,
	}
	var response struct {
		Data *struct {
			Audio string `json:"audio"`
		} `json:"data"`
		ExtraInfo any                 `json:"extra_info"`
		TraceID   string              `json:"trace_id"`
		BaseResp  miniMaxBaseResponse `json:"base_resp"`
	}
	raw, err := s.doJSON(ctx, http.MethodPost, baseURL+"/v1/t2a_v2", key, payload, &response)
	if err != nil {
		return nil, nil, err
	}
	if response.BaseResp.StatusCode != 0 || response.Data == nil || response.Data.Audio == "" {
		return nil, nil, fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
	}
	data, err := hex.DecodeString(response.Data.Audio)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decode activation audio: %v", ErrProviderRequest, err)
	}
	return data, jsonSummary(raw, "trace_id", "extra_info"), nil
}

func (s *VoiceService) uploadSiliconFlowVoice(ctx context.Context, baseURL, key, modelID, customName, text string, file VoiceFile) (string, json.RawMessage, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for field, value := range map[string]string{"model": modelID, "customName": customName, "text": text} {
		if err := writer.WriteField(field, value); err != nil {
			return "", nil, err
		}
	}
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return "", nil, err
	}
	if _, err := part.Write(file.Data); err != nil {
		return "", nil, err
	}
	if err := writer.Close(); err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/uploads/audio/voice", &body)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", raw, fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(raw), 600))
	}
	var response struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || strings.TrimSpace(response.URI) == "" {
		return "", raw, fmt.Errorf("%w: SiliconFlow voice upload returned no uri", ErrProviderRequest)
	}
	return strings.TrimSpace(response.URI), jsonSummary(raw), nil
}

func (s *VoiceService) previewSiliconFlowVoice(ctx context.Context, baseURL, key, modelID, voiceID, text string) ([]byte, json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"model": modelID, "input": text, "voice": voiceID,
		"response_format": "mp3", "sample_rate": 44100, "stream": false,
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(data), 600))
	}
	metadata, _ := json.Marshal(map[string]any{
		"trace_id": resp.Header.Get("x-siliconcloud-trace-id"), "bytes": len(data),
	})
	return data, metadata, nil
}

func (s *VoiceService) deleteSiliconFlowVoice(ctx context.Context, baseURL, key, uri string) error {
	var response any
	_, err := s.doJSON(ctx, http.MethodPost, baseURL+"/audio/voice/deletions", key, map[string]any{"uri": uri}, &response)
	return err
}

func (s *VoiceService) uploadZhipuVoiceFile(ctx context.Context, baseURL, key string, file VoiceFile) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", "voice-clone-input"); err != nil {
		return "", err
	}
	part, err := writer.CreateFormFile("file", file.Name)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(file.Data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/files", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(raw), 600))
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("%w: Zhipu voice upload returned no file id", ErrProviderRequest)
	}
	return strings.TrimSpace(response.ID), nil
}

func (s *VoiceService) cloneZhipuVoice(ctx context.Context, baseURL, key, voiceName, sourceText, previewText, fileID string) (string, string, json.RawMessage, error) {
	payload := map[string]any{
		"model": "glm-tts-clone", "voice_name": voiceName, "input": previewText,
		"file_id": fileID, "request_id": uuid.NewString(),
	}
	if sourceText != "" {
		payload["text"] = sourceText
	}
	var response struct {
		Voice     string `json:"voice"`
		FileID    string `json:"file_id"`
		RequestID string `json:"request_id"`
	}
	raw, err := s.doJSON(ctx, http.MethodPost, baseURL+"/voice/clone", key, payload, &response)
	if err != nil {
		return "", "", nil, err
	}
	if strings.TrimSpace(response.Voice) == "" {
		return "", "", raw, fmt.Errorf("%w: Zhipu voice clone returned no voice id", ErrProviderRequest)
	}
	return strings.TrimSpace(response.Voice), strings.TrimSpace(response.FileID), jsonSummary(raw), nil
}

func (s *VoiceService) previewZhipuVoice(ctx context.Context, baseURL, key, voiceID, text string) ([]byte, json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{
		"model": "glm-tts", "input": text, "voice": voiceID, "response_format": "wav",
	})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "audio/wav")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(data), 600))
	}
	metadata, _ := json.Marshal(map[string]any{"bytes": len(data)})
	return data, metadata, nil
}

func (s *VoiceService) deleteZhipuVoice(ctx context.Context, baseURL, key, voiceID string) error {
	var response any
	_, err := s.doJSON(ctx, http.MethodPost, baseURL+"/voice/delete", key, map[string]any{
		"voice": voiceID, "request_id": uuid.NewString(),
	}, &response)
	return err
}

func (s *VoiceService) deleteZhipuFile(ctx context.Context, baseURL, key, fileID string) {
	if strings.TrimSpace(fileID) == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/files/"+fileID, nil)
	if err != nil {
		s.log.Warn("create Zhipu file deletion request", zap.String("file_id", fileID), zap.Error(err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.Warn("delete Zhipu clone input", zap.String("file_id", fileID), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 600))
		s.log.Warn("delete Zhipu clone input", zap.String("file_id", fileID), zap.Int("status", resp.StatusCode), zap.String("body", string(data)))
	}
}

func (s *VoiceService) deleteProviderFile(ctx context.Context, baseURL, key, purpose string, id int64) {
	if id == 0 {
		return
	}
	var response any
	if _, err := s.doJSON(ctx, http.MethodPost, baseURL+"/v1/files/delete", key, map[string]any{"file_id": id, "purpose": purpose}, &response); err != nil {
		s.log.Warn("delete MiniMax clone input", zap.Int64("file_id", id), zap.Error(err))
	}
}

func (s *VoiceService) doJSON(ctx context.Context, method, url, key string, payload any, out any) (json.RawMessage, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, fmt.Errorf("%w: status=%d body=%s", ErrProviderRequest, resp.StatusCode, truncate(string(raw), 600))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return raw, fmt.Errorf("%w: decode response: %v", ErrProviderRequest, err)
	}
	return raw, nil
}

type miniMaxBaseResponse struct {
	StatusCode int64  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

func supportedCloneAudio(mimeType, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".mp3" || ext == ".m4a" || ext == ".wav" {
		return true
	}
	mimeType = strings.ToLower(strings.Split(mimeType, ";")[0])
	return mimeType == "audio/mpeg" || mimeType == "audio/mp4" || mimeType == "audio/x-m4a" || mimeType == "audio/wav" || mimeType == "audio/x-wav"
}

func supportedZhipuCloneAudio(mimeType, name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".mp3" || ext == ".wav" {
		return true
	}
	mimeType = strings.ToLower(strings.Split(mimeType, ";")[0])
	return mimeType == "audio/mpeg" || mimeType == "audio/wav" || mimeType == "audio/x-wav"
}
