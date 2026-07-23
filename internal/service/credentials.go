package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const (
	ProviderDeepSeek        = "deepseek"
	ProviderVolcengine      = "volcengine"
	ProviderMiniMax         = "minimax"
	ProviderAliyunBailian   = "aliyun_bailian"
	ProviderSiliconFlow     = "siliconflow"
	ProviderZhipu           = "zhipu"
	ProviderTencentTokenHub = "tencent_tokenhub"
	ProviderOllama          = "ollama"
	ProviderComfyUI         = "comfyui"
	ProviderOpenAIChat      = "openai_chat"
	ProviderOpenAIResponses = "openai_responses"
)

type CredentialService struct {
	store *db.Store
	box   *SecretBox
}
type CredentialSecret struct {
	ID                                    *uuid.UUID
	ProviderID                            uuid.UUID
	ProviderCode, BaseURL, APIKey, Source string
}
type SaveCredentialInput struct {
	ID           uuid.UUID
	ProviderID   uuid.UUID
	Name, APIKey string
	Activate     bool
}

func NewCredentialService(store *db.Store, masterKeyPath string) (*CredentialService, error) {
	secret, err := loadOrCreateMasterKey(masterKeyPath)
	if err != nil {
		return nil, err
	}
	box, err := NewSecretBox(secret)
	if err != nil {
		return nil, err
	}
	return &CredentialService{store: store, box: box}, nil
}
func (s *CredentialService) List(ctx context.Context, providerID *uuid.UUID) ([]db.ProviderCredential, error) {
	return s.store.ListProviderCredentials(ctx, providerID)
}
func (s *CredentialService) Create(ctx context.Context, input SaveCredentialInput) (db.ProviderCredential, error) {
	if input.ProviderID == uuid.Nil {
		return db.ProviderCredential{}, fmt.Errorf("%w: provider_id is required", ErrInvalidInput)
	}
	key := strings.TrimSpace(input.APIKey)
	if key == "" {
		return db.ProviderCredential{}, fmt.Errorf("%w: api_key is required", ErrInvalidInput)
	}
	box, err := s.requireBox()
	if err != nil {
		return db.ProviderCredential{}, err
	}
	encrypted, err := box.Encrypt(key)
	if err != nil {
		return db.ProviderCredential{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Primary credential"
	}
	return s.store.CreateProviderCredential(ctx, db.SaveCredentialInput{ProviderID: input.ProviderID, Name: name, EncryptedAPIKey: encrypted, KeyHint: keyHint(key), Activate: input.Activate})
}
func (s *CredentialService) Update(ctx context.Context, input SaveCredentialInput) (db.ProviderCredential, error) {
	existing, err := s.store.GetProviderCredential(ctx, input.ID)
	if err != nil {
		return db.ProviderCredential{}, mapStoreError(err)
	}
	encrypted := existing.EncryptedAPIKey
	hint := existing.KeyHint
	if key := strings.TrimSpace(input.APIKey); key != "" {
		box, err := s.requireBox()
		if err != nil {
			return db.ProviderCredential{}, err
		}
		encrypted, err = box.Encrypt(key)
		if err != nil {
			return db.ProviderCredential{}, err
		}
		hint = keyHint(key)
	}
	providerID := input.ProviderID
	if providerID == uuid.Nil {
		providerID = existing.ProviderID
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = existing.Name
	}
	return s.store.UpdateProviderCredential(ctx, db.SaveCredentialInput{ID: input.ID, ProviderID: providerID, Name: name, EncryptedAPIKey: encrypted, KeyHint: hint, Activate: input.Activate})
}
func (s *CredentialService) Activate(ctx context.Context, id uuid.UUID) (db.ProviderCredential, error) {
	value, err := s.store.ActivateProviderCredential(ctx, id)
	return value, mapStoreError(err)
}
func (s *CredentialService) Delete(ctx context.Context, id uuid.UUID) error {
	return mapStoreError(s.store.DeleteProviderCredential(ctx, id))
}
func (s *CredentialService) SecretForProvider(ctx context.Context, provider db.ModelProvider) (CredentialSecret, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
	if provider.AuthType == "none" {
		if baseURL == "" {
			return CredentialSecret{}, fmt.Errorf("%w: provider base_url is required", ErrInvalidInput)
		}
		return CredentialSecret{ProviderID: provider.ID, ProviderCode: provider.Code, BaseURL: baseURL, Source: "none"}, nil
	}
	credential, err := s.store.GetActiveProviderCredential(ctx, provider.ID)
	if err == nil {
		return s.decrypt(ctx, credential, provider.BaseURL, true)
	}
	if !errors.Is(err, db.ErrNotFound) {
		return CredentialSecret{}, err
	}
	return CredentialSecret{}, ErrProviderCredentialMissing
}
func (s *CredentialService) SecretByID(ctx context.Context, id uuid.UUID) (CredentialSecret, error) {
	credential, err := s.store.GetProviderCredential(ctx, id)
	if err != nil {
		return CredentialSecret{}, mapStoreError(err)
	}
	provider, err := s.store.GetModelProvider(ctx, credential.ProviderID)
	if err != nil {
		return CredentialSecret{}, mapStoreError(err)
	}
	return s.decrypt(ctx, credential, provider.BaseURL, false)
}
func (s *CredentialService) decrypt(ctx context.Context, credential db.ProviderCredential, baseURL string, touch bool) (CredentialSecret, error) {
	box, err := s.requireBox()
	if err != nil {
		return CredentialSecret{}, err
	}
	key, err := box.Decrypt(credential.EncryptedAPIKey)
	if err != nil {
		return CredentialSecret{}, err
	}
	if touch {
		if err := s.store.TouchProviderCredential(ctx, credential.ID); err != nil {
			return CredentialSecret{}, err
		}
	}
	id := credential.ID
	return CredentialSecret{ID: &id, ProviderID: credential.ProviderID, ProviderCode: credential.ProviderCode, BaseURL: baseURL, APIKey: key, Source: "database"}, nil
}
func (s *CredentialService) requireBox() (*SecretBox, error) {
	if s == nil || s.box == nil {
		return nil, ErrCredentialEncryption
	}
	return s.box, nil
}

func (s *CredentialService) EncryptSecret(value string) (string, error) {
	box, err := s.requireBox()
	if err != nil {
		return "", err
	}
	return box.Encrypt(value)
}

func (s *CredentialService) DecryptSecret(value string) (string, error) {
	box, err := s.requireBox()
	if err != nil {
		return "", err
	}
	return box.Decrypt(value)
}
func loadOrCreateMasterKey(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		if secret := strings.TrimSpace(string(data)); secret != "" {
			return secret, nil
		}
		return "", fmt.Errorf("credential master key is empty: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read credential master key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write credential master key: %w", err)
	}
	return secret, nil
}
func keyHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return "****"
	}
	return key[:3] + "..." + key[len(key)-4:]
}
func mapStoreError(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
