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
	"strconv"
	"strings"
	"time"

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
}

type VoiceFile struct {
	Name     string
	MIMEType string
	Data     []byte
}

type CreateVoiceProfileInput struct {
	ModelID                 uuid.UUID
	Name                    string
	Description             string
	VoiceID                 string
	Source                  VoiceFile
	Prompt                  *VoiceFile
	PromptText              string
	PreviewText             string
	NeedNoiseReduction      bool
	NeedVolumeNormalization bool
}

func NewVoiceService(store *db.Store, credentials *CredentialService, objectStore *storage.Manager, log *zap.Logger) *VoiceService {
	if log == nil {
		log = zap.NewNop()
	}
	return &VoiceService{
		store: store, credentials: credentials, storage: objectStore,
		httpClient: &http.Client{Timeout: 5 * time.Minute}, log: log,
	}
}

func (s *VoiceService) Create(ctx context.Context, input CreateVoiceProfileInput) (db.VoiceProfile, error) {
	if input.ModelID == uuid.Nil {
		return db.VoiceProfile{}, fmt.Errorf("%w: model_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(input.Name) == "" || !voiceIDPattern.MatchString(strings.TrimSpace(input.VoiceID)) {
		return db.VoiceProfile{}, fmt.Errorf("%w: name and a valid 8-256 character voice_id are required", ErrInvalidInput)
	}
	if len(input.Source.Data) == 0 || len(input.Source.Data) > 20<<20 {
		return db.VoiceProfile{}, fmt.Errorf("%w: clone audio is required and must not exceed 20 MB", ErrInvalidInput)
	}
	if !supportedCloneAudio(input.Source.MIMEType, input.Source.Name) {
		return db.VoiceProfile{}, fmt.Errorf("%w: clone audio must be mp3, m4a or wav", ErrInvalidInput)
	}
	if input.Prompt != nil {
		if len(input.Prompt.Data) == 0 || len(input.Prompt.Data) > 20<<20 || !supportedCloneAudio(input.Prompt.MIMEType, input.Prompt.Name) {
			return db.VoiceProfile{}, fmt.Errorf("%w: prompt audio must be mp3, m4a or wav and not exceed 20 MB", ErrInvalidInput)
		}
		if strings.TrimSpace(input.PromptText) == "" {
			return db.VoiceProfile{}, fmt.Errorf("%w: prompt_text is required with prompt audio", ErrInvalidInput)
		}
	}
	if strings.TrimSpace(input.PreviewText) == "" {
		input.PreviewText = "这是音色创建后的正式试听，用于确认声音效果。"
	}
	model, err := s.store.GetModel(ctx, input.ModelID)
	if err != nil {
		return db.VoiceProfile{}, mapStoreError(err)
	}
	if model.Capability != "audio" || (model.ProviderCode != ProviderMiniMax && model.ProviderCode != ProviderSiliconFlow) {
		return db.VoiceProfile{}, fmt.Errorf("%w: voice cloning requires a supported audio model", ErrInvalidInput)
	}
	provider, err := s.store.GetModelProvider(ctx, model.ProviderID)
	if err != nil {
		return db.VoiceProfile{}, mapStoreError(err)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return db.VoiceProfile{}, err
	}
	baseURL := strings.TrimRight(secret.BaseURL, "/")
	var (
		remoteVoiceID    string
		providerFileID   string
		providerPromptID string
		previewData      []byte
		previewMetadata  json.RawMessage
		providerMetadata json.RawMessage
		cleanupRemote    func()
	)
	defer func() {
		if cleanupRemote != nil {
			cleanupRemote()
		}
	}()
	var promptFileID int64
	switch model.ProviderCode {
	case ProviderMiniMax:
		sourceFileID, uploadErr := s.uploadProviderFile(ctx, baseURL, secret.APIKey, "voice_clone", input.Source)
		if uploadErr != nil {
			return db.VoiceProfile{}, uploadErr
		}
		providerFileID = strconv.FormatInt(sourceFileID, 10)
		defer s.deleteProviderFile(context.Background(), baseURL, secret.APIKey, "voice_clone", sourceFileID)
		if input.Prompt != nil {
			promptFileID, err = s.uploadProviderFile(ctx, baseURL, secret.APIKey, "prompt_audio", *input.Prompt)
			if err != nil {
				return db.VoiceProfile{}, err
			}
			providerPromptID = optionalIntString(promptFileID)
			defer s.deleteProviderFile(context.Background(), baseURL, secret.APIKey, "prompt_audio", promptFileID)
		}
		clonePayload := map[string]any{
			"file_id": sourceFileID, "voice_id": strings.TrimSpace(input.VoiceID), "model": model.ModelID,
			"need_noise_reduction":      input.NeedNoiseReduction,
			"need_volume_normalization": input.NeedVolumeNormalization,
			"aigc_watermark":            false,
		}
		if promptFileID != 0 {
			clonePayload["clone_prompt"] = map[string]any{"prompt_audio": promptFileID, "prompt_text": strings.TrimSpace(input.PromptText)}
		}
		var cloneResponse struct {
			InputSensitive     bool                `json:"input_sensitive"`
			InputSensitiveType int                 `json:"input_sensitive_type"`
			BaseResp           miniMaxBaseResponse `json:"base_resp"`
		}
		rawClone, cloneErr := s.doJSON(ctx, http.MethodPost, baseURL+"/v1/voice_clone", secret.APIKey, clonePayload, &cloneResponse)
		if cloneErr != nil {
			return db.VoiceProfile{}, cloneErr
		}
		if cloneResponse.BaseResp.StatusCode != 0 {
			return db.VoiceProfile{}, fmt.Errorf("%w: %s", ErrProviderRequest, cloneResponse.BaseResp.StatusMsg)
		}
		remoteVoiceID = strings.TrimSpace(input.VoiceID)
		providerMetadata = jsonSummary(rawClone)
		previewData, previewMetadata, err = s.activateVoice(ctx, baseURL, secret.APIKey, model.ModelID, remoteVoiceID, input.PreviewText)
		if err != nil {
			return db.VoiceProfile{}, fmt.Errorf("activate cloned voice: %w", err)
		}
	case ProviderSiliconFlow:
		if input.Prompt != nil {
			return db.VoiceProfile{}, fmt.Errorf("%w: SiliconFlow uses the clone source directly and does not accept a separate prompt audio", ErrInvalidInput)
		}
		if strings.TrimSpace(input.PromptText) == "" {
			return db.VoiceProfile{}, fmt.Errorf("%w: prompt_text must match the SiliconFlow reference audio", ErrInvalidInput)
		}
		remoteVoiceID, providerMetadata, err = s.uploadSiliconFlowVoice(ctx, baseURL, secret.APIKey, model.ModelID, strings.TrimSpace(input.VoiceID), strings.TrimSpace(input.PromptText), input.Source)
		if err != nil {
			return db.VoiceProfile{}, err
		}
		providerFileID = remoteVoiceID
		cleanupRemote = func() {
			if deleteErr := s.deleteSiliconFlowVoice(context.Background(), baseURL, secret.APIKey, remoteVoiceID); deleteErr != nil {
				s.log.Warn("delete orphaned SiliconFlow voice", zap.String("voice_id", remoteVoiceID), zap.Error(deleteErr))
			}
		}
		previewData, previewMetadata, err = s.previewSiliconFlowVoice(ctx, baseURL, secret.APIKey, model.ModelID, remoteVoiceID, input.PreviewText)
		if err != nil {
			return db.VoiceProfile{}, fmt.Errorf("activate cloned voice: %w", err)
		}
	}
	profileID := uuid.New()
	sourceObject, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{Purpose: "voices/source", OriginalName: input.Source.Name, UploadInput: storage.UploadInput{Data: input.Source.Data, ContentType: input.Source.MIMEType}})
	if err != nil {
		return db.VoiceProfile{}, err
	}
	var promptObject *storage.ManagedObject
	if input.Prompt != nil {
		created, uploadErr := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{Purpose: "voices/prompt", OriginalName: input.Prompt.Name, UploadInput: storage.UploadInput{Data: input.Prompt.Data, ContentType: input.Prompt.MIMEType}})
		err = uploadErr
		promptObject = &created
		if err != nil {
			_ = s.storage.DeleteManaged(ctx, sourceObject.Record.ID)
			return db.VoiceProfile{}, err
		}
	}
	previewObject, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{Purpose: "voices/preview", OriginalName: "preview.mp3", UploadInput: storage.UploadInput{Data: previewData, ContentType: "audio/mpeg"}})
	if err != nil {
		_ = s.storage.DeleteManaged(ctx, sourceObject.Record.ID)
		if promptObject != nil {
			_ = s.storage.DeleteManaged(ctx, promptObject.Record.ID)
		}
		return db.VoiceProfile{}, err
	}
	now := time.Now()
	profile := db.VoiceProfile{
		ID: profileID, ProviderID: model.ProviderID, ModelID: model.ID,
		Name: input.Name, Description: input.Description, VoiceID: remoteVoiceID, Status: "ready",
		SourceName: input.Source.Name, SourceMimeType: input.Source.MIMEType, SourceFileSizeBytes: int64(len(input.Source.Data)),
		SourceObjectID:  sourceObject.Record.ID,
		PromptText:      strings.TrimSpace(input.PromptText),
		PreviewMimeType: "audio/mpeg", PreviewFileSizeBytes: int64(len(previewData)),
		PreviewObjectID: previewObject.Record.ID,
		ProviderFileID:  providerFileID, ProviderPromptFileID: providerPromptID,
		ActivatedAt: &now, Metadata: db.JSON(map[string]any{
			"clone_response": providerMetadata,
			"preview":        previewMetadata,
			"preview_text":   input.PreviewText,
			"custom_name":    strings.TrimSpace(input.VoiceID),
		}),
	}
	if promptObject != nil {
		profile.PromptObjectID = &promptObject.Record.ID
	}
	created, err := s.store.CreateVoiceProfile(ctx, profile)
	if err != nil {
		_ = s.storage.DeleteManaged(ctx, sourceObject.Record.ID)
		if promptObject != nil {
			_ = s.storage.DeleteManaged(ctx, promptObject.Record.ID)
		}
		_ = s.storage.DeleteManaged(ctx, previewObject.Record.ID)
		return db.VoiceProfile{}, err
	}
	cleanupRemote = nil
	return created, nil
}

func (s *VoiceService) Delete(ctx context.Context, id uuid.UUID) error {
	profile, err := s.store.GetVoiceProfile(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	provider, err := s.store.GetModelProvider(ctx, profile.ProviderID)
	if err != nil {
		return mapStoreError(err)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(secret.BaseURL, "/")
	switch provider.AdapterCode {
	case ProviderMiniMax:
		var response struct {
			BaseResp miniMaxBaseResponse `json:"base_resp"`
		}
		if _, err := s.doJSON(ctx, http.MethodPost, baseURL+"/v1/delete_voice", secret.APIKey, map[string]any{
			"voice_type": "voice_cloning", "voice_id": profile.VoiceID,
		}, &response); err != nil {
			return err
		}
		if response.BaseResp.StatusCode != 0 {
			return fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
		}
	case ProviderSiliconFlow:
		if err := s.deleteSiliconFlowVoice(ctx, baseURL, secret.APIKey, profile.VoiceID); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: voice deletion is unavailable for adapter %s", ErrInvalidInput, provider.AdapterCode)
	}
	if err := s.store.ConfirmDeleteVoiceProfile(ctx, id); err != nil {
		return err
	}
	for _, objectID := range []uuid.UUID{profile.SourceObjectID, profile.PreviewObjectID} {
		if err := s.storage.DeleteManaged(context.Background(), objectID); err != nil {
			s.log.Warn("delete managed voice profile object", zap.String("voice_profile_id", profile.ID.String()), zap.Error(err))
		}
	}
	if profile.PromptObjectID != nil {
		if err := s.storage.DeleteManaged(context.Background(), *profile.PromptObjectID); err != nil {
			s.log.Warn("delete managed voice prompt object", zap.String("voice_profile_id", profile.ID.String()), zap.Error(err))
		}
	}
	return nil
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
		"trace_id": resp.Header.Get("x-siliconcloud-trace-id"),
		"bytes":    len(data),
	})
	return data, metadata, nil
}

func (s *VoiceService) deleteSiliconFlowVoice(ctx context.Context, baseURL, key, uri string) error {
	var response any
	if _, err := s.doJSON(ctx, http.MethodPost, baseURL+"/audio/voice/deletions", key, map[string]any{"uri": uri}, &response); err != nil {
		return err
	}
	return nil
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

func optionalIntString(value int64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
