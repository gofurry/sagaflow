package modelcatalog

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

const (
	ManifestSchemaVersion = 2
	DefaultManifestURL    = "https://github.com/gofurry/sagaflow/releases/latest/download/model-catalog.json"
	maxManifestSize       = int64(4 << 20)
)

var manifestNamespace = uuid.MustParse("9a794da7-a426-4cb0-b1ca-a613b649e71d")

//go:embed model-catalog.json
var defaultManifestData []byte

type Manifest struct {
	SchemaVersion  int                        `json:"schema_version"`
	CatalogVersion string                     `json:"catalog_version"`
	PublishedAt    string                     `json:"published_at,omitempty"`
	Profiles       map[string]ManifestProfile `json:"profiles,omitempty"`
	Models         []ManifestModel            `json:"models"`
}

type ManifestProfile struct {
	Task              string          `json:"task,omitempty"`
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
	Task              string          `json:"task,omitempty"`
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
	LifecycleStatus   string          `json:"lifecycle_status,omitempty"`
	DeprecatedAt      string          `json:"deprecated_at,omitempty"`
	SunsetAt          string          `json:"sunset_at,omitempty"`
	Replacement       *ModelReference `json:"replacement,omitempty"`
	LifecycleMessage  string          `json:"lifecycle_message,omitempty"`
}

type ModelReference struct {
	ProviderCode string `json:"provider_code"`
	ModelID      string `json:"model_id"`
	Capability   string `json:"capability"`
}

type ManifestInfo struct {
	Path           string `json:"path"`
	Installed      bool   `json:"installed"`
	SchemaVersion  int    `json:"schema_version"`
	CatalogVersion string `json:"catalog_version"`
	PublishedAt    string `json:"published_at,omitempty"`
	ModelCount     int    `json:"model_count"`
}

type ManifestDiff struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Retired int `json:"retired"`
	Removed int `json:"removed"`
}

func EmbeddedManifest() (Manifest, error) {
	return DecodeManifest(strings.NewReader(string(defaultManifestData)))
}

func EffectiveManifest(path string) (Manifest, error) {
	embedded, err := EmbeddedManifest()
	if err != nil {
		return Manifest{}, err
	}
	if strings.TrimSpace(path) != "" {
		manifest, loadErr := LoadManifest(path)
		if loadErr == nil {
			return mergeManifests(embedded, manifest)
		}
		if !errors.Is(loadErr, os.ErrNotExist) {
			return Manifest{}, loadErr
		}
	}
	return embedded, nil
}

func mergeManifests(base, overlay Manifest) (Manifest, error) {
	baseModels, err := resolvedManifestModels(base)
	if err != nil {
		return Manifest{}, err
	}
	overlayModels, err := resolvedManifestModels(overlay)
	if err != nil {
		return Manifest{}, err
	}
	for key, model := range overlayModels {
		baseModels[key] = model
	}
	keys := make([]string, 0, len(baseModels))
	for key := range baseModels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	models := make([]ManifestModel, 0, len(keys))
	for _, key := range keys {
		models = append(models, baseModels[key])
	}
	result := Manifest{
		SchemaVersion: overlay.SchemaVersion, CatalogVersion: overlay.CatalogVersion,
		PublishedAt: overlay.PublishedAt, Models: models,
	}
	if result.SchemaVersion == 0 {
		result.SchemaVersion = base.SchemaVersion
	}
	if result.CatalogVersion == "" {
		result.CatalogVersion = base.CatalogVersion
	}
	if result.PublishedAt == "" {
		result.PublishedAt = base.PublishedAt
	}
	if err := ValidateManifest(result); err != nil {
		return Manifest{}, err
	}
	return result, nil
}

func CompareManifests(current, next Manifest) (ManifestDiff, error) {
	currentModels, err := resolvedManifestModels(current)
	if err != nil {
		return ManifestDiff{}, err
	}
	nextModels, err := resolvedManifestModels(next)
	if err != nil {
		return ManifestDiff{}, err
	}
	diff := ManifestDiff{}
	for key, nextModel := range nextModels {
		currentModel, exists := currentModels[key]
		if !exists {
			diff.Added++
		} else if !reflect.DeepEqual(currentModel, nextModel) {
			diff.Updated++
		}
		if nextModel.LifecycleStatus == "retired" && (!exists || currentModel.LifecycleStatus != "retired") {
			diff.Retired++
		}
	}
	for key := range currentModels {
		if _, exists := nextModels[key]; !exists {
			diff.Removed++
		}
	}
	return diff, nil
}

func resolvedManifestModels(manifest Manifest) (map[string]ManifestModel, error) {
	result := make(map[string]ManifestModel, len(manifest.Models))
	for _, item := range manifest.Models {
		resolved, err := resolveManifestModel(manifest, item)
		if err != nil {
			return nil, err
		}
		resolved.Profile = ""
		resolved.ParameterSchema = compactJSON(resolved.ParameterSchema)
		resolved.DefaultParameters = compactJSON(resolved.DefaultParameters)
		if len(resolved.InputModalities) == 0 {
			resolved.InputModalities = nil
		}
		if len(resolved.Features) == 0 {
			resolved.Features = nil
		}
		result[definitionKey(resolved.ProviderCode, resolved.ModelID, resolved.Capability)] = resolved
	}
	return result, nil
}

func compactJSON(value json.RawMessage) json.RawMessage {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, value); err != nil {
		return value
	}
	return buffer.Bytes()
}

func LoadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	return DecodeManifest(io.LimitReader(file, maxManifestSize+1))
}

func embeddedDefinitions() ([]Definition, error) {
	manifest, err := DecodeManifest(strings.NewReader(string(defaultManifestData)))
	if err != nil {
		return nil, fmt.Errorf("decode embedded model catalog: %w", err)
	}
	return manifestDefinitions(manifest, "embedded")
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
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > ManifestSchemaVersion {
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
			Task:       profile.Task,
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
		if err := validateLifecycle(resolved); err != nil {
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

func validateLifecycle(item ManifestModel) error {
	switch item.LifecycleStatus {
	case "active", "deprecated", "retired":
	default:
		return fmt.Errorf("unsupported lifecycle_status %q", item.LifecycleStatus)
	}
	for field, value := range map[string]string{"deprecated_at": item.DeprecatedAt, "sunset_at": item.SunsetAt} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return fmt.Errorf("%s must be RFC3339: %w", field, err)
		}
	}
	if item.Replacement != nil {
		if strings.TrimSpace(item.Replacement.ProviderCode) == "" || strings.TrimSpace(item.Replacement.ModelID) == "" {
			return errors.New("replacement provider_code and model_id are required")
		}
		switch item.Replacement.Capability {
		case "text", "image", "audio", "video":
		default:
			return fmt.Errorf("replacement has unsupported capability %q", item.Replacement.Capability)
		}
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
	switch item.Task {
	case "", "chat", "image_generation", "image_edit", "speech_generation", "speech_recognition", "text_to_video", "image_to_video", "start_end_video", "reference_to_video":
	default:
		return fmt.Errorf("unsupported task %q", item.Task)
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
	if item.Profile != "" {
		profile, ok := manifest.Profiles[item.Profile]
		if !ok {
			return ManifestModel{}, fmt.Errorf("unknown profile %q", item.Profile)
		}
		if item.Capability == "" {
			item.Capability = profile.Capability
		}
		if item.Task == "" {
			item.Task = profile.Task
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
	}
	if item.LifecycleStatus == "" {
		item.LifecycleStatus = "active"
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
				Enabled: enabled, Available: resolved.LifecycleStatus != "retired",
				Metadata: db.JSON(map[string]any{
					"source": "builtin", "catalog_source": source,
					"catalog_version":   manifest.CatalogVersion,
					"support_status":    resolved.SupportStatus,
					"task":              resolved.Task,
					"documentation_url": resolved.DocumentationURL,
					"lifecycle_status":  resolved.LifecycleStatus,
					"deprecated_at":     resolved.DeprecatedAt,
					"sunset_at":         resolved.SunsetAt,
					"replacement":       resolved.Replacement,
					"lifecycle_message": resolved.LifecycleMessage,
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
	manifest, err := FetchManifest(ctx, client, sourceURL)
	if err != nil {
		return ManifestInfo{}, err
	}
	return installManifest(destination, manifest)
}

func FetchManifest(ctx context.Context, client *http.Client, sourceURL string) (Manifest, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(sourceURL) == "" {
		sourceURL = DefaultManifestURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return Manifest{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Manifest{}, fmt.Errorf("download model catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Manifest{}, fmt.Errorf("download model catalog: HTTP %d", response.StatusCode)
	}
	manifest, err := DecodeManifest(response.Body)
	if err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func InstallManifest(source, destination string) (ManifestInfo, error) {
	manifest, err := LoadManifest(source)
	if err != nil {
		return ManifestInfo{}, err
	}
	return installManifest(destination, manifest)
}

func InstallManifestContent(reader io.Reader, destination string) (ManifestInfo, error) {
	manifest, err := DecodeManifest(reader)
	if err != nil {
		return ManifestInfo{}, err
	}
	return installManifest(destination, manifest)
}

func InstallManifestDocument(manifest Manifest, destination string) (ManifestInfo, error) {
	if err := ValidateManifest(manifest); err != nil {
		return ManifestInfo{}, err
	}
	return installManifest(destination, manifest)
}

func installManifest(destination string, manifest Manifest) (ManifestInfo, error) {
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
	previous := destination + ".previous"
	if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(temporaryPath, destination); err != nil {
			return fmt.Errorf("install model catalog: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect existing model catalog: %w", err)
	}
	if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace previous model catalog: %w", err)
	}
	if err := os.Rename(destination, previous); err != nil {
		return fmt.Errorf("preserve previous model catalog: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		_ = os.Rename(previous, destination)
		return fmt.Errorf("install model catalog: %w", err)
	}
	return nil
}

func definitionKey(providerCode, modelID, capability string) string {
	return strings.ToLower(strings.TrimSpace(providerCode)) + "\x00" +
		strings.TrimSpace(modelID) + "\x00" + strings.ToLower(strings.TrimSpace(capability))
}
