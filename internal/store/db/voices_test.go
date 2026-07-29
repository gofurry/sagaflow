package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/modelcatalog"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	storedb "github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

func TestVoiceProfilesAttachModelSpecificBindings(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "voices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := storedb.New(database)
	if _, err := modelcatalog.Sync(ctx, store); err != nil {
		t.Fatal(err)
	}
	profile, err := store.CreateVoiceProfile(ctx, storedb.VoiceProfile{
		Name: "旁白", Kind: "design", DesignPrompt: "温暖、清晰的青年女声",
	})
	if err != nil {
		t.Fatal(err)
	}
	modelID := uuid.MustParse("20000000-0000-0000-0000-000000000004")
	model, err := store.GetModel(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := store.CreateVoiceBinding(ctx, storedb.VoiceBinding{
		VoiceProfileID: profile.ID, ProviderID: model.ProviderID, ModelID: model.ID,
		Operation: "design", VoiceID: "SagaVoice01",
	})
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := store.ListVoiceProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || len(profiles[0].Bindings) != 1 {
		t.Fatalf("unexpected voice profiles: %#v", profiles)
	}
	got := profiles[0].Bindings[0]
	if got.ID != binding.ID || got.ModelIdentifier != "speech-2.8-hd" || got.ProviderCode != "minimax" {
		t.Fatalf("unexpected attached binding: %#v", got)
	}
	if err := store.ConfirmDeleteVoiceBinding(ctx, binding.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmDeleteVoiceProfile(ctx, profile.ID); err != nil {
		t.Fatal(err)
	}
}
