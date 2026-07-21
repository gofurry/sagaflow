package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LocalStore struct{ root string }

func NewLocalStore(dataDir string) LocalStore {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	return LocalStore{root: filepath.Join(dataDir, "objects")}
}
func (LocalStore) Backend() string { return BackendLocal }
func (s LocalStore) Upload(_ context.Context, input UploadInput) (Object, error) {
	key := filepath.ToSlash(strings.TrimLeft(strings.TrimSpace(input.Key), "/\\"))
	if key == "" {
		return Object{}, fmt.Errorf("storage key is required")
	}
	path := filepath.Join(s.root, filepath.FromSlash(key))
	root, err := filepath.Abs(s.root)
	if err != nil {
		return Object{}, err
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return Object{}, err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return Object{}, fmt.Errorf("storage key escapes local root")
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return Object{}, err
	}
	body, _, err := input.body()
	if err != nil {
		return Object{}, err
	}
	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Object{}, err
	}
	if _, err = io.Copy(file, body); err != nil {
		_ = file.Close()
		_ = os.Remove(resolved)
		return Object{}, err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(resolved)
		return Object{}, err
	}
	return Object{Backend: BackendLocal, Key: key}, nil
}
func (LocalStore) PresignGet(context.Context, Object, time.Duration) (SignedURL, error) {
	return SignedURL{}, ErrRemoteURLUnavailable
}
func (s LocalStore) Read(_ context.Context, object Object) ([]byte, error) {
	key := filepath.ToSlash(strings.TrimLeft(strings.TrimSpace(object.Key), "/\\"))
	path := filepath.Join(s.root, filepath.FromSlash(key))
	root, _ := filepath.Abs(s.root)
	resolved, _ := filepath.Abs(path)
	if resolved != root && !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("storage key escapes local root")
	}
	return os.ReadFile(resolved)
}
func (s LocalStore) Delete(_ context.Context, object Object) error {
	key := filepath.ToSlash(strings.TrimLeft(strings.TrimSpace(object.Key), "/\\"))
	path := filepath.Join(s.root, filepath.FromSlash(key))
	root, _ := filepath.Abs(s.root)
	resolved, _ := filepath.Abs(path)
	if resolved != root && !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return fmt.Errorf("storage key escapes local root")
	}
	if err := os.Remove(resolved); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
