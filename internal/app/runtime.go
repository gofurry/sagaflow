package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofurry/sagaflow/internal/runtimecontract"
)

// RunOptions contains the optional process-control contract used by the
// desktop launcher. Headless and Docker launches leave it empty.
type RunOptions struct {
	RuntimeFile         string
	DesktopControlToken string
}

func writeRuntimeFile(path string, info runtimecontract.RuntimeInfo) error {
	if path == "" {
		return nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve runtime file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime info: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(absolute), ".sagaflow-runtime-*")
	if err != nil {
		return fmt.Errorf("create runtime file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("protect runtime file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write runtime file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close runtime file: %w", err)
	}
	if err := os.Remove(absolute); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace runtime file: %w", err)
	}
	if err := os.Rename(tempPath, absolute); err != nil {
		return fmt.Errorf("publish runtime file: %w", err)
	}
	return nil
}

func removeRuntimeFile(path string, pid int) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var info runtimecontract.RuntimeInfo
	if json.Unmarshal(data, &info) == nil && info.PID == pid {
		_ = os.Remove(path)
	}
}
