package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackendLocal = "local"
	BackendS3    = "s3"
)

var (
	ErrRemoteURLUnavailable = errors.New("remote storage URL unavailable")
	ErrStorageNotConfigured = errors.New("no enabled storage connection is configured")
)

type Store interface {
	Backend() string
	Upload(ctx context.Context, input UploadInput) (Object, error)
	PresignGet(ctx context.Context, object Object, ttl time.Duration) (SignedURL, error)
	Read(ctx context.Context, object Object) ([]byte, error)
	Delete(ctx context.Context, object Object) error
}

func normalizeObjectKey(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	return strings.Trim(value, "/")
}

type UploadInput struct {
	Key         string
	Data        []byte
	Reader      io.Reader
	Size        int64
	ContentType string
}

func (input UploadInput) body() (io.Reader, int64, error) {
	if input.Reader != nil && input.Data != nil {
		return nil, 0, fmt.Errorf("storage upload accepts either Data or Reader, not both")
	}
	if input.Reader != nil {
		if input.Size < 0 {
			return nil, 0, fmt.Errorf("streaming storage upload requires a non-negative size")
		}
		return input.Reader, input.Size, nil
	}
	return bytes.NewReader(input.Data), int64(len(input.Data)), nil
}

type Object struct {
	ID        string
	BackendID string
	Backend   string
	Bucket    string
	Key       string
	URL       string
}

type SignedURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
