// Package modelcatalog owns SagaFlow's versioned built-in cloud model catalog.
// User-created catalog entries remain database-owned and are never rewritten.
package modelcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/gofurry/sagaflow/internal/store/db"
)

type Definition struct {
	Model        db.Model
	ProviderCode string
}

type SyncResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func Sync(ctx context.Context, store *db.Store) (SyncResult, error) {
	return SyncFromPath(ctx, store, "")
}

func SyncFromPath(ctx context.Context, store *db.Store, manifestPath string) (SyncResult, error) {
	if store == nil {
		return SyncResult{}, fmt.Errorf("model catalog store is required")
	}
	definitions := Builtins()
	if manifestPath != "" {
		manifest, loadErr := LoadManifest(manifestPath)
		if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
			return SyncResult{}, fmt.Errorf("load external model catalog: %w", loadErr)
		}
		if loadErr == nil {
			external, definitionErr := manifestDefinitions(manifest, "external")
			if definitionErr != nil {
				return SyncResult{}, definitionErr
			}
			definitions = mergeDefinitions(definitions, external)
		}
	}
	providers, err := store.ListModelProviders(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	providerByCode := make(map[string]db.ModelProvider, len(providers))
	for _, provider := range providers {
		providerByCode[provider.Code] = provider
	}
	existing, err := store.ListModels(ctx, "")
	if err != nil {
		return SyncResult{}, err
	}
	byID := make(map[string]db.Model, len(existing))
	byKey := make(map[string]db.Model, len(existing))
	for _, model := range existing {
		byID[model.ID.String()] = model
		byKey[catalogKey(model.ProviderID.String(), model.ModelID, model.Capability)] = model
	}

	result := SyncResult{}
	for _, definition := range definitions {
		provider, ok := providerByCode[definition.ProviderCode]
		if !ok {
			return SyncResult{}, fmt.Errorf("built-in model provider %q is missing", definition.ProviderCode)
		}
		desired := definition.Model
		desired.ProviderID = provider.ID
		current, foundByID := byID[desired.ID.String()]
		if !foundByID {
			current, foundByID = byKey[catalogKey(provider.ID.String(), desired.ModelID, desired.Capability)]
		}
		if !foundByID {
			created, createErr := store.CreateModel(ctx, desired)
			if createErr != nil {
				return SyncResult{}, fmt.Errorf("create built-in model %s: %w", desired.ModelID, createErr)
			}
			byID[created.ID.String()] = created
			byKey[catalogKey(created.ProviderID.String(), created.ModelID, created.Capability)] = created
			result.Created++
			continue
		}
		if current.ID != desired.ID && !isBuiltin(current.Metadata) {
			result.Skipped++
			continue
		}
		desired.ID = current.ID
		desired.Enabled = current.Enabled
		if equivalent(current, desired) {
			continue
		}
		if _, updateErr := store.UpdateModel(ctx, desired); updateErr != nil {
			return SyncResult{}, fmt.Errorf("update built-in model %s: %w", desired.ModelID, updateErr)
		}
		result.Updated++
	}
	return result, nil
}

func ValidateProviders(ctx context.Context, store *db.Store, manifest Manifest) error {
	if store == nil {
		return fmt.Errorf("model catalog store is required")
	}
	definitions, err := manifestDefinitions(manifest, "external")
	if err != nil {
		return err
	}
	providers, err := store.ListModelProviders(ctx)
	if err != nil {
		return err
	}
	available := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		available[provider.Code] = struct{}{}
	}
	for _, definition := range definitions {
		if _, ok := available[definition.ProviderCode]; !ok {
			return fmt.Errorf("model provider %q requires a newer SagaFlow binary", definition.ProviderCode)
		}
	}
	return nil
}

func mergeDefinitions(base, overlays []Definition) []Definition {
	merged := append([]Definition(nil), base...)
	indexByKey := make(map[string]int, len(merged))
	for index, definition := range merged {
		indexByKey[definitionKey(definition.ProviderCode, definition.Model.ModelID, definition.Model.Capability)] = index
	}
	for _, overlay := range overlays {
		key := definitionKey(overlay.ProviderCode, overlay.Model.ModelID, overlay.Model.Capability)
		if index, exists := indexByKey[key]; exists {
			overlay.Model.ID = merged[index].Model.ID
			merged[index] = overlay
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, overlay)
	}
	return merged
}

func catalogKey(providerID, modelID, capability string) string {
	return providerID + "\x00" + modelID + "\x00" + capability
}

func isBuiltin(metadata json.RawMessage) bool {
	var value struct {
		Source string `json:"source"`
	}
	return json.Unmarshal(metadata, &value) == nil && value.Source == "builtin"
}

func equivalent(current, desired db.Model) bool {
	return current.ProviderID == desired.ProviderID &&
		current.ModelID == desired.ModelID &&
		current.DisplayName == desired.DisplayName &&
		current.Capability == desired.Capability &&
		reflect.DeepEqual(current.InputModalities, desired.InputModalities) &&
		reflect.DeepEqual(current.Features, desired.Features) &&
		jsonEqual(current.ParameterSchema, desired.ParameterSchema) &&
		jsonEqual(current.DefaultParameters, desired.DefaultParameters) &&
		current.Available == desired.Available &&
		jsonEqual(current.Metadata, desired.Metadata)
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftBuffer, rightBuffer bytes.Buffer
	if json.Compact(&leftBuffer, left) != nil || json.Compact(&rightBuffer, right) != nil {
		return bytes.Equal(left, right)
	}
	return bytes.Equal(leftBuffer.Bytes(), rightBuffer.Bytes())
}
