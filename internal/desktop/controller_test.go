package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/modelcatalog"
)

func TestControllerAttachesWithoutOwningExistingCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":{"status":"ok","version":"v0.1.0","api_version":1,"data_schema_version":1}}`))
	}))
	defer server.Close()

	controller, err := NewController(Options{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.Status != StatusRunning || snapshot.Managed {
		t.Fatalf("expected attached running core, got %+v", snapshot)
	}
	if err := controller.Stop(context.Background()); err == nil {
		t.Fatal("expected controller to refuse stopping an external core")
	}
	if _, err := controller.Backup(context.Background()); err == nil {
		t.Fatal("expected controller to refuse maintaining an external core")
	}
}

func TestControllerRejectsIncompatibleCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":{"status":"ok","version":"v9.0.0","api_version":9,"data_schema_version":1}}`))
	}))
	defer server.Close()

	controller, err := NewController(Options{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(context.Background()); err == nil {
		t.Fatal("expected incompatible API version to be rejected")
	}
}

func TestResolveCorePathUsesExplicitBinary(t *testing.T) {
	name := "sagaflow"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveCorePath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := filepath.Abs(path)
	if resolved != expected {
		t.Fatalf("expected %s, got %s", expected, resolved)
	}
}

func TestEnvironmentOverridesExistingValueOnce(t *testing.T) {
	t.Setenv("SAGAFLOW_PORT", "9999")
	environment := environmentWith(map[string]string{"SAGAFLOW_PORT": "18848"})
	count := 0
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), "SAGAFLOW_PORT=") {
			count++
			if entry != "SAGAFLOW_PORT=18848" {
				t.Fatalf("unexpected port entry %q", entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one port entry, got %d", count)
	}
}

func TestConciseFFmpegVersion(t *testing.T) {
	tests := map[string]string{
		"ffmpeg version 8.1.2-1-gcc3c09c101 Copyright": "8.1.2-1-gcc3c09c101",
		"FFmpeg VERSION 7.1":                           "7.1",
		"unexpected output":                            "",
	}
	for input, expected := range tests {
		if actual := conciseFFmpegVersion(input); actual != expected {
			t.Errorf("conciseFFmpegVersion(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestControllerChecksAndAppliesModelCatalogUpdate(t *testing.T) {
	current, err := modelcatalog.EmbeddedManifest()
	if err != nil {
		t.Fatal(err)
	}
	remote := current
	remote.Models = append([]modelcatalog.ManifestModel(nil), current.Models...)
	remote.Models[0].DisplayName += " Updated"
	remote.CatalogVersion = current.CatalogVersion + ".1"
	published, err := time.Parse(time.RFC3339, current.PublishedAt)
	if err != nil {
		t.Fatal(err)
	}
	remote.PublishedAt = published.Add(24 * time.Hour).Format(time.RFC3339)
	payload, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(payload)
	}))
	defer server.Close()

	dataDir := t.TempDir()
	t.Setenv("SAGAFLOW_DATA_DIR", dataDir)
	controller, err := NewController(Options{CatalogURL: server.URL, CatalogHTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	check, err := controller.CheckModelCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !check.UpdateAvailable || check.Updated != 1 || check.RemoteVersion != remote.CatalogVersion {
		t.Fatalf("unexpected catalog update check: %+v", check)
	}
	info, err := controller.ApplyModelCatalog(check)
	if err != nil {
		t.Fatal(err)
	}
	if info.CatalogVersion != remote.CatalogVersion {
		t.Fatalf("unexpected installed catalog: %+v", info)
	}
}

func TestControllerStartsAndGracefullyStopsRealCore(t *testing.T) {
	corePath := os.Getenv("SAGAFLOW_INTEGRATION_CORE")
	if corePath == "" {
		t.Skip("set SAGAFLOW_INTEGRATION_CORE to run the process integration test")
	}
	tempDir := t.TempDir()
	t.Setenv("SAGAFLOW_DATA_DIR", filepath.Join(tempDir, "data"))
	controller, err := NewController(Options{
		BaseURL: DefaultURL, CorePath: corePath,
		RuntimeFile: filepath.Join(tempDir, "runtime.json"),
		LogFile:     filepath.Join(tempDir, "core.log"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = controller.Stop(ctx)
	})

	startContext, cancelStart := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStart()
	if err := controller.Start(startContext); err != nil {
		t.Fatal(err)
	}
	snapshot := controller.Snapshot()
	if snapshot.Status != StatusRunning || !snapshot.Managed || snapshot.PID == 0 {
		t.Fatalf("expected managed running core, got %+v", snapshot)
	}
	doctorContext, cancelDoctor := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelDoctor()
	doctorResult, err := controller.Doctor(doctorContext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doctorResult, "工作台数据：正常") {
		t.Fatalf("expected user-facing diagnosis, got %q", doctorResult)
	}
	if strings.Contains(strings.ToLower(doctorResult), "goose") || strings.Contains(doctorResult, `{"`) {
		t.Fatalf("diagnosis leaked implementation output: %q", doctorResult)
	}
	backupContext, cancelBackup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelBackup()
	backupResult, err := controller.Backup(backupContext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(backupResult, "备份已完成。") {
		t.Fatalf("expected user-facing backup result, got %q", backupResult)
	}
	backups, err := filepath.Glob(filepath.Join(tempDir, "data", "backups", "*.zip"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one backup archive, got %v (%v)", backups, err)
	}

	stopContext, cancelStop := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStop()
	if err := controller.Stop(stopContext); err != nil {
		t.Fatal(err)
	}
	if snapshot = controller.Snapshot(); snapshot.Status != StatusStopped {
		t.Fatalf("expected stopped core, got %+v", snapshot)
	}
}
