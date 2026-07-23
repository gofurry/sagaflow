package modelcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const (
	ManifestSchemaVersion = 1
	DefaultManifestURL    = "https://github.com/gofurry/sagaflow/releases/latest/download/model-catalog.json"
	maxManifestSize       = int64(4 << 20)
)

var manifestNamespace = uuid.MustParse("9a794da7-a426-4cb0-b1ca-a613b649e71d")

type Manifest struct {
	SchemaVersion  int                        `json:"schema_version"`
	CatalogVersion string                     `json:"catalog_version"`
	PublishedAt    string                     `json:"published_at,omitempty"`
	Profiles       map[string]ManifestProfile `json:"profiles,omitempty"`
	Models         []ManifestModel            `json:"models"`
}

type ManifestProfile struct {
	Capability        string          `json:"capability"`
	InputModalities   []string        `json:"input_modalities"`
	Features          []string        `json:"features"`
	ParameterSchema   json.RawMessage `json:"parameter_schema"`
	DefaultParameters json.RawMessage `json:"default_parameters"`
	SupportStatus     string          `json:"support_status"`
	DocumentationURL  string          `json:"documentation_url,omitempty"`
}

type ManifestModel struct {
	Profile           string          `json:"profile,omitempty"`
	ProviderCode      string          `json:"provider_code"`
	ModelID           string          `json:"model_id"`
	DisplayName       string          `json:"display_name"`
	Capability        string          `json:"capability,omitempty"`
	InputModalities   []string        `json:"input_modalities,omitempty"`
	Features          []string        `json:"features,omitempty"`
	ParameterSchema   json.RawMessage `json:"parameter_schema,omitempty"`
	DefaultParameters json.RawMessage `json:"default_parameters,omitempty"`
	SupportStatus     string          `json:"support_status,omitempty"`
	DocumentationURL  string          `json:"documentation_url,omitempty"`
	Enabled           *bool           `json:"enabled,omitempty"`
}

type ManifestInfo struct {
	Path           string `json:"path"`
	Installed      bool   `json:"installed"`
	SchemaVersion  int    `json:"schema_version"`
	CatalogVersion string `json:"catalog_version"`
	PublishedAt    string `json:"published_at,omitempty"`
	ModelCount     int    `json:"model_count"`
}

func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	return DecodeManifest(io.LimitReader(file, maxManifestSize+1))
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("read model catalog manifest: %w", err)
	}
	if int64(len(data)) > maxManifestSize {
		return Manifest{}, fmt.Errorf("model catalog manifest exceeds %d bytes", maxManifestSize)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode model catalog manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported model catalog schema_version %d", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.CatalogVersion) == "" {
		return errors.New("model catalog catalog_version is required")
	}
	if manifest.PublishedAt != "" {
		if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
			return fmt.Errorf("model catalog published_at must be RFC3339: %w", err)
		}
	}
	for name, profile := range manifest.Profiles {
		if strings.TrimSpace(name) == "" {
			return errors.New("model catalog profile name is required")
		}
		if err := validateResolvedModel(ManifestModel{
			ProviderCode: "profile", ModelID: name, DisplayName: name,
			Capability: profile.Capability, InputModalities: profile.InputModalities,
			Features: profile.Features, ParameterSchema: profile.ParameterSchema,
			DefaultParameters: profile.DefaultParameters, SupportStatus: profile.SupportStatus,
		}); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
	}
	seen := make(map[string]struct{}, len(manifest.Models))
	for index, item := range manifest.Models {
		resolved, err := resolveManifestModel(manifest, item)
		if err != nil {
			return fmt.Errorf("model %d: %w", index, err)
		}
		if err := validateResolvedModel(resolved); err != nil {
			return fmt.Errorf("model %d: %w", index, err)
		}
		key := definitionKey(resolved.ProviderCode, resolved.ModelID, resolved.Capability)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("model %d duplicates %s", index, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateResolvedModel(item ManifestModel) error {
	if strings.TrimSpace(item.ProviderCode) == "" || strings.TrimSpace(item.ModelID) == "" || strings.TrimSpace(item.DisplayName) == "" {
		return errors.New("provider_code, model_id and display_name are required")
	}
	switch item.Capability {
	case "text", "image", "audio", "video":
	default:
		return fmt.Errorf("unsupported capability %q", item.Capability)
	}
	switch item.SupportStatus {
	case "verified", "compatible", "experimental":
	default:
		return fmt.Errorf("unsupported support_status %q", item.SupportStatus)
	}
	if len(item.ParameterSchema) == 0 || !json.Valid(item.ParameterSchema) {
		return errors.New("parameter_schema must be valid JSON")
	}
	if len(item.DefaultParameters) == 0 || !json.Valid(item.DefaultParameters) {
		return errors.New("default_parameters must be valid JSON")
	}
	return nil
}

func resolveManifestModel(manifest Manifest, item ManifestModel) (ManifestModel, error) {
	if item.Profile == "" {
		return item, nil
	}
	profile, ok := manifest.Profiles[item.Profile]
	if !ok {
		return ManifestModel{}, fmt.Errorf("unknown profile %q", item.Profile)
	}
	if item.Capability == "" {
		item.Capability = profile.Capability
	}
	if item.InputModalities == nil {
		item.InputModalities = profile.InputModalities
	}
	if item.Features == nil {
		item.Features = profile.Features
	}
	if len(item.ParameterSchema) == 0 {
		item.ParameterSchema = profile.ParameterSchema
	}
	if len(item.DefaultParameters) == 0 {
		item.DefaultParameters = profile.DefaultParameters
	}
	if item.SupportStatus == "" {
		item.SupportStatus = profile.SupportStatus
	}
	if item.DocumentationURL == "" {
		item.DocumentationURL = profile.DocumentationURL
	}
	return item, nil
}

func manifestDefinitions(manifest Manifest, source string) ([]Definition, error) {
	definitions := make([]Definition, 0, len(manifest.Models))
	for _, item := range manifest.Models {
		resolved, err := resolveManifestModel(manifest, item)
		if err != nil {
			return nil, err
		}
		enabled := true
		if resolved.Enabled != nil {
			enabled = *resolved.Enabled
		}
		key := definitionKey(resolved.ProviderCode, resolved.ModelID, resolved.Capability)
		definitions = append(definitions, Definition{
			ProviderCode: resolved.ProviderCode,
			Model: db.Model{
				ID: uuid.NewSHA1(manifestNamespace, []byte(key)), ModelID: resolved.ModelID,
				DisplayName: resolved.DisplayName, Capability: resolved.Capability,
				InputModalities: resolved.InputModalities, Features: resolved.Features,
				ParameterSchema: resolved.ParameterSchema, DefaultParameters: resolved.DefaultParameters,
				Enabled: enabled, Available: true,
				Metadata: db.JSON(map[string]any{
					"source": "builtin", "catalog_source": source,
					"catalog_version":   manifest.CatalogVersion,
					"support_status":    resolved.SupportStatus,
					"documentation_url": resolved.DocumentationURL,
				}),
			},
		})
	}
	return definitions, nil
}

func ManifestStatus(path string) (ManifestInfo, error) {
	info := ManifestInfo{Path: path}
	manifest, err := LoadManifest(path)
	if errors.Is(err, os.ErrNotExist) {
		return info, nil
	}
	if err != nil {
		return ManifestInfo{}, err
	}
	info.Installed = true
	info.SchemaVersion = manifest.SchemaVersion
	info.CatalogVersion = manifest.CatalogVersion
	info.PublishedAt = manifest.PublishedAt
	info.ModelCount = len(manifest.Models)
	return info, nil
}

func DownloadManifest(ctx context.Context, client *http.Client, sourceURL, destination string) (ManifestInfo, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(sourceURL) == "" {
		sourceURL = DefaultManifestURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return ManifestInfo{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ManifestInfo{}, fmt.Errorf("download model catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ManifestInfo{}, fmt.Errorf("download model catalog: HTTP %d", response.StatusCode)
	}
	manifest, err := DecodeManifest(response.Body)
	if err != nil {
		return ManifestInfo{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ManifestInfo{}, err
	}
	data = append(data, '\n')
	if err := writeManifestAtomically(destination, data); err != nil {
		return ManifestInfo{}, err
	}
	return ManifestStatus(destination)
}

func InstallManifest(source, destination string) (ManifestInfo, error) {
	manifest, err := LoadManifest(source)
	if err != nil {
		return ManifestInfo{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ManifestInfo{}, err
	}
	data = append(data, '\n')
	if err := writeManifestAtomically(destination, data); err != nil {
		return ManifestInfo{}, err
	}
	return ManifestStatus(destination)
}

func writeManifestAtomically(destination string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".model-catalog-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install model catalog: %w", err)
	}
	return nil
}

func definitionKey(providerCode, modelID, capability string) string {
	return strings.ToLower(strings.TrimSpace(providerCode)) + "\x00" +
		strings.TrimSpace(modelID) + "\x00" + strings.ToLower(strings.TrimSpace(capability))
}
