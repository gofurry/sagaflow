package storage

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalStoreStreamingUpload(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	data := []byte("streamed artifact")
	object, err := store.Upload(context.Background(), UploadInput{
		Key: "generated/result.bin", Reader: bytes.NewReader(data), Size: int64(len(data)), ContentType: "application/octet-stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Read(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatalf("stored data mismatch: %q", stored)
	}
}

func TestUploadInputRejectsAmbiguousBody(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	_, err := store.Upload(context.Background(), UploadInput{
		Key: "generated/result.bin", Data: []byte("data"), Reader: bytes.NewReader([]byte("reader")), Size: 6,
	})
	if err == nil {
		t.Fatal("expected ambiguous upload body to fail")
	}
}
