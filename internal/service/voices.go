package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
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
	ProjectID               uuid.UUID
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
	if input.ProjectID == uuid.Nil || input.ModelID == uuid.Nil {
		return db.VoiceProfile{}, fmt.Errorf("%w: project_id and model_id are required", ErrInvalidInput)
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
	if model.Capability != "audio" || model.ProviderCode != ProviderMiniMax {
		return db.VoiceProfile{}, fmt.Errorf("%w: voice cloning currently requires a MiniMax audio model", ErrInvalidInput)
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
	sourceFileID, err := s.uploadProviderFile(ctx, baseURL, secret.APIKey, "voice_clone", input.Source)
	if err != nil {
		return db.VoiceProfile{}, err
	}
	defer s.deleteProviderFile(context.Background(), baseURL, secret.APIKey, "voice_clone", sourceFileID)
	var promptFileID int64
	if input.Prompt != nil {
		promptFileID, err = s.uploadProviderFile(ctx, baseURL, secret.APIKey, "prompt_audio", *input.Prompt)
		if err != nil {
			return db.VoiceProfile{}, err
		}
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
	rawClone, err := s.doJSON(ctx, http.MethodPost, baseURL+"/v1/voice_clone", secret.APIKey, clonePayload, &cloneResponse)
	if err != nil {
		return db.VoiceProfile{}, err
	}
	if cloneResponse.BaseResp.StatusCode != 0 {
		return db.VoiceProfile{}, fmt.Errorf("%w: %s", ErrProviderRequest, cloneResponse.BaseResp.StatusMsg)
	}
	previewData, previewMetadata, err := s.activateVoice(ctx, baseURL, secret.APIKey, model.ModelID, strings.TrimSpace(input.VoiceID), input.PreviewText)
	if err != nil {
		return db.VoiceProfile{}, fmt.Errorf("activate cloned voice: %w", err)
	}
	profileID := uuid.New()
	sourceObject, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{ProjectID: &input.ProjectID, Purpose: "voices/source", OriginalName: input.Source.Name, UploadInput: storage.UploadInput{Data: input.Source.Data, ContentType: input.Source.MIMEType}})
	if err != nil {
		return db.VoiceProfile{}, err
	}
	var promptObject *storage.ManagedObject
	if input.Prompt != nil {
		created, uploadErr := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{ProjectID: &input.ProjectID, Purpose: "voices/prompt", OriginalName: input.Prompt.Name, UploadInput: storage.UploadInput{Data: input.Prompt.Data, ContentType: input.Prompt.MIMEType}})
		err = uploadErr
		promptObject = &created
		if err != nil {
			_ = s.storage.DeleteManaged(ctx, sourceObject.Record.ID)
			return db.VoiceProfile{}, err
		}
	}
	previewObject, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{ProjectID: &input.ProjectID, Purpose: "voices/preview", OriginalName: "preview.mp3", UploadInput: storage.UploadInput{Data: previewData, ContentType: "audio/mpeg"}})
	if err != nil {
		_ = s.storage.DeleteManaged(ctx, sourceObject.Record.ID)
		if promptObject != nil {
			_ = s.storage.DeleteManaged(ctx, promptObject.Record.ID)
		}
		return db.VoiceProfile{}, err
	}
	now := time.Now()
	profile := db.VoiceProfile{
		ID: profileID, ProjectID: input.ProjectID, ProviderID: model.ProviderID, ModelID: model.ID,
		Name: input.Name, Description: input.Description, VoiceID: input.VoiceID, Status: "ready",
		SourceName: input.Source.Name, SourceMimeType: input.Source.MIMEType, SourceFileSizeBytes: int64(len(input.Source.Data)),
		SourceObjectID:  sourceObject.Record.ID,
		PromptText:      strings.TrimSpace(input.PromptText),
		PreviewMimeType: "audio/mpeg", PreviewFileSizeBytes: int64(len(previewData)),
		PreviewObjectID: previewObject.Record.ID,
		ProviderFileID:  strconv.FormatInt(sourceFileID, 10), ProviderPromptFileID: optionalIntString(promptFileID),
		ActivatedAt: &now, Metadata: db.JSON(map[string]any{
			"clone_response": jsonSummary(rawClone),
			"preview":        previewMetadata,
			"preview_text":   input.PreviewText,
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
	var response struct {
		BaseResp miniMaxBaseResponse `json:"base_resp"`
	}
	if _, err := s.doJSON(ctx, http.MethodPost, strings.TrimRight(secret.BaseURL, "/")+"/v1/delete_voice", secret.APIKey, map[string]any{
		"voice_type": "voice_cloning", "voice_id": profile.VoiceID,
	}, &response); err != nil {
		return err
	}
	if response.BaseResp.StatusCode != 0 {
		return fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
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

func voiceObjectKey(projectID, profileID uuid.UUID, role, name, mimeType string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		if exts, _ := mime.ExtensionsByType(strings.Split(mimeType, ";")[0]); len(exts) > 0 {
			ext = exts[0]
		}
	}
	if ext == "" {
		ext = ".bin"
	}
	return filepath.ToSlash(filepath.Join("voice-profiles", projectID.String(), profileID.String(), role+ext))
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
