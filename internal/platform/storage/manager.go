package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
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
	OwnerID      *uuid.UUID
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
	purpose, err := safePurpose(input.Purpose)
	if err != nil {
		return ManagedObject{}, err
	}
	body, declaredSize, err := input.UploadInput.body()
	if err != nil {
		return ManagedObject{}, err
	}
	id := uuid.New()
	record, err := m.catalog.CreateLocalObject(ctx, db.LocalObject{
		ID: id, ProjectID: input.ProjectID, OriginalName: safeObjectName(input.OriginalName, input.ContentType),
		Purpose: purpose, MimeType: input.ContentType,
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
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(body, declaredSize+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		if copyErr != nil {
			return ManagedObject{}, copyErr
		}
		return ManagedObject{}, closeErr
	}
	if size != declaredSize {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, fmt.Errorf("storage upload size mismatch: received %d bytes, expected %d", size, declaredSize)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	ownerID := record.ID
	if input.OwnerID != nil && *input.OwnerID != uuid.Nil {
		ownerID = *input.OwnerID
	}
	key := managedObjectKey(record.ProjectID, purpose, ownerID, record.OriginalName)
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
	} else {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return ManagedObject{}, fmt.Errorf("managed object path already exists")
	}
	record, err = m.catalog.MarkLocalObjectReady(ctx, id, key, digest, size)
	if err != nil {
		return ManagedObject{}, err
	}
	return ManagedObject{Record: record, Object: Object{ID: id.String(), Backend: BackendLocal, Key: key}}, nil
}

func (m *Manager) MigrateLegacyLayout(ctx context.Context, legacyRoot string) error {
	records, err := m.catalog.ListReadyLocalObjects(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		placement, err := m.catalog.ResolveLocalObjectPlacement(ctx, record)
		if err != nil {
			return fmt.Errorf("resolve local object %s placement: %w", record.ID, err)
		}
		destinationKey := managedObjectKey(record.ProjectID, placement.Purpose, placement.OwnerID, record.OriginalName)
		if destinationKey == record.ObjectKey {
			continue
		}
		sourceRoot := m.root
		if !isManagedObjectKey(record.ObjectKey) {
			sourceRoot = legacyRoot
		}
		if err := m.relocateRecord(ctx, record, sourceRoot, destinationKey); err != nil {
			return fmt.Errorf("migrate local object %s: %w", record.ID, err)
		}
	}
	return removeEmptyDirectories(legacyRoot)
}

func (m *Manager) ReconcileManaged(ctx context.Context, id uuid.UUID) error {
	record, err := m.catalog.GetLocalObject(ctx, id)
	if err != nil {
		return err
	}
	placement, err := m.catalog.ResolveLocalObjectPlacement(ctx, record)
	if err != nil {
		return err
	}
	destinationKey := managedObjectKey(record.ProjectID, placement.Purpose, placement.OwnerID, record.OriginalName)
	if destinationKey == record.ObjectKey {
		return nil
	}
	return m.relocateRecord(ctx, record, m.root, destinationKey)
}

func (m *Manager) Path(ctx context.Context, id uuid.UUID) (string, error) {
	record, err := m.catalog.GetLocalObject(ctx, id)
	if err != nil {
		return "", err
	}
	return safeLocalPath(m.root, record.ObjectKey)
}

func (m *Manager) ProjectDirectory(projectID uuid.UUID) (string, error) {
	if projectID == uuid.Nil {
		return "", fmt.Errorf("project id is required")
	}
	return safeLocalPath(m.root, path.Join("projects", projectID.String()))
}

func (m *Manager) RefreshProjectManifest(ctx context.Context, projectID uuid.UUID) error {
	project, err := m.catalog.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	groups, err := m.catalog.ListAssetGroups(ctx, projectID)
	if err != nil {
		return err
	}
	assets, err := m.catalog.ListAssets(ctx, db.AssetFilter{ProjectID: projectID})
	if err != nil {
		return err
	}
	episodes, err := m.catalog.ListEpisodes(ctx, projectID)
	if err != nil {
		return err
	}
	directory, err := m.ProjectDirectory(projectID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	type manifestAsset struct {
		ID           string     `json:"id"`
		Name         string     `json:"name"`
		MediaType    string     `json:"media_type"`
		Status       string     `json:"status"`
		RelativePath string     `json:"relative_path"`
		GroupID      *uuid.UUID `json:"group_id,omitempty"`
		EpisodeID    *uuid.UUID `json:"episode_id,omitempty"`
	}
	manifestAssets := make([]manifestAsset, 0, len(assets))
	for _, asset := range assets {
		record, recordErr := m.catalog.GetLocalObject(ctx, asset.ObjectID)
		if recordErr != nil {
			return recordErr
		}
		relative, relativeErr := filepath.Rel(directory, filepath.Join(m.root, filepath.FromSlash(record.ObjectKey)))
		if relativeErr != nil {
			return relativeErr
		}
		manifestAssets = append(manifestAssets, manifestAsset{ID: asset.ID.String(), Name: asset.Name, MediaType: asset.MediaType, Status: asset.Status, RelativePath: filepath.ToSlash(relative), GroupID: asset.GroupID, EpisodeID: asset.EpisodeID})
	}
	manifest := struct {
		SchemaVersion int             `json:"schema_version"`
		Project       db.Project      `json:"project"`
		Groups        []db.AssetGroup `json:"asset_groups"`
		Episodes      []db.Episode    `json:"episodes"`
		Assets        []manifestAsset `json:"assets"`
	}{SchemaVersion: 1, Project: project, Groups: groups, Episodes: episodes, Assets: manifestAssets}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "project-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err = temporary.Write(payload); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err != nil {
		return err
	}
	target := filepath.Join(directory, "project.json")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryName, target)
}

func (m *Manager) relocateRecord(ctx context.Context, record db.LocalObject, sourceRoot, destinationKey string) error {
	source, err := safeLocalPath(sourceRoot, record.ObjectKey)
	if err != nil {
		return err
	}
	destination, err := safeLocalPath(m.root, destinationKey)
	if err != nil {
		return err
	}
	if source == destination {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	if err := linkOrCopyVerified(source, destination, record.SHA256, record.SizeBytes); err != nil {
		return err
	}
	if _, err := m.catalog.MarkLocalObjectReady(ctx, record.ID, destinationKey, record.SHA256, record.SizeBytes); err != nil {
		return err
	}
	remaining, err := m.catalog.CountReadyLocalObjectsByKey(ctx, record.ObjectKey)
	if err != nil {
		return err
	}
	if remaining == 0 {
		if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func managedObjectKey(projectID *uuid.UUID, purpose string, ownerID uuid.UUID, name string) string {
	scope := "shared"
	if projectID != nil && *projectID != uuid.Nil {
		scope = path.Join("projects", projectID.String())
	}
	return path.Join(scope, purpose, ownerID.String(), safeObjectName(name, ""))
}

func isManagedObjectKey(key string) bool {
	key = filepath.ToSlash(strings.TrimSpace(key))
	return strings.HasPrefix(key, "projects/") || strings.HasPrefix(key, "shared/")
}

func safePurpose(value string) (string, error) {
	value = strings.Trim(filepath.ToSlash(strings.TrimSpace(value)), "/")
	if value == "" {
		return "", fmt.Errorf("storage purpose is required")
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("storage purpose is invalid")
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.Map(safePathRune, segment) != segment {
			return "", fmt.Errorf("storage purpose contains unsupported characters")
		}
	}
	return cleaned, nil
}

func linkOrCopyVerified(source, destination, expectedHash string, expectedSize int64) error {
	if info, err := os.Stat(destination); err == nil {
		if info.Size() != expectedSize {
			return fmt.Errorf("destination file size mismatch")
		}
		return verifyFileHash(destination, expectedHash)
	} else if !os.IsNotExist(err) {
		return err
	}
	created := false
	if err := os.Link(source, destination); err != nil {
		input, openErr := os.Open(source)
		if openErr != nil {
			return openErr
		}
		defer input.Close()
		temporary, createErr := os.CreateTemp(filepath.Dir(destination), ".migrate-*")
		if createErr != nil {
			return createErr
		}
		temporaryName := temporary.Name()
		defer os.Remove(temporaryName)
		if _, copyErr := io.Copy(temporary, input); copyErr != nil {
			_ = temporary.Close()
			return copyErr
		}
		if closeErr := temporary.Close(); closeErr != nil {
			return closeErr
		}
		if renameErr := os.Rename(temporaryName, destination); renameErr != nil {
			return renameErr
		}
		created = true
	} else {
		created = true
	}
	if err := verifyFileHash(destination, expectedHash); err != nil {
		if created {
			_ = os.Remove(destination)
		}
		return err
	}
	return nil
}

func verifyFileHash(filename, expected string) error {
	if strings.TrimSpace(expected) == "" {
		return nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("file checksum mismatch")
	}
	return nil
}

func removeEmptyDirectories(root string) error {
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() {
		return err
	}
	var directories []string
	if err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, current)
		}
		return nil
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		entries, readErr := os.ReadDir(directories[index])
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		if len(entries) == 0 {
			if err := os.Remove(directories[index]); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
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
	remaining, err := m.catalog.CountReadyLocalObjectsByKey(ctx, record.ObjectKey)
	if err != nil {
		_ = m.catalog.MarkLocalObjectFailed(ctx, id)
		return err
	}
	if remaining == 0 {
		filename, pathErr := safeLocalPath(m.root, record.ObjectKey)
		if pathErr != nil {
			_ = m.catalog.MarkLocalObjectFailed(ctx, id)
			return pathErr
		}
		if removeErr := os.Remove(filename); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = m.catalog.MarkLocalObjectFailed(ctx, id)
			return removeErr
		}
	}
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
	name = strings.Map(safePathRune, name)
	if filepath.Ext(name) == "" {
		name += media.ExtensionForMIME(mimeType)
	}
	return name
}

func safePathRune(r rune) rune {
	if r < ' ' || strings.ContainsRune(`<>:"/\\|?*`, r) {
		return '_'
	}
	return r
}
