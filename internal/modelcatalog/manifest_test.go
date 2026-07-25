package modelcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validManifest = `{
  "schema_version": 2,
  "catalog_version": "2026.07.23.1",
  "published_at": "2026-07-23T12:00:00Z",
  "profiles": {
    "chat": {
      "capability": "text",
      "input_modalities": ["text"],
      "features": ["chat"],
      "parameter_schema": {"type":"object","properties":{"temperature":{"type":"number"}}},
      "default_parameters": {"temperature":0.7},
      "support_status": "compatible"
    }
  },
  "models": [{
    "profile": "chat",
    "provider_code": "example",
    "model_id": "example/chat",
    "display_name": "Example Chat",
    "lifecycle_status": "deprecated",
    "deprecated_at": "2026-07-01T00:00:00Z",
    "sunset_at": "2026-12-01T00:00:00Z",
    "replacement": {
      "provider_code": "example",
      "model_id": "example/chat-next",
      "capability": "text"
    },
    "lifecycle_message": "Use the next model"
  }]
}`

func TestDecodeManifestResolvesProfiles(t *testing.T) {
	manifest, err := DecodeManifest(strings.NewReader(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := manifestDefinitions(manifest, "external")
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 {
		t.Fatalf("expected one definition, got %d", len(definitions))
	}
	model := definitions[0].Model
	if model.Capability != "text" || model.ModelID != "example/chat" {
		t.Fatalf("unexpected resolved model: %+v", model)
	}
	if !strings.Contains(string(model.Metadata), `"support_status":"compatible"`) {
		t.Fatalf("expected compatible metadata, got %s", model.Metadata)
	}
	if !strings.Contains(string(model.Metadata), `"lifecycle_status":"deprecated"`) || !model.Available {
		t.Fatalf("expected available deprecated model metadata, got %s", model.Metadata)
	}
}

func TestDecodeManifestRejectsUnknownFieldsAndStatus(t *testing.T) {
	for name, value := range map[string]string{
		"unknown field":     strings.Replace(validManifest, `"models":`, `"unexpected":true,"models":`, 1),
		"unknown status":    strings.Replace(validManifest, `"compatible"`, `"untested"`, 1),
		"unknown lifecycle": strings.Replace(validManifest, `"deprecated"`, `"abandoned"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeManifest(strings.NewReader(value)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDecodeManifestKeepsVersionOneActiveByDefault(t *testing.T) {
	legacy := strings.Replace(validManifest, `"schema_version": 2`, `"schema_version": 1`, 1)
	legacy = strings.Replace(legacy, `,
    "lifecycle_status": "deprecated",
    "deprecated_at": "2026-07-01T00:00:00Z",
    "sunset_at": "2026-12-01T00:00:00Z",
    "replacement": {
      "provider_code": "example",
      "model_id": "example/chat-next",
      "capability": "text"
    },
    "lifecycle_message": "Use the next model"`, "", 1)
	manifest, err := DecodeManifest(strings.NewReader(legacy))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := manifestDefinitions(manifest, "external")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(definitions[0].Model.Metadata), `"lifecycle_status":"active"`) {
		t.Fatalf("expected version one model to default active, got %s", definitions[0].Model.Metadata)
	}
}

func TestRetiredManifestModelBecomesUnavailable(t *testing.T) {
	retired := strings.Replace(validManifest, `"lifecycle_status": "deprecated"`, `"lifecycle_status": "retired"`, 1)
	manifest, err := DecodeManifest(strings.NewReader(retired))
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := manifestDefinitions(manifest, "external")
	if err != nil {
		t.Fatal(err)
	}
	if definitions[0].Model.Available {
		t.Fatal("expected retired model to be unavailable")
	}
	if !definitions[0].Model.Enabled {
		t.Fatal("retiring a model must not erase its enabled preference")
	}
}

func TestDownloadManifestValidatesBeforeAtomicInstall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validManifest))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "catalog", "model-catalog.json")
	info, err := DownloadManifest(context.Background(), server.Client(), server.URL, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Installed || info.CatalogVersion != "2026.07.23.1" || info.ModelCount != 1 {
		t.Fatalf("unexpected manifest info: %+v", info)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestInstallManifestPreservesPreviousVersion(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "catalog", "model-catalog.json")
	first, err := DecodeManifest(strings.NewReader(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installManifest(destination, first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.CatalogVersion = "2026.07.24.1"
	if _, err := installManifest(destination, second); err != nil {
		t.Fatal(err)
	}
	previous, err := LoadManifest(destination + ".previous")
	if err != nil {
		t.Fatal(err)
	}
	if previous.CatalogVersion != first.CatalogVersion {
		t.Fatalf("expected previous version %q, got %q", first.CatalogVersion, previous.CatalogVersion)
	}
}

func TestCompareManifestsReportsVisibleChanges(t *testing.T) {
	current, err := DecodeManifest(strings.NewReader(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Models = append([]ManifestModel(nil), current.Models...)
	next.Models[0].DisplayName = "Updated Chat"
	added := next.Models[0]
	added.ModelID = "example/chat-next"
	added.LifecycleStatus = "retired"
	next.Models = append(next.Models, added)
	diff, err := CompareManifests(current, next)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Added != 1 || diff.Updated != 1 || diff.Retired != 1 || diff.Removed != 0 {
		t.Fatalf("unexpected manifest diff: %+v", diff)
	}
}

func TestMergeManifestsKeepsUnchangedBuiltIns(t *testing.T) {
	base, err := DecodeManifest(strings.NewReader(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	second := base.Models[0]
	second.ModelID = "example/second"
	base.Models = append(base.Models, second)
	overlay := base
	overlay.Models = append([]ManifestModel(nil), base.Models[:1]...)
	overlay.Models[0].DisplayName = "Overridden"
	merged, err := mergeManifests(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Models) != 2 {
		t.Fatalf("expected partial overlay to keep both models, got %d", len(merged.Models))
	}
}

func TestMergeDefinitionsPreservesBuiltInID(t *testing.T) {
	base := Builtins()[:1]
	overlay := base[0]
	originalID := base[0].Model.ID
	overlay.Model.ID = manifestNamespace
	overlay.Model.DisplayName = "Updated"
	merged := mergeDefinitions(base, []Definition{overlay})
	if len(merged) != 1 || merged[0].Model.ID != originalID || merged[0].Model.DisplayName != "Updated" {
		t.Fatalf("unexpected merge result: %+v", merged)
	}
}
