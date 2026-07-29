package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoreLogWriterHasBoundedRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sagaflow-core.log")
	writer := newCoreLogWriter(path)
	if writer.Filename != path {
		t.Fatalf("unexpected log path %q", writer.Filename)
	}
	if writer.MaxSize != 10 || writer.MaxBackups != 5 || writer.MaxAge != 14 {
		t.Fatalf("unexpected retention settings: size=%d backups=%d age=%d", writer.MaxSize, writer.MaxBackups, writer.MaxAge)
	}
	if !writer.Compress {
		t.Fatal("expected rotated logs to be compressed")
	}
}

func TestClearOldLogsPreservesActiveAndUnrelatedFiles(t *testing.T) {
	directory := t.TempDir()
	active := filepath.Join(directory, "sagaflow-core.log")
	files := map[string]string{
		"sagaflow-core.log":                          "active",
		"sagaflow-core-2026-07-25T120000.000.log":    "old",
		"sagaflow-core-2026-07-24T120000.000.log.gz": "compressed",
		"other.log": "unrelated",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	controller := &Controller{logFile: active}
	result, err := controller.ClearOldLogs()
	if err != nil {
		t.Fatal(err)
	}
	if result.Files != 2 {
		t.Fatalf("expected two historical logs removed, got %+v", result)
	}
	for _, name := range []string{"sagaflow-core.log", "other.log"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("expected %s to remain: %v", name, err)
		}
	}
}
