package backup

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
)

const databaseName = "sagaflow.db"

// Create writes a consistent, self-contained personal workspace archive. The
// credentials master key is included so encrypted provider and S3 secrets stay
// usable after restore; callers must protect the resulting archive accordingly.
func Create(ctx context.Context, cfg config.Config, output string) (string, error) {
	if err := cfg.EnsureRuntimeDirs(); err != nil {
		return "", err
	}
	if strings.TrimSpace(output) == "" {
		output = filepath.Join(cfg.BackupDir(), "sagaflow-"+time.Now().Format("20060102-150405")+".zip")
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return "", err
	}
	snapshot, err := os.CreateTemp(cfg.TempDir(), "backup-*.db")
	if err != nil {
		return "", err
	}
	snapshotPath := snapshot.Name()
	if err := snapshot.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(snapshotPath)
	defer os.Remove(snapshotPath)
	database, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return "", err
	}
	quoted := strings.ReplaceAll(filepath.ToSlash(snapshotPath), "'", "''")
	_, snapshotErr := database.ExecContext(ctx, "VACUUM INTO '"+quoted+"'")
	closeErr := database.Close()
	if snapshotErr != nil {
		return "", fmt.Errorf("create SQLite snapshot: %w", snapshotErr)
	}
	if closeErr != nil {
		return "", closeErr
	}

	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	archive := zip.NewWriter(file)
	writeErr := addFile(archive, snapshotPath, databaseName)
	for _, item := range []struct{ path, name string }{
		{filepath.Join(cfg.App.DataDir, "projects"), "projects"},
		{filepath.Join(cfg.App.DataDir, "shared"), "shared"},
		// Keep legacy archives recoverable while pre-v0.1.0 workspaces migrate.
		{cfg.ObjectDir(), "objects"},
		{cfg.SecretDir(), "secrets"},
	} {
		if writeErr == nil {
			writeErr = addTree(archive, item.path, item.name)
		}
	}
	zipErr := archive.Close()
	fileErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(absolute)
		return "", writeErr
	}
	if zipErr != nil {
		_ = os.Remove(absolute)
		return "", zipErr
	}
	if fileErr != nil {
		return "", fileErr
	}
	return absolute, nil
}

// Restore validates an archive in a sibling staging directory, then swaps the
// entire data directory. The previous directory remains beside it for manual
// rollback until the user chooses to remove it.
func Restore(ctx context.Context, cfg config.Config, archivePath string) (string, error) {
	absoluteArchive, err := filepath.Abs(archivePath)
	if err != nil {
		return "", err
	}
	reader, err := zip.OpenReader(absoluteArchive)
	if err != nil {
		return "", fmt.Errorf("open backup: %w", err)
	}
	defer reader.Close()
	dataDir, err := filepath.Abs(cfg.App.DataDir)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(dataDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(parent, ".sagaflow-restore-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	for _, entry := range reader.File {
		if err := extractEntry(entry, staging); err != nil {
			return "", err
		}
	}
	if err := reader.Close(); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(staging, databaseName)); err != nil {
		return "", fmt.Errorf("backup does not contain %s", databaseName)
	}
	check, err := sqlite.Open(ctx, filepath.Join(staging, databaseName))
	if err != nil {
		return "", fmt.Errorf("validate restored database: %w", err)
	}
	var ok string
	if err := check.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&ok); err != nil {
		_ = check.Close()
		return "", fmt.Errorf("run restored database integrity check: %w", err)
	}
	if ok != "ok" {
		_ = check.Close()
		return "", fmt.Errorf("restored database integrity check failed: %s", ok)
	}
	if err := check.Close(); err != nil {
		return "", err
	}
	previous := dataDir + ".before-restore-" + time.Now().Format("20060102-150405")
	if _, err := os.Stat(dataDir); err == nil {
		if err := os.Rename(dataDir, previous); err != nil {
			return "", fmt.Errorf("move current data directory (is SagaFlow still running?): %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	} else {
		previous = ""
	}
	if err := os.Rename(staging, dataDir); err != nil {
		if previous != "" {
			_ = os.Rename(previous, dataDir)
		}
		return "", fmt.Errorf("activate restored data: %w", err)
	}
	return previous, nil
}

func addTree(archive *zip.Writer, root, archiveRoot string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to back up symlink %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return addFile(archive, path, filepath.ToSlash(filepath.Join(archiveRoot, relative)))
	})
}

func addFile(archive *zip.Writer, path, name string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(name)
	header.Method = zip.Deflate
	output, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return err
}

func extractEntry(entry *zip.File, root string) error {
	name := filepath.Clean(filepath.FromSlash(entry.Name))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("backup contains unsafe path %q", entry.Name)
	}
	target := filepath.Join(root, name)
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o700)
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("backup contains unsupported symlink %q", entry.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
