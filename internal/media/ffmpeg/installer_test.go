package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ulikunitz/xz"
)

func TestReleaseManifestCoversBuildMatrix(t *testing.T) {
	for _, platform := range [][2]string{
		{"windows", "amd64"}, {"windows", "arm64"},
		{"linux", "amd64"}, {"linux", "arm64"},
		{"darwin", "amd64"}, {"darwin", "arm64"},
	} {
		release, ok := releaseFor(platform[0], platform[1])
		if !ok {
			t.Fatalf("missing release for %s/%s", platform[0], platform[1])
		}
		if release.BundleID == "" || release.Version != "8.1.2" || release.SourceURL == "" || release.DownloadBytes <= 0 {
			t.Fatalf("incomplete release for %s/%s: %#v", platform[0], platform[1], release)
		}
		var total int64
		for _, asset := range release.Assets {
			total += asset.Size
			if !strings.HasPrefix(asset.URL, "https://") || len(asset.SHA256) != sha256.Size*2 {
				t.Fatalf("invalid asset for %s/%s: %#v", platform[0], platform[1], asset)
			}
			if _, err := hex.DecodeString(asset.SHA256); err != nil {
				t.Fatalf("invalid SHA-256 for %s/%s: %v", platform[0], platform[1], err)
			}
		}
		if total != release.DownloadBytes {
			t.Fatalf("download size mismatch for %s/%s: got %d want %d", platform[0], platform[1], release.DownloadBytes, total)
		}
	}
}

func TestDownloadVerifiesSizeAndSHA256(t *testing.T) {
	payload := []byte("fixed ffmpeg test payload")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	tool := &Toolchain{httpClient: server.Client()}
	asset := releaseAsset{Name: "ffmpeg", URL: server.URL, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload))}
	destination := filepath.Join(t.TempDir(), "download")
	if err := tool.download(context.Background(), server.Client(), asset, destination, 0); err != nil {
		t.Fatalf("download: %v", err)
	}
	stored, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read download: %v", err)
	}
	if !bytes.Equal(stored, payload) {
		t.Fatalf("unexpected payload %q", stored)
	}

	bad := asset
	bad.SHA256 = strings.Repeat("0", sha256.Size*2)
	if err := tool.download(context.Background(), server.Client(), bad, filepath.Join(t.TempDir(), "bad"), 0); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected SHA-256 error, got %v", err)
	}
}

func TestStartInstallReportsProgressAndCanBeCanceled(t *testing.T) {
	const size = int64(1 << 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		_, _ = w.Write(make([]byte, 32<<10))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-request.Context().Done()
	}))
	defer server.Close()

	tool := &Toolchain{
		managedRoot: t.TempDir(),
		httpClient:  server.Client(),
		release: platformRelease{
			BundleID: "test-cancel", Version: "8.1.2", Format: "zip", DownloadBytes: size,
			Assets: []releaseAsset{{Name: "ffmpeg", URL: server.URL, SHA256: strings.Repeat("0", sha256.Size*2), Size: size}},
		},
	}
	tool.updatePaths("", "", Status{Message: "missing"})
	status, err := tool.StartInstall(InstallOptions{Mode: "direct"})
	if err != nil || !status.Installing || !status.CanCancel {
		t.Fatalf("start install: status=%#v err=%v", status, err)
	}
	waitForStatus(t, tool, func(status Status) bool { return status.DownloadedBytes > 0 })
	tool.CancelInstall()
	status = waitForStatus(t, tool, func(status Status) bool { return !status.Installing })
	if status.InstallStage != "canceled" || status.CanCancel {
		t.Fatalf("unexpected canceled status: %#v", status)
	}
}

func TestProxyInstallClientUsesLocalPortAndOptionalCredentials(t *testing.T) {
	requested := make(chan *http.Request, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requested <- request.Clone(context.Background())
		_, _ = w.Write([]byte("proxied"))
	}))
	defer proxy.Close()
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	client, err := (&Toolchain{}).installHTTPClient(InstallOptions{
		Mode: "proxy", ProxyPort: port, ProxyUsername: "creator", ProxyPassword: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("http://example.invalid/ffmpeg.zip")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	request := <-requested
	if request.URL.Host != "example.invalid" || request.Header.Get("Proxy-Authorization") == "" {
		t.Fatalf("request did not traverse authenticated proxy: url=%s headers=%v", request.URL, request.Header)
	}
}

func waitForStatus(t *testing.T, tool *Toolchain, ready func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := tool.Status()
		if ready(status) {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for install status: %#v", tool.Status())
	return Status{}
}

func TestUnpackZIPExecutable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "ffmpeg.zip")
	writeTestZIP(t, source, map[string]string{"nested/ffmpeg": "binary"})
	destination := filepath.Join(root, "output")
	if err := unpackZIPExecutable(source, destination, "ffmpeg"); err != nil {
		t.Fatalf("unpack zip executable: %v", err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "binary" {
		t.Fatalf("unexpected extracted content %q: %v", content, err)
	}
}

func TestUnpackZIPPair(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "ffmpeg.zip")
	ffmpegName, ffprobeName := executableNames()
	writeTestZIP(t, source, map[string]string{
		"bundle/bin/" + ffmpegName:  "ffmpeg",
		"bundle/bin/" + ffprobeName: "ffprobe",
		"bundle/bin/ffplay":         "ignored",
	})
	destination := filepath.Join(root, "output")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatalf("create destination: %v", err)
	}
	if err := unpackZIPPair(source, destination); err != nil {
		t.Fatalf("unpack zip: %v", err)
	}
	for name, expected := range map[string]string{ffmpegName: "ffmpeg", ffprobeName: "ffprobe"} {
		content, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil || string(content) != expected {
			t.Fatalf("unexpected %s content %q: %v", name, content, readErr)
		}
	}
}

func TestUnpackTarXZPair(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "ffmpeg.tar.xz")
	file, err := os.Create(source)
	if err != nil {
		t.Fatalf("create tar.xz: %v", err)
	}
	xzWriter, err := xz.NewWriter(file)
	if err != nil {
		t.Fatalf("create xz writer: %v", err)
	}
	tarWriter := tar.NewWriter(xzWriter)
	ffmpegName, ffprobeName := executableNames()
	for name, content := range map[string]string{
		"bundle/bin/" + ffmpegName:  "ffmpeg",
		"bundle/bin/" + ffprobeName: "ffprobe",
		"bundle/bin/ffplay":         "ignored",
	} {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := io.WriteString(tarWriter, content); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := xzWriter.Close(); err != nil {
		t.Fatalf("close xz writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close tar.xz file: %v", err)
	}
	destination := filepath.Join(root, "output")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatalf("create destination: %v", err)
	}
	if err := unpackTarXZPair(source, destination); err != nil {
		t.Fatalf("unpack tar.xz: %v", err)
	}
	for name, expected := range map[string]string{ffmpegName: "ffmpeg", ffprobeName: "ffprobe"} {
		content, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil || string(content) != expected {
			t.Fatalf("unexpected %s content %q: %v", name, content, readErr)
		}
	}
}

func writeTestZIP(t *testing.T, source string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(source)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, createErr := writer.Create(name)
		if createErr != nil {
			t.Fatalf("create zip entry: %v", createErr)
		}
		if _, writeErr := io.WriteString(entry, content); writeErr != nil {
			t.Fatalf("write zip entry: %v", writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}
