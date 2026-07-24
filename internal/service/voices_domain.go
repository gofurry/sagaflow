package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const defaultVoicePreviewText = "这是音色创建后的正式试听，用于确认声音效果。"

type VoiceProviderCapability struct {
	ProviderCode           string   `json:"provider_code"`
	ProviderName           string   `json:"provider_name"`
	Operations             []string `json:"operations"`
	AcceptedAudio          []string `json:"accepted_audio"`
	MaxSourceBytes         int64    `json:"max_source_bytes"`
	ReferenceTextRequired  bool     `json:"reference_text_required"`
	CloneRequiresPublicURL bool     `json:"clone_requires_public_url"`
	SupportsNoiseReduction bool     `json:"supports_noise_reduction"`
	SupportsNormalization  bool     `json:"supports_normalization"`
	VoiceIDLabel           string   `json:"voice_id_label"`
	VoiceIDHint            string   `json:"voice_id_hint"`
}

type CreateVoiceProfileInput struct {
	Name          string
	Description   string
	Kind          string
	Source        *VoiceFile
	ReferenceText string
	DesignPrompt  string
}

type CreateVoiceBindingInput struct {
	ProfileID               uuid.UUID
	ModelID                 uuid.UUID
	VoiceID                 string
	PreviewText             string
	SourceURL               string
	NeedNoiseReduction      bool
	NeedVolumeNormalization bool
}

type voiceAdapterInput struct {
	Profile                 db.VoiceProfile
	Model                   db.Model
	Secret                  CredentialSecret
	Source                  *VoiceFile
	VoiceID                 string
	PreviewText             string
	SourceURL               string
	NeedNoiseReduction      bool
	NeedVolumeNormalization bool
}

type voiceAdapterResult struct {
	VoiceID              string
	Preview              *VoiceFile
	ProviderFileID       string
	ProviderPromptFileID string
	Metadata             json.RawMessage
}

type voiceAdapter interface {
	Capability() VoiceProviderCapability
	Create(context.Context, voiceAdapterInput) (voiceAdapterResult, error)
	Delete(context.Context, CredentialSecret, db.VoiceBinding) error
}

func NewVoiceService(store *db.Store, credentials *CredentialService, objectStore *storage.Manager, log *zap.Logger) *VoiceService {
	if log == nil {
		log = zap.NewNop()
	}
	service := &VoiceService{
		store: store, credentials: credentials, storage: objectStore,
		httpClient: &http.Client{Timeout: 5 * time.Minute}, log: log,
	}
	service.adapters = service.buildVoiceAdapters()
	return service
}

func (s *VoiceService) Capabilities(ctx context.Context) ([]VoiceProviderCapability, error) {
	providers, err := s.store.ListModelProviders(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]VoiceProviderCapability, 0, len(s.adapters))
	for _, provider := range providers {
		adapter := s.adapters[provider.AdapterCode]
		if adapter == nil || !provider.Enabled {
			continue
		}
		capability := adapter.Capability()
		capability.ProviderCode = provider.Code
		capability.ProviderName = provider.DisplayName
		result = append(result, capability)
	}
	return result, nil
}

func (s *VoiceService) CreateProfile(ctx context.Context, input CreateVoiceProfileInput) (db.VoiceProfile, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.ReferenceText = strings.TrimSpace(input.ReferenceText)
	input.DesignPrompt = strings.TrimSpace(input.DesignPrompt)
	if input.Name == "" {
		return db.VoiceProfile{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if input.Kind != "clone" && input.Kind != "design" {
		return db.VoiceProfile{}, fmt.Errorf("%w: kind must be clone or design", ErrInvalidInput)
	}
	profile := db.VoiceProfile{
		ID: uuid.New(), Name: input.Name, Description: input.Description, Kind: input.Kind,
		ReferenceText: input.ReferenceText, DesignPrompt: input.DesignPrompt, Metadata: db.JSON(map[string]any{}),
	}
	if input.Kind == "clone" {
		if input.Source == nil || len(input.Source.Data) == 0 || len(input.Source.Data) > 20<<20 {
			return db.VoiceProfile{}, fmt.Errorf("%w: clone audio is required and must not exceed 20 MB", ErrInvalidInput)
		}
		if !supportedCloneAudio(input.Source.MIMEType, input.Source.Name) {
			return db.VoiceProfile{}, fmt.Errorf("%w: clone audio must be mp3, m4a or wav", ErrInvalidInput)
		}
		if input.ReferenceText == "" {
			return db.VoiceProfile{}, fmt.Errorf("%w: reference_text is required for a portable clone profile", ErrInvalidInput)
		}
		object, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{
			Purpose: "voices/source", OriginalName: input.Source.Name,
			UploadInput: storage.UploadInput{Data: input.Source.Data, ContentType: input.Source.MIMEType},
		})
		if err != nil {
			return db.VoiceProfile{}, err
		}
		profile.SourceName = input.Source.Name
		profile.SourceMimeType = input.Source.MIMEType
		profile.SourceFileSizeBytes = int64(len(input.Source.Data))
		profile.SourceObjectID = &object.Record.ID
		created, err := s.store.CreateVoiceProfile(ctx, profile)
		if err != nil {
			_ = s.storage.DeleteManaged(context.Background(), object.Record.ID)
			return db.VoiceProfile{}, err
		}
		return created, nil
	}
	if input.DesignPrompt == "" {
		return db.VoiceProfile{}, fmt.Errorf("%w: design_prompt is required", ErrInvalidInput)
	}
	return s.store.CreateVoiceProfile(ctx, profile)
}

func (s *VoiceService) CreateBinding(ctx context.Context, input CreateVoiceBindingInput) (db.VoiceBinding, error) {
	if input.ProfileID == uuid.Nil || input.ModelID == uuid.Nil {
		return db.VoiceBinding{}, fmt.Errorf("%w: profile_id and model_id are required", ErrInvalidInput)
	}
	profile, err := s.store.GetVoiceProfile(ctx, input.ProfileID)
	if err != nil {
		return db.VoiceBinding{}, mapStoreError(err)
	}
	model, err := s.store.GetModel(ctx, input.ModelID)
	if err != nil {
		return db.VoiceBinding{}, mapStoreError(err)
	}
	if model.Capability != "audio" || !containsString(model.Features, "voice_"+profile.Kind) {
		return db.VoiceBinding{}, fmt.Errorf("%w: selected model does not support voice_%s", ErrInvalidInput, profile.Kind)
	}
	provider, err := s.store.GetModelProvider(ctx, model.ProviderID)
	if err != nil {
		return db.VoiceBinding{}, mapStoreError(err)
	}
	adapter := s.adapters[provider.AdapterCode]
	if adapter == nil || !containsString(adapter.Capability().Operations, profile.Kind) {
		return db.VoiceBinding{}, fmt.Errorf("%w: provider does not support %s voices", ErrInvalidInput, profile.Kind)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return db.VoiceBinding{}, err
	}
	var source *VoiceFile
	if profile.Kind == "clone" {
		if profile.SourceObjectID == nil {
			return db.VoiceBinding{}, fmt.Errorf("%w: clone profile has no local source", ErrInvalidInput)
		}
		file, _, openErr := s.storage.Open(ctx, *profile.SourceObjectID)
		if openErr != nil {
			return db.VoiceBinding{}, openErr
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 20<<20+1))
		closeErr := file.Close()
		if readErr != nil {
			return db.VoiceBinding{}, readErr
		}
		if closeErr != nil {
			return db.VoiceBinding{}, closeErr
		}
		source = &VoiceFile{Name: profile.SourceName, MIMEType: profile.SourceMimeType, Data: data}
	}
	previewText := strings.TrimSpace(input.PreviewText)
	if previewText == "" {
		previewText = defaultVoicePreviewText
	}
	result, err := adapter.Create(ctx, voiceAdapterInput{
		Profile: profile, Model: model, Secret: secret, Source: source,
		VoiceID: strings.TrimSpace(input.VoiceID), PreviewText: previewText,
		SourceURL: strings.TrimSpace(input.SourceURL), NeedNoiseReduction: input.NeedNoiseReduction,
		NeedVolumeNormalization: input.NeedVolumeNormalization,
	})
	if err != nil {
		return db.VoiceBinding{}, err
	}
	binding := db.VoiceBinding{
		ID: uuid.New(), VoiceProfileID: profile.ID, ProviderID: provider.ID, ModelID: model.ID,
		Operation: profile.Kind, VoiceID: result.VoiceID, Status: "ready", PreviewText: previewText,
		ProviderFileID: result.ProviderFileID, ProviderPromptFileID: result.ProviderPromptFileID,
		Metadata: result.Metadata,
	}
	now := time.Now()
	binding.ActivatedAt = &now
	var previewObjectID *uuid.UUID
	if result.Preview != nil && len(result.Preview.Data) != 0 {
		object, uploadErr := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{
			Purpose: "voices/preview", OriginalName: result.Preview.Name,
			UploadInput: storage.UploadInput{Data: result.Preview.Data, ContentType: result.Preview.MIMEType},
		})
		if uploadErr != nil {
			_ = adapter.Delete(context.Background(), secret, binding)
			return db.VoiceBinding{}, uploadErr
		}
		binding.PreviewObjectID = &object.Record.ID
		binding.PreviewMimeType = result.Preview.MIMEType
		binding.PreviewFileSizeBytes = int64(len(result.Preview.Data))
		previewObjectID = &object.Record.ID
	}
	created, err := s.store.CreateVoiceBinding(ctx, binding)
	if err != nil {
		_ = adapter.Delete(context.Background(), secret, binding)
		if previewObjectID != nil {
			_ = s.storage.DeleteManaged(context.Background(), *previewObjectID)
		}
		return db.VoiceBinding{}, err
	}
	return created, nil
}

func (s *VoiceService) DeleteBinding(ctx context.Context, id uuid.UUID) error {
	binding, err := s.store.GetVoiceBinding(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	provider, err := s.store.GetModelProvider(ctx, binding.ProviderID)
	if err != nil {
		return mapStoreError(err)
	}
	adapter := s.adapters[provider.AdapterCode]
	if adapter == nil {
		return fmt.Errorf("%w: provider voice adapter is unavailable", ErrInvalidInput)
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return err
	}
	if err := adapter.Delete(ctx, secret, binding); err != nil {
		return err
	}
	if err := s.store.ConfirmDeleteVoiceBinding(ctx, id); err != nil {
		return mapStoreError(err)
	}
	if binding.PreviewObjectID != nil {
		if err := s.storage.DeleteManaged(context.Background(), *binding.PreviewObjectID); err != nil {
			s.log.Warn("delete managed voice preview", zap.String("voice_binding_id", id.String()), zap.Error(err))
		}
	}
	return nil
}

func (s *VoiceService) DeleteProfile(ctx context.Context, id uuid.UUID) error {
	profile, err := s.store.GetVoiceProfile(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	for _, binding := range profile.Bindings {
		if err := s.DeleteBinding(ctx, binding.ID); err != nil {
			return fmt.Errorf("delete %s binding: %w", binding.ProviderName, err)
		}
	}
	if err := s.store.ConfirmDeleteVoiceProfile(ctx, id); err != nil {
		return mapStoreError(err)
	}
	if profile.SourceObjectID != nil {
		if err := s.storage.DeleteManaged(context.Background(), *profile.SourceObjectID); err != nil {
			s.log.Warn("delete managed voice source", zap.String("voice_profile_id", id.String()), zap.Error(err))
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
