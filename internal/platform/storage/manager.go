package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofurry/sagaflow/internal/media"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

// Manager owns SagaFlow's canonical local files. Remote object storage is an
// explicit publishing destination and is deliberately not part of this type.
type Manager struct {
	catalog *db.Store
	root    string
	temp    string
}

type ManagedUploadInput struct {
	ProjectID    *uuid.UUID
	Purpose      string
	OriginalName string
	UploadInput
}

type ManagedObject struct {
	Record db.LocalObject
	Object Object
}

func NewManager(catalog *db.Store, objectDir, tempDir string) (*Manager, error) {
	if catalog == nil {
		return nil, fmt.Errorf("storage catalog is required")
	}
	for _, dir := range []string{objectDir, tempDir} {
		if strings.TrimSpace(dir) == "" {
			return nil, fmt.Errorf("storage directory is required")
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create storage directory %s: %w", dir, err)
		}
	}
	return &Manager{catalog: catalog, root: objectDir, temp: tempDir}, nil
}

func (m *Manager) Backend() string { return BackendLocal }

func (m *Manager) UploadManaged(ctx context.Context, input ManagedUploadInput) (ManagedObject, error) {
	if strings.TrimSpace(input.Purpose) == "" {
		return ManagedObject{}, fmt.Errorf("storage purpose is required")
	}
	body, declaredSize, err := input.UploadInput.body()
	if err != nil {
		return ManagedObject{}, err
	}
	id := uuid.New()
	record, err := m.catalog.CreateLocalObject(ctx, db.LocalObject{
		ID: id, ProjectID: input.ProjectID, OriginalName: safeObjectName(input.OriginalName, input.ContentType),
		Purpose: strings.Trim(strings.ReplaceAll(input.Purpose, "..", ""), "/"), MimeType: input.ContentType,
		SizeBytes: declaredSize, State: "pending",
	})
	if err != nil {
		return ManagedObject{}, err
	}

	tmp, err := os.CreateTemp(m.temp, "upload-*")
	if err != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		if copyErr != nil {
			return ManagedObject{}, copyErr
		}
		return ManagedObject{}, closeErr
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	ext := strings.ToLower(filepath.Ext(record.OriginalName))
	key := filepath.ToSlash(filepath.Join(digest[:2], digest+ext))
	destination, err := safeLocalPath(m.root, key)
	if err != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, err
	}
	if _, statErr := os.Stat(destination); os.IsNotExist(statErr) {
		if err := os.Rename(tmpName, destination); err != nil {
			_ = m.catalog.MarkLocalObjectFailed(ctx, id)
			return ManagedObject{}, err
		}
	} else if statErr != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, statErr
	}
	record, err = m.catalog.MarkLocalObjectReady(ctx, id, key, digest, size)
	if err != nil {
		return ManagedObject{}, err
	}
	return ManagedObject{Record: record, Object: Object{ID: id.String(), Backend: BackendLocal, Key: key}}, nil
}

func (m *Manager) Object(ctx context.Context, id uuid.UUID) (Object, error) {
	record, err := m.catalog.GetLocalObject(ctx, id)
	if err != nil {
		return Object{}, err
	}
	return Object{ID: id.String(), Backend: BackendLocal, Key: record.ObjectKey}, nil
}

func (m *Manager) Open(ctx context.Context, id uuid.UUID) (*os.File, db.LocalObject, error) {
	record, err := m.catalog.GetLocalObject(ctx, id)
	if err != nil {
		return nil, db.LocalObject{}, err
	}
	path, err := safeLocalPath(m.root, record.ObjectKey)
	if err != nil {
		return nil, db.LocalObject{}, err
	}
	file, err := os.Open(path)
	return file, record, err
}

func (m *Manager) Read(ctx context.Context, object Object) ([]byte, error) {
	if object.ID != "" {
		id, err := uuid.Parse(object.ID)
		if err != nil {
			return nil, err
		}
		object, err = m.Object(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	path, err := safeLocalPath(m.root, object.Key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (m *Manager) DeleteManaged(ctx context.Context, id uuid.UUID) error {
	record, err := m.catalog.MarkLocalObjectDeleting(ctx, id)
	if err != nil {
		return err
	}
	// Content-addressed files may be shared. Backup/compact tooling can later
	// garbage-collect bytes that no logical object references.
	if err := m.catalog.MarkLocalObjectDeleted(ctx, record.ID); err != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return err
	}
	return nil
}

func (m *Manager) CanProvideProviderURL(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func safeLocalPath(root, key string) (string, error) {
	key = filepath.ToSlash(strings.TrimLeft(strings.TrimSpace(key), "/\\"))
	if key == "" {
		return "", fmt.Errorf("storage key is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(key)))
	if err != nil {
		return "", err
	}
	if resolved != rootAbs && !strings.HasPrefix(resolved, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage key escapes local root")
	}
	return resolved, nil
}

func safeObjectName(name, mimeType string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	if name == "" || name == "." {
		name = "object.bin"
	}
	name = strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	if filepath.Ext(name) == "" {
		name += media.ExtensionForMIME(mimeType)
	}
	return name
}
