package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofurry/sagaflow/internal/store/db"
)

var aliyunVoicePrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,9}$`)

func (s *VoiceService) buildVoiceAdapters() map[string]voiceAdapter {
	return map[string]voiceAdapter{
		ProviderMiniMax:       miniMaxVoiceAdapter{service: s},
		ProviderSiliconFlow:   siliconFlowVoiceAdapter{service: s},
		ProviderZhipu:         zhipuVoiceAdapter{service: s},
		ProviderAliyunBailian: aliyunVoiceAdapter{service: s},
	}
}

type miniMaxVoiceAdapter struct{ service *VoiceService }

func (a miniMaxVoiceAdapter) Capability() VoiceProviderCapability {
	return VoiceProviderCapability{
		Operations: []string{"clone", "design"}, AcceptedAudio: []string{"mp3", "m4a", "wav"},
		MaxSourceBytes: 20 << 20, SupportsNoiseReduction: true, SupportsNormalization: true,
		VoiceIDLabel: "Voice ID", VoiceIDHint: "8–256 位，以字母开头，仅字母、数字、-、_",
	}
}

func (a miniMaxVoiceAdapter) Create(ctx context.Context, input voiceAdapterInput) (voiceAdapterResult, error) {
	baseURL := strings.TrimRight(input.Secret.BaseURL, "/")
	if input.Profile.Kind == "design" {
		payload := map[string]any{
			"prompt": input.Profile.DesignPrompt, "preview_text": input.PreviewText,
		}
		if input.VoiceID != "" {
			if !voiceIDPattern.MatchString(input.VoiceID) {
				return voiceAdapterResult{}, fmt.Errorf("%w: MiniMax voice_id must be 8-256 valid characters", ErrInvalidInput)
			}
			payload["voice_id"] = input.VoiceID
		}
		var response struct {
			VoiceID    string              `json:"voice_id"`
			TrialAudio string              `json:"trial_audio"`
			BaseResp   miniMaxBaseResponse `json:"base_resp"`
		}
		raw, err := a.service.doJSON(ctx, http.MethodPost, baseURL+"/v1/voice_design", input.Secret.APIKey, payload, &response)
		if err != nil {
			return voiceAdapterResult{}, err
		}
		if response.BaseResp.StatusCode != 0 || strings.TrimSpace(response.VoiceID) == "" {
			return voiceAdapterResult{}, fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
		}
		audio, err := hex.DecodeString(response.TrialAudio)
		if err != nil || len(audio) == 0 {
			return voiceAdapterResult{}, fmt.Errorf("%w: decode MiniMax trial audio", ErrProviderRequest)
		}
		voiceID := strings.TrimSpace(response.VoiceID)
		preview, previewMetadata, err := a.service.activateVoice(
			ctx, baseURL, input.Secret.APIKey, input.Model.ModelID, voiceID, input.PreviewText,
		)
		if err != nil {
			_ = a.deleteVoice(context.Background(), input.Secret, voiceID, "voice_generation")
			return voiceAdapterResult{}, fmt.Errorf("activate designed voice: %w", err)
		}
		return voiceAdapterResult{
			VoiceID: voiceID,
			Preview: &VoiceFile{Name: "preview.mp3", MIMEType: "audio/mpeg", Data: preview},
			Metadata: db.JSON(map[string]any{
				"create": jsonSummary(raw), "preview": previewMetadata, "trial_bytes": len(audio),
			}),
		}, nil
	}
	if input.Source == nil {
		return voiceAdapterResult{}, fmt.Errorf("%w: clone source is required", ErrInvalidInput)
	}
	if !voiceIDPattern.MatchString(input.VoiceID) {
		return voiceAdapterResult{}, fmt.Errorf("%w: MiniMax voice_id must be 8-256 valid characters", ErrInvalidInput)
	}
	fileID, err := a.service.uploadProviderFile(ctx, baseURL, input.Secret.APIKey, "voice_clone", *input.Source)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	defer a.service.deleteProviderFile(context.Background(), baseURL, input.Secret.APIKey, "voice_clone", fileID)
	payload := map[string]any{
		"file_id": fileID, "voice_id": input.VoiceID, "model": input.Model.ModelID,
		"need_noise_reduction":      input.NeedNoiseReduction,
		"need_volume_normalization": input.NeedVolumeNormalization,
		"aigc_watermark":            false,
	}
	var response struct {
		BaseResp miniMaxBaseResponse `json:"base_resp"`
	}
	raw, err := a.service.doJSON(ctx, http.MethodPost, baseURL+"/v1/voice_clone", input.Secret.APIKey, payload, &response)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	if response.BaseResp.StatusCode != 0 {
		return voiceAdapterResult{}, fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
	}
	preview, previewMetadata, err := a.service.activateVoice(ctx, baseURL, input.Secret.APIKey, input.Model.ModelID, input.VoiceID, input.PreviewText)
	if err != nil {
		_ = a.deleteVoice(context.Background(), input.Secret, input.VoiceID, "voice_cloning")
		return voiceAdapterResult{}, fmt.Errorf("activate cloned voice: %w", err)
	}
	return voiceAdapterResult{
		VoiceID: input.VoiceID, ProviderFileID: strconv.FormatInt(fileID, 10),
		Preview:  &VoiceFile{Name: "preview.mp3", MIMEType: "audio/mpeg", Data: preview},
		Metadata: db.JSON(map[string]any{"create": jsonSummary(raw), "preview": previewMetadata}),
	}, nil
}

func (a miniMaxVoiceAdapter) Delete(ctx context.Context, secret CredentialSecret, binding db.VoiceBinding) error {
	voiceType := "voice_cloning"
	if binding.Operation == "design" {
		voiceType = "voice_generation"
	}
	return a.deleteVoice(ctx, secret, binding.VoiceID, voiceType)
}

func (a miniMaxVoiceAdapter) deleteVoice(ctx context.Context, secret CredentialSecret, voiceID, voiceType string) error {
	var response struct {
		BaseResp miniMaxBaseResponse `json:"base_resp"`
	}
	_, err := a.service.doJSON(ctx, http.MethodPost, strings.TrimRight(secret.BaseURL, "/")+"/v1/delete_voice", secret.APIKey, map[string]any{
		"voice_id": voiceID, "voice_type": voiceType,
	}, &response)
	if err == nil && response.BaseResp.StatusCode != 0 {
		err = fmt.Errorf("%w: %s", ErrProviderRequest, response.BaseResp.StatusMsg)
	}
	return err
}

type siliconFlowVoiceAdapter struct{ service *VoiceService }

func (a siliconFlowVoiceAdapter) Capability() VoiceProviderCapability {
	return VoiceProviderCapability{
		Operations: []string{"clone"}, AcceptedAudio: []string{"mp3", "m4a", "wav"},
		MaxSourceBytes: 20 << 20, ReferenceTextRequired: true,
		VoiceIDLabel: "自定义名称", VoiceIDHint: "以字母开头，仅字母、数字、-、_",
	}
}

func (a siliconFlowVoiceAdapter) Create(ctx context.Context, input voiceAdapterInput) (voiceAdapterResult, error) {
	if input.Source == nil || input.Profile.ReferenceText == "" || input.VoiceID == "" {
		return voiceAdapterResult{}, fmt.Errorf("%w: SiliconFlow requires source audio, reference text and a custom name", ErrInvalidInput)
	}
	baseURL := strings.TrimRight(input.Secret.BaseURL, "/")
	voiceID, metadata, err := a.service.uploadSiliconFlowVoice(ctx, baseURL, input.Secret.APIKey, input.Model.ModelID, input.VoiceID, input.Profile.ReferenceText, *input.Source)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	preview, previewMetadata, err := a.service.previewSiliconFlowVoice(ctx, baseURL, input.Secret.APIKey, input.Model.ModelID, voiceID, input.PreviewText)
	if err != nil {
		_ = a.service.deleteSiliconFlowVoice(context.Background(), baseURL, input.Secret.APIKey, voiceID)
		return voiceAdapterResult{}, err
	}
	return voiceAdapterResult{
		VoiceID: voiceID, ProviderFileID: voiceID,
		Preview:  &VoiceFile{Name: "preview.mp3", MIMEType: "audio/mpeg", Data: preview},
		Metadata: db.JSON(map[string]any{"create": metadata, "preview": previewMetadata}),
	}, nil
}

func (a siliconFlowVoiceAdapter) Delete(ctx context.Context, secret CredentialSecret, binding db.VoiceBinding) error {
	return a.service.deleteSiliconFlowVoice(ctx, strings.TrimRight(secret.BaseURL, "/"), secret.APIKey, binding.VoiceID)
}

type zhipuVoiceAdapter struct{ service *VoiceService }

func (a zhipuVoiceAdapter) Capability() VoiceProviderCapability {
	return VoiceProviderCapability{
		Operations: []string{"clone"}, AcceptedAudio: []string{"mp3", "wav"},
		MaxSourceBytes: 10 << 20, VoiceIDLabel: "音色名称", VoiceIDHint: "用于平台识别音色的名称",
	}
}

func (a zhipuVoiceAdapter) Create(ctx context.Context, input voiceAdapterInput) (voiceAdapterResult, error) {
	if input.Source == nil || input.VoiceID == "" {
		return voiceAdapterResult{}, fmt.Errorf("%w: Zhipu requires source audio and a voice name", ErrInvalidInput)
	}
	if len(input.Source.Data) > 10<<20 || !supportedZhipuCloneAudio(input.Source.MIMEType, input.Source.Name) {
		return voiceAdapterResult{}, fmt.Errorf("%w: Zhipu clone audio must be mp3 or wav and not exceed 10 MB", ErrInvalidInput)
	}
	baseURL := strings.TrimRight(input.Secret.BaseURL, "/")
	fileID, err := a.service.uploadZhipuVoiceFile(ctx, baseURL, input.Secret.APIKey, *input.Source)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	defer a.service.deleteZhipuFile(context.Background(), baseURL, input.Secret.APIKey, fileID)
	voiceID, promptFileID, metadata, err := a.service.cloneZhipuVoice(ctx, baseURL, input.Secret.APIKey, input.VoiceID, input.Profile.ReferenceText, input.PreviewText, fileID)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	preview, previewMetadata, err := a.service.previewZhipuVoice(ctx, baseURL, input.Secret.APIKey, voiceID, input.PreviewText)
	if err != nil {
		_ = a.service.deleteZhipuVoice(context.Background(), baseURL, input.Secret.APIKey, voiceID)
		return voiceAdapterResult{}, err
	}
	return voiceAdapterResult{
		VoiceID: voiceID, ProviderFileID: fileID, ProviderPromptFileID: promptFileID,
		Preview:  &VoiceFile{Name: "preview.wav", MIMEType: "audio/wav", Data: preview},
		Metadata: db.JSON(map[string]any{"create": metadata, "preview": previewMetadata}),
	}, nil
}

func (a zhipuVoiceAdapter) Delete(ctx context.Context, secret CredentialSecret, binding db.VoiceBinding) error {
	return a.service.deleteZhipuVoice(ctx, strings.TrimRight(secret.BaseURL, "/"), secret.APIKey, binding.VoiceID)
}

type aliyunVoiceAdapter struct{ service *VoiceService }

func (a aliyunVoiceAdapter) Capability() VoiceProviderCapability {
	return VoiceProviderCapability{
		Operations: []string{"clone", "design"}, AcceptedAudio: []string{"mp3", "m4a", "wav"},
		MaxSourceBytes: 20 << 20, CloneRequiresPublicURL: true,
		VoiceIDLabel: "音色前缀", VoiceIDHint: "1–10 位，以小写字母开头，仅小写字母和数字；平台会生成完整 Voice ID",
	}
}

func (a aliyunVoiceAdapter) Create(ctx context.Context, input voiceAdapterInput) (voiceAdapterResult, error) {
	if !aliyunVoicePrefixPattern.MatchString(input.VoiceID) {
		return voiceAdapterResult{}, fmt.Errorf("%w: Aliyun voice prefix must be 1-10 lowercase alphanumeric characters and start with a letter", ErrInvalidInput)
	}
	baseURL := strings.TrimRight(input.Secret.BaseURL, "/")
	payloadInput := map[string]any{
		"action": "create_voice", "target_model": input.Model.ModelID, "prefix": input.VoiceID,
		"language_hints": []string{"zh"},
	}
	if input.Profile.Kind == "clone" {
		if strings.TrimSpace(input.SourceURL) == "" {
			return voiceAdapterResult{}, fmt.Errorf("%w: Aliyun clone requires a publicly reachable source_url", ErrInvalidInput)
		}
		payloadInput["url"] = strings.TrimSpace(input.SourceURL)
	} else {
		payloadInput["voice_prompt"] = input.Profile.DesignPrompt
		payloadInput["preview_text"] = input.PreviewText
	}
	payload := map[string]any{"model": "voice-enrollment", "input": payloadInput}
	if input.Profile.Kind == "design" {
		payload["parameters"] = map[string]any{"sample_rate": 24000, "response_format": "wav"}
	}
	var response struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Output    struct {
			VoiceID      string `json:"voice_id"`
			PreviewAudio struct {
				Data string `json:"data"`
			} `json:"preview_audio"`
		} `json:"output"`
	}
	raw, err := a.service.doJSON(ctx, http.MethodPost, bailianEndpoint(baseURL, "/api/v1/services/audio/tts/customization"), input.Secret.APIKey, payload, &response)
	if err != nil {
		return voiceAdapterResult{}, err
	}
	if response.Code != "" || strings.TrimSpace(response.Output.VoiceID) == "" {
		return voiceAdapterResult{}, fmt.Errorf("%w: %s", ErrProviderRequest, strings.TrimSpace(response.Message))
	}
	result := voiceAdapterResult{VoiceID: strings.TrimSpace(response.Output.VoiceID), Metadata: jsonSummary(raw, "request_id")}
	if response.Output.PreviewAudio.Data != "" {
		audio, decodeErr := base64.StdEncoding.DecodeString(response.Output.PreviewAudio.Data)
		if decodeErr != nil {
			_ = a.deleteVoice(context.Background(), input.Secret, result.VoiceID)
			return voiceAdapterResult{}, fmt.Errorf("%w: decode Aliyun preview audio", ErrProviderRequest)
		}
		result.Preview = &VoiceFile{Name: "preview.wav", MIMEType: "audio/wav", Data: audio}
		return result, nil
	}
	preview, mimeType, previewMetadata, err := a.preview(ctx, input.Secret, input.Model.ModelID, result.VoiceID, input.PreviewText)
	if err != nil {
		_ = a.deleteVoice(context.Background(), input.Secret, result.VoiceID)
		return voiceAdapterResult{}, err
	}
	result.Preview = &VoiceFile{Name: "preview.mp3", MIMEType: mimeType, Data: preview}
	result.Metadata = db.JSON(map[string]any{"create": jsonSummary(raw, "request_id"), "preview": previewMetadata})
	return result, nil
}

func (a aliyunVoiceAdapter) Delete(ctx context.Context, secret CredentialSecret, binding db.VoiceBinding) error {
	return a.deleteVoice(ctx, secret, binding.VoiceID)
}

func (a aliyunVoiceAdapter) deleteVoice(ctx context.Context, secret CredentialSecret, voiceID string) error {
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_, err := a.service.doJSON(ctx, http.MethodPost, bailianEndpoint(secret.BaseURL, "/api/v1/services/audio/tts/customization"), secret.APIKey, map[string]any{
		"model": "voice-enrollment", "input": map[string]any{"action": "delete_voice", "voice_id": voiceID},
	}, &response)
	if err == nil && response.Code != "" {
		err = fmt.Errorf("%w: %s", ErrProviderRequest, response.Message)
	}
	return err
}

func (a aliyunVoiceAdapter) preview(ctx context.Context, secret CredentialSecret, modelID, voiceID, text string) ([]byte, string, json.RawMessage, error) {
	var response struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Output    struct {
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
	}
	raw, err := a.service.doJSON(ctx, http.MethodPost, bailianEndpoint(secret.BaseURL, "/api/v1/services/audio/tts/SpeechSynthesizer"), secret.APIKey, map[string]any{
		"model": modelID, "input": map[string]any{
			"text": text, "voice": voiceID, "format": "mp3", "sample_rate": 24000,
			"volume": 50, "rate": 1, "pitch": 1,
		},
	}, &response)
	if err != nil {
		return nil, "", nil, err
	}
	if response.Code != "" || strings.TrimSpace(response.Output.Audio.URL) == "" {
		return nil, "", nil, fmt.Errorf("%w: %s", ErrProviderRequest, response.Message)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, response.Output.Audio.URL, nil)
	if err != nil {
		return nil, "", nil, err
	}
	resp, err := a.service.httpClient.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("%w: %v", ErrProviderRequest, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", nil, fmt.Errorf("%w: audio status=%d", ErrProviderRequest, resp.StatusCode)
	}
	return data, "audio/mpeg", jsonSummary(raw, "request_id"), nil
}

func bailianEndpoint(baseURL, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/api/v1") && strings.HasPrefix(path, "/api/v1/") {
		return baseURL + strings.TrimPrefix(path, "/api/v1")
	}
	return baseURL + path
}
