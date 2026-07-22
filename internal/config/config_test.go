package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaultsAndRuntimeEnv(t *testing.T) {
	t.Setenv("SAGAFLOW_DATA_DIR", t.TempDir())
	t.Setenv("SAGAFLOW_PORT", "9090")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  name: sagaflow-test\nserver:\n  host: 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("expected env port override, got %d", cfg.Server.Port)
	}
	if cfg.DatabasePath() != filepath.Join(cfg.App.DataDir, "sagaflow.db") {
		t.Fatalf("unexpected database path: %s", cfg.DatabasePath())
	}
}

func TestWriteDefaultDoesNotOverwriteWithoutForce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := WriteDefault(path, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteDefault(path, false); err == nil {
		t.Fatal("expected second write without force to fail")
	}
	if err := WriteDefault(path, true); err != nil {
		t.Fatalf("expected forced write to succeed: %v", err)
	}
}

func TestLoadRejectsProviderAndStorageSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("storage:\n  backend: s3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected legacy storage config to be rejected")
	}
}

func TestPortableDataDirUsesReleasedExecutableDirectory(t *testing.T) {
	executable := filepath.Join("opt", "sagaflow", "sagaflow")
	want := filepath.Join("opt", "sagaflow", "data")
	if got := portableDataDir(executable, filepath.Join("work", "repo"), filepath.Join("tmp")); got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestPortableDataDirUsesWorkingDirectoryForGoRun(t *testing.T) {
	tempDir := filepath.Join("tmp")
	executable := filepath.Join(tempDir, "go-build123", "b001", "exe", "sagaflow")
	cwd := filepath.Join("work", "repo")
	want := filepath.Join(cwd, "data")
	if got := portableDataDir(executable, cwd, tempDir); got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
