package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// StorageService manages optional S3-compatible publication connections.
// Canonical application data always remains in local storage.Manager.
type StorageService struct {
	store       *db.Store
	credentials *CredentialService
	local       *storage.Manager
	log         *zap.Logger
}

func NewStorageService(store *db.Store, credentials *CredentialService, local *storage.Manager, log *zap.Logger) *StorageService {
	if log == nil {
		log = zap.NewNop()
	}
	return &StorageService{store: store, credentials: credentials, local: local, log: log}
}

type SaveStorageInput struct {
	ID                                                               uuid.UUID
	Name, Provider, Endpoint, PublicEndpoint, Region, Bucket, Prefix string
	ForcePathStyle                                                   bool
	AccessKeyID, SecretAccessKey                                     string
	Enabled, IsDefault                                               bool
}

func (s *StorageService) List(ctx context.Context) ([]db.S3Connection, error) {
	return s.store.ListS3Connections(ctx)
}

func (s *StorageService) Save(ctx context.Context, input SaveStorageInput) (db.S3Connection, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if input.Name == "" || strings.TrimSpace(input.Bucket) == "" {
		return db.S3Connection{}, fmt.Errorf("%w: name and bucket are required", ErrInvalidInput)
	}
	if input.Provider == "" {
		input.Provider = "s3"
	}
	if err := validateStorageEndpoint("S3 endpoint", input.Endpoint); err != nil {
		return db.S3Connection{}, err
	}
	if err := validateStorageEndpoint("public endpoint", input.PublicEndpoint); err != nil {
		return db.S3Connection{}, err
	}
	encrypted := ""
	if input.ID != uuid.Nil {
		existing, err := s.store.GetS3Connection(ctx, input.ID)
		if err != nil {
			return db.S3Connection{}, mapStoreError(err)
		}
		encrypted = existing.EncryptedCredentials
	}
	keyID, keySecret := strings.TrimSpace(input.AccessKeyID), strings.TrimSpace(input.SecretAccessKey)
	if (keyID == "") != (keySecret == "") {
		return db.S3Connection{}, fmt.Errorf("%w: access key id and secret access key must be provided together", ErrInvalidInput)
	}
	if keyID != "" {
		raw, _ := json.Marshal(map[string]string{"access_key_id": keyID, "secret_access_key": keySecret})
		var err error
		encrypted, err = s.credentials.EncryptSecret(string(raw))
		if err != nil {
			return db.S3Connection{}, err
		}
	}
	if encrypted == "" {
		return db.S3Connection{}, fmt.Errorf("%w: S3 credentials are required", ErrInvalidInput)
	}
	return s.store.SaveS3Connection(ctx, db.SaveS3ConnectionInput{
		ID: input.ID, Name: input.Name, Provider: input.Provider,
		Endpoint: strings.TrimRight(strings.TrimSpace(input.Endpoint), "/"), PublicEndpoint: strings.TrimRight(strings.TrimSpace(input.PublicEndpoint), "/"),
		Region: strings.TrimSpace(input.Region), Bucket: strings.TrimSpace(input.Bucket), Prefix: strings.Trim(input.Prefix, "/"),
		ForcePathStyle: input.ForcePathStyle, EncryptedCredentials: encrypted, Enabled: input.Enabled, IsDefault: input.IsDefault,
	})
}

func (s *StorageService) Delete(ctx context.Context, id uuid.UUID) error {
	return mapStoreError(s.store.DeleteS3Connection(ctx, id))
}

func (s *StorageService) Test(ctx context.Context, id uuid.UUID) error {
	connection, err := s.store.GetS3Connection(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	remote, err := s.remoteStore(connection)
	if err != nil {
		return err
	}
	payload := []byte("sagaflow s3 connection probe")
	object, err := remote.Upload(ctx, storage.UploadInput{Key: "_healthchecks/" + uuid.NewString() + ".txt", Data: payload, ContentType: "text/plain"})
	if err != nil {
		return fmt.Errorf("%w: write probe: %v", ErrStorageUnavailable, err)
	}
	defer func() { _ = remote.Delete(context.Background(), object) }()
	read, err := remote.Read(ctx, object)
	if err != nil || string(read) != string(payload) {
		return fmt.Errorf("%w: read probe failed", ErrStorageUnavailable)
	}
	if err := remote.Delete(ctx, object); err != nil {
		return fmt.Errorf("%w: delete probe: %v", ErrStorageUnavailable, err)
	}
	return nil
}

func (s *StorageService) PublishAsset(ctx context.Context, assetID, connectionID uuid.UUID) (db.AssetRemoteExport, error) {
	asset, err := s.store.GetAsset(ctx, assetID)
	if err != nil {
		return db.AssetRemoteExport{}, mapStoreError(err)
	}
	connection, err := s.store.GetS3Connection(ctx, connectionID)
	if err != nil {
		return db.AssetRemoteExport{}, mapStoreError(err)
	}
	if !connection.Enabled {
		return db.AssetRemoteExport{}, fmt.Errorf("%w: S3 connection is disabled", ErrInvalidInput)
	}
	remote, err := s.remoteStore(connection)
	if err != nil {
		return db.AssetRemoteExport{}, err
	}
	exported, _, err := s.publishAsset(ctx, asset, connection, remote)
	return exported, err
}

type AssetPublishFailure struct {
	AssetID string `json:"asset_id"`
	Name    string `json:"name"`
	Error   string `json:"error"`
}

type AssetGroupPublishResult struct {
	Total     int                   `json:"total"`
	Published int                   `json:"published"`
	Skipped   int                   `json:"skipped"`
	Failed    []AssetPublishFailure `json:"failed"`
}

func (s *StorageService) PublishAssetGroup(ctx context.Context, groupID, connectionID uuid.UUID) (AssetGroupPublishResult, error) {
	if _, err := s.store.GetAssetGroup(ctx, groupID); err != nil {
		return AssetGroupPublishResult{}, mapStoreError(err)
	}
	assets, err := s.store.ListAssetGroupSubtreeAssets(ctx, groupID)
	if err != nil {
		return AssetGroupPublishResult{}, err
	}
	connection, err := s.store.GetS3Connection(ctx, connectionID)
	if err != nil {
		return AssetGroupPublishResult{}, mapStoreError(err)
	}
	if !connection.Enabled {
		return AssetGroupPublishResult{}, fmt.Errorf("%w: S3 connection is disabled", ErrInvalidInput)
	}
	remote, err := s.remoteStore(connection)
	if err != nil {
		return AssetGroupPublishResult{}, err
	}
	result := AssetGroupPublishResult{Total: len(assets), Failed: make([]AssetPublishFailure, 0)}
	for _, asset := range assets {
		if asset.Status == "discarded" {
			result.Skipped++
			continue
		}
		_, created, publishErr := s.publishAsset(ctx, asset, connection, remote)
		if publishErr != nil {
			result.Failed = append(result.Failed, AssetPublishFailure{AssetID: asset.ID.String(), Name: asset.Name, Error: publishErr.Error()})
			continue
		}
		if created {
			result.Published++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func (s *StorageService) publishAsset(ctx context.Context, asset db.Asset, connection db.S3Connection, remote *storage.S3Store) (db.AssetRemoteExport, bool, error) {
	existing, err := s.store.ListAssetRemoteExports(ctx, asset.ID)
	if err != nil {
		return db.AssetRemoteExport{}, false, err
	}
	for _, exported := range existing {
		if exported.ConnectionID == connection.ID && exported.State == "ready" {
			return exported, false, nil
		}
	}
	file, object, err := s.local.Open(ctx, asset.ObjectID)
	if err != nil {
		return db.AssetRemoteExport{}, false, err
	}
	defer file.Close()
	key := filepath.ToSlash(filepath.Join("projects", asset.ProjectID.String(), "assets", asset.ID.String(), object.OriginalName))
	uploaded, err := remote.Upload(ctx, storage.UploadInput{Key: key, Reader: file, Size: object.SizeBytes, ContentType: object.MimeType})
	if err != nil {
		return db.AssetRemoteExport{}, false, err
	}
	exported, err := s.store.CreateAssetRemoteExport(ctx, db.AssetRemoteExport{
		AssetID: asset.ID, ConnectionID: connection.ID, ObjectKey: uploaded.Key, PublicURL: uploaded.URL, State: "ready",
	})
	return exported, err == nil, err
}

func (s *StorageService) ProviderURL(ctx context.Context, exportID uuid.UUID, ttl time.Duration) (storage.SignedURL, error) {
	exported, err := s.store.GetAssetRemoteExport(ctx, exportID)
	if err != nil {
		return storage.SignedURL{}, mapStoreError(err)
	}
	connection, err := s.store.GetS3Connection(ctx, exported.ConnectionID)
	if err != nil {
		return storage.SignedURL{}, mapStoreError(err)
	}
	if exported.State != "ready" || !connection.Enabled {
		return storage.SignedURL{}, fmt.Errorf("%w: the remote copy or its S3 connection is not available", ErrStorageUnavailable)
	}
	remote, err := s.remoteStore(connection)
	if err != nil {
		return storage.SignedURL{}, err
	}
	return remote.PresignGet(ctx, storage.Object{Bucket: connection.Bucket, Key: exported.ObjectKey}, ttl)
}

func (s *StorageService) DeleteExport(ctx context.Context, id uuid.UUID) error {
	exported, err := s.store.GetAssetRemoteExport(ctx, id)
	if err != nil {
		return mapStoreError(err)
	}
	connection, err := s.store.GetS3Connection(ctx, exported.ConnectionID)
	if err != nil {
		return mapStoreError(err)
	}
	remote, err := s.remoteStore(connection)
	if err != nil {
		return err
	}
	if err := remote.Delete(ctx, storage.Object{Bucket: connection.Bucket, Key: exported.ObjectKey}); err != nil {
		return err
	}
	return s.store.DeleteAssetRemoteExport(ctx, id)
}

type s3Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

func (s *StorageService) remoteStore(connection db.S3Connection) (*storage.S3Store, error) {
	raw, err := s.credentials.DecryptSecret(connection.EncryptedCredentials)
	if err != nil {
		return nil, err
	}
	var secret s3Credentials
	if err := json.Unmarshal([]byte(raw), &secret); err != nil {
		return nil, fmt.Errorf("decode S3 credentials: %w", err)
	}
	return storage.NewS3Store(storage.S3Config{
		Bucket: connection.Bucket, Region: connection.Region, Endpoint: connection.Endpoint, PublicBaseURL: connection.PublicEndpoint,
		Prefix: connection.Prefix, ForcePathStyle: connection.ForcePathStyle, AccessKeyID: secret.AccessKeyID, SecretAccessKey: secret.SecretAccessKey,
	}, s.log)
}

func validateStorageEndpoint(label, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: %s must be an absolute http(s) URL", ErrInvalidInput, label)
	}
	return nil
}
