package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestManagerStoresCanonicalContentAddressedObjects(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := sqlite.Open(ctx, filepath.Join(root, "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager, err := NewManager(db.New(database), filepath.Join(root, "objects"), filepath.Join(root, "temp"))
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
	if first.Record.ID == second.Record.ID || first.Record.ObjectKey != second.Record.ObjectKey {
		t.Fatalf("expected distinct logical records sharing one physical key: first=%+v second=%+v", first.Record, second.Record)
	}
	data, err := manager.Read(ctx, first.Object)
	if err != nil || string(data) != "hello" {
		t.Fatalf("read stored object: %q, %v", data, err)
	}
}
