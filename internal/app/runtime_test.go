package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/runtimecontract"
)

func TestRuntimeFileLifecycleIsOwnedByPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "sagaflow.json")
	cfg := config.Default()
	cfg.App.Version = "v0.1.0-test"
	info := runtimecontract.NewInfo(cfg)
	if err := writeRuntimeFile(path, info); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded runtimecontract.RuntimeInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PID != info.PID || decoded.URL != "http://127.0.0.1:18848" {
		t.Fatalf("unexpected runtime info: %+v", decoded)
	}

	removeRuntimeFile(path, info.PID+1)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("another process must not remove the runtime file: %v", err)
	}
	removeRuntimeFile(path, info.PID)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected owned runtime file to be removed, got %v", err)
	}
}
