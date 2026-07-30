package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

func TestManagerStoresCanonicalContentAddressedObjects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := sqlite.Open(ctx, filepath.Join(root, "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager, err := NewManager(db.New(database), root, filepath.Join(root, "temp"))
	if err != nil {
		t.Fatal(err)
	}
	input := ManagedUploadInput{Purpose: "asset", OriginalName: "note.txt", UploadInput: UploadInput{Data: []byte("hello"), ContentType: "text/plain"}}
	first, err := manager.UploadManaged(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.UploadManaged(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.ID == second.Record.ID || first.Record.ObjectKey == second.Record.ObjectKey {
		t.Fatalf("expected distinct logical records and readable project paths: first=%+v second=%+v", first.Record, second.Record)
	}
	if !strings.HasPrefix(first.Record.ObjectKey, "shared/asset/") || filepath.Base(first.Record.ObjectKey) != "note.txt" {
		t.Fatalf("unexpected managed object path: %s", first.Record.ObjectKey)
	}
	data, err := manager.Read(ctx, first.Object)
	if err != nil || string(data) != "hello" {
		t.Fatalf("read stored object: %q, %v", data, err)
	}
	if err := manager.DeleteManaged(ctx, first.Record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(first.Record.ObjectKey))); !os.IsNotExist(err) {
		t.Fatalf("expected deleted managed file, got %v", err)
	}
}

func TestManagerMigratesLegacyHashObjectIntoProjectLayout(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := sqlite.Open(ctx, filepath.Join(root, "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	catalog := db.New(database)
	project, err := catalog.CreateProject(ctx, "Story", "", "16:9", "1920x1080", nil)
	if err != nil {
		t.Fatal(err)
	}
	group, err := catalog.CreateAssetGroup(ctx, db.AssetGroup{ProjectID: project.ID, Kind: "character", Name: "Wolf"})
	if err != nil {
		t.Fatal(err)
	}
	legacyRoot := filepath.Join(root, "objects")
	legacyKey := "ab/legacy.txt"
	if err := os.MkdirAll(filepath.Join(legacyRoot, "ab"), 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("legacy")
	if err := os.WriteFile(filepath.Join(legacyRoot, filepath.FromSlash(legacyKey)), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	object, err := catalog.CreateLocalObject(ctx, db.LocalObject{ID: uuid.New(), ProjectID: &project.ID, OriginalName: "wolf.png", Purpose: "asset", MimeType: "image/png", SizeBytes: int64(len(payload)), State: "ready", ObjectKey: legacyKey, SHA256: hex.EncodeToString(hash[:])})
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := catalog.CreateAsset(ctx, db.CreateAssetInput{ID: assetID, ProjectID: project.ID, GroupID: &group.ID, ObjectID: object.ID, Name: "Wolf", MediaType: "image", MimeType: "image/png", FileSizeBytes: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(catalog, root, filepath.Join(root, "temp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.MigrateLegacyLayout(ctx, legacyRoot); err != nil {
		t.Fatal(err)
	}
	migrated, err := catalog.GetLocalObject(ctx, object.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.ToSlash(filepath.Join("projects", project.ID.String(), "assets", "character", group.ID.String(), assetID.String(), "wolf.png"))
	if migrated.ObjectKey != want {
		t.Fatalf("migrated key = %q, want %q", migrated.ObjectKey, want)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacyRoot, filepath.FromSlash(legacyKey))); !os.IsNotExist(err) {
		t.Fatalf("legacy file still exists: %v", err)
	}
}
