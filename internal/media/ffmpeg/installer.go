package ffmpeg

import (
	"archive/tar"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ulikunitz/xz"
)

const managedBundleRevision = "2026.07"

var ErrInstallUnsupported = errors.New("FFmpeg managed installation is unsupported on this platform")

type InstallOptions struct {
	Mode          string
	ProxyPort     int
	ProxyUsername string
	ProxyPassword string
}

type releaseAsset struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64
}

type platformRelease struct {
	BundleID      string
	Version       string
	Format        string
	SourceURL     string
	DownloadBytes int64
	Assets        []releaseAsset
}

func releaseFor(goos, goarch string) (platformRelease, bool) {
	key := goos + "/" + goarch
	const (
		btbnBase = "https://github.com/BtbN/FFmpeg-Builds/releases/download/autobuild-2026-06-30-13-34/"
		btbnPage = "https://github.com/BtbN/FFmpeg-Builds/releases/tag/autobuild-2026-06-30-13-34"
	)
	releases := map[string]platformRelease{
		"windows/amd64": archiveRelease(key, "zip", btbnPage,
			releaseAsset{
				Name: "ffmpeg", URL: btbnBase + "ffmpeg-n8.1.2-21-gce3c09c101-win64-gpl-8.1.zip",
				SHA256: "682361e32c9631caec09e5d9f09077101c9ed90c14e275f62014fefa6d397990", Size: 166_372_072,
			}),
		"windows/arm64": {
			BundleID:  bundleID(key, "8.1.2"),
			Version:   "8.1.2",
			Format:    "zip",
			SourceURL: btbnPage,
			Assets: []releaseAsset{{
				Name:   "ffmpeg",
				URL:    btbnBase + "ffmpeg-n8.1.2-21-gce3c09c101-winarm64-gpl-8.1.zip",
				SHA256: "e4a86d390c1ab524acf73be651915be34bb7f1151540ab22c9774184aab18fdf",
				Size:   110_148_073,
			}},
			DownloadBytes: 110_148_073,
		},
		"linux/amd64": archiveRelease(key, "tar-xz", btbnPage,
			releaseAsset{
				Name: "ffmpeg", URL: btbnBase + "ffmpeg-n8.1.2-21-gce3c09c101-linux64-gpl-8.1.tar.xz",
				SHA256: "0ba73bbd93472c7622f6dec26d334c5e62e64d858d072490b2844320970456cd", Size: 124_756_048,
			}),
		"linux/arm64": archiveRelease(key, "tar-xz", btbnPage,
			releaseAsset{
				Name: "ffmpeg", URL: btbnBase + "ffmpeg-n8.1.2-21-gce3c09c101-linuxarm64-gpl-8.1.tar.xz",
				SHA256: "d3f90a71a38238466de2e4dc98537862d244e3307383435f94cbc4b8491033f8", Size: 106_131_164,
			}),
		"darwin/amd64": archiveRelease(key, "zip-pair", "https://evermeet.cx/ffmpeg/",
			releaseAsset{
				Name: "ffmpeg", URL: "https://evermeet.cx/ffmpeg/ffmpeg-8.1.2.zip",
				SHA256: "e91df72a1ee7c26606f90dd2dd4dcccc6a75140ff9ea6fdd50faae828b82ba69", Size: 26_037_786,
			},
			releaseAsset{
				Name: "ffprobe", URL: "https://evermeet.cx/ffmpeg/ffprobe-8.1.2.zip",
				SHA256: "399b93f0b9862f69767afa343e90c2f48d7e7958cadbb6deb76a012d0e3b7ce3", Size: 25_941_651,
			}),
		"darwin/arm64": archiveRelease(key, "zip-pair", "https://ffmpeg.martin-riedl.de/info/detail/macos/arm64/1783011502_8.1.2",
			releaseAsset{
				Name: "ffmpeg", URL: "https://ffmpeg.martin-riedl.de/download/macos/arm64/1783011502_8.1.2/ffmpeg.zip",
				SHA256: "ef1aa60006c7b77ce170c1608c08d8e4ba1c30c5746f2ac986ded932d0ac2c3c", Size: 28_196_358,
			},
			releaseAsset{
				Name: "ffprobe", URL: "https://ffmpeg.martin-riedl.de/download/macos/arm64/1783011502_8.1.2/ffprobe.zip",
				SHA256: "c39787f4af7a3932502d2d48db6f6feaaa836b48a73ef78c32cc3285df61dfaf", Size: 28_118_222,
			}),
	}
	release, ok := releases[key]
	return release, ok
}

func archiveRelease(key, format, sourceURL string, assets ...releaseAsset) platformRelease {
	var total int64
	for _, asset := range assets {
		total += asset.Size
	}
	return platformRelease{
		BundleID: bundleID(key, "8.1.2"), Version: "8.1.2", Format: format,
		SourceURL: sourceURL, DownloadBytes: total, Assets: assets,
	}
}

func bundleID(platform, version string) string {
	return "managed-" + managedBundleRevision + "-" + strings.ReplaceAll(platform, "/", "-") + "-" + version
}

func (t *Toolchain) StartInstall(options InstallOptions) (Status, error) {
	if t == nil {
		return Status{}, errors.New("FFmpeg service is not initialized")
	}
	t.installMu.Lock()
	defer t.installMu.Unlock()

	status := t.Status()
	if status.Available || status.Installing {
		return status, nil
	}
	if t.managedRoot == "" || t.release.BundleID == "" {
		return status, ErrInstallUnsupported
	}
	options, err := normalizeInstallOptions(options)
	if err != nil {
		return status, err
	}
	client, err := t.installHTTPClient(options)
	if err != nil {
		return status, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.installCancel = cancel
	t.setInstallState(true, true, "connecting", fmt.Sprintf("正在连接 FFmpeg %s 下载源", t.release.Version), 0)
	t.setInstallMode(options.Mode)
	go t.runInstall(ctx, client)
	return t.Status(), nil
}

func normalizeInstallOptions(options InstallOptions) (InstallOptions, error) {
	options.Mode = strings.ToLower(strings.TrimSpace(options.Mode))
	if options.Mode == "" {
		options.Mode = "direct"
	}
	switch options.Mode {
	case "direct":
		options.ProxyPort = 0
		options.ProxyUsername = ""
		options.ProxyPassword = ""
	case "proxy":
		if options.ProxyPort < 1 || options.ProxyPort > 65535 {
			return InstallOptions{}, errors.New("proxy port must be between 1 and 65535")
		}
		options.ProxyUsername = strings.TrimSpace(options.ProxyUsername)
		if options.ProxyUsername == "" && options.ProxyPassword != "" {
			return InstallOptions{}, errors.New("proxy username is required when a password is provided")
		}
	default:
		return InstallOptions{}, errors.New("install mode must be direct or proxy")
	}
	return options, nil
}

func (t *Toolchain) installHTTPClient(options InstallOptions) (*http.Client, error) {
	if t.httpClient != nil {
		return t.httpClient, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.IdleConnTimeout = 90 * time.Second
	if options.Mode == "proxy" {
		proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", options.ProxyPort))}
		if options.ProxyUsername != "" {
			if options.ProxyPassword == "" {
				proxyURL.User = url.User(options.ProxyUsername)
			} else {
				proxyURL.User = url.UserPassword(options.ProxyUsername, options.ProxyPassword)
			}
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Minute}, nil
}

func (t *Toolchain) CancelInstall() Status {
	if t == nil {
		return Status{Message: "FFmpeg 服务未初始化"}
	}
	t.installMu.Lock()
	defer t.installMu.Unlock()
	if t.installCancel == nil || !t.Status().Installing {
		return t.Status()
	}
	t.setInstallState(true, false, "canceling", "正在取消 FFmpeg 下载…", t.Status().DownloadedBytes)
	t.installCancel()
	return t.Status()
}

func (t *Toolchain) runInstall(ctx context.Context, client *http.Client) {
	if err := t.install(ctx, client); err != nil {
		if errors.Is(err, context.Canceled) {
			t.setInstallState(false, false, "canceled", "FFmpeg 下载已取消", 0)
		} else {
			t.setInstallState(false, false, "failed", "FFmpeg 安装失败："+err.Error(), 0)
		}
		t.clearInstallCancel()
		return
	}
	installMode := t.Status().InstallMode
	t.refresh()
	t.setInstallMode(installMode)
	status := t.Status()
	if !status.Available {
		t.setInstallState(false, false, "failed", "FFmpeg 安装失败：安装完成但可执行文件不可用", 0)
		t.clearInstallCancel()
		return
	}
	t.clearInstallCancel()
}

func (t *Toolchain) clearInstallCancel() {
	t.installMu.Lock()
	t.installCancel = nil
	t.installMu.Unlock()
}

func (t *Toolchain) setInstallState(installing, canCancel bool, stage, message string, downloaded int64) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	t.status.Installing = installing
	t.status.CanCancel = canCancel
	t.status.InstallStage = stage
	t.status.Message = message
	t.status.DownloadedBytes = downloaded
	if !installing {
		t.status.DownloadSpeed = 0
		t.status.ETASeconds = 0
	}
	if t.status.DownloadBytes > 0 {
		t.status.InstallProgress = min(float64(downloaded)/float64(t.status.DownloadBytes), 1)
	} else {
		t.status.InstallProgress = 0
	}
}

func (t *Toolchain) setInstallMode(mode string) {
	t.stateMu.Lock()
	t.status.InstallMode = mode
	t.stateMu.Unlock()
}

func (t *Toolchain) setDownloadProgress(downloaded, speed int64) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if !t.status.Installing || (t.status.InstallStage != "downloading" && t.status.InstallStage != "retrying") {
		return
	}
	if downloaded < t.status.DownloadedBytes {
		return
	}
	t.status.DownloadedBytes = min(downloaded, t.status.DownloadBytes)
	t.status.DownloadSpeed = max(speed, 0)
	if t.status.DownloadBytes > 0 {
		t.status.InstallProgress = min(float64(t.status.DownloadedBytes)/float64(t.status.DownloadBytes), 1)
	}
	if t.status.DownloadSpeed > 0 && t.status.DownloadBytes > t.status.DownloadedBytes {
		t.status.ETASeconds = (t.status.DownloadBytes - t.status.DownloadedBytes) / t.status.DownloadSpeed
	} else {
		t.status.ETASeconds = 0
	}
}

func (t *Toolchain) install(ctx context.Context, client *http.Client) error {
	if err := os.MkdirAll(t.managedRoot, 0o755); err != nil {
		return fmt.Errorf("create managed tools directory: %w", err)
	}
	target := filepath.Join(t.managedRoot, t.release.BundleID)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("安装目录已存在但不完整：%s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect install directory: %w", err)
	}
	staging, err := os.MkdirTemp(t.managedRoot, ".ffmpeg-install-")
	if err != nil {
		return fmt.Errorf("create installation staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	switch t.release.Format {
	case "zip":
		err = t.installZIP(ctx, client, staging)
	case "zip-pair":
		err = t.installZIPPair(ctx, client, staging)
	case "tar-xz":
		err = t.installTarXZ(ctx, client, staging)
	default:
		err = fmt.Errorf("unsupported FFmpeg package format %q", t.release.Format)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t.setInstallState(true, false, "verifying", "正在校验 FFmpeg 可执行文件…", t.release.DownloadBytes)
	ffmpegName, ffprobeName := executableNames()
	ffmpegPath := filepath.Join(staging, ffmpegName)
	ffprobePath := filepath.Join(staging, ffprobeName)
	status := inspectToolchain(ffmpegPath, ffprobePath, "managed")
	if !status.Available {
		return errors.New(status.Message)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	notice := fmt.Sprintf("SagaFlow managed FFmpeg download\nVersion: %s\nPlatform: %s/%s\nSource: %s\nFFmpeg license information: https://ffmpeg.org/legal.html\n",
		t.release.Version, runtime.GOOS, runtime.GOARCH, t.release.SourceURL)
	if err := os.WriteFile(filepath.Join(staging, "THIRD-PARTY-NOTICE.txt"), []byte(notice), 0o644); err != nil {
		return fmt.Errorf("write third-party notice: %w", err)
	}
	if err := os.Rename(staging, target); err != nil {
		return fmt.Errorf("activate FFmpeg installation: %w", err)
	}
	return nil
}

func (t *Toolchain) installZIP(ctx context.Context, client *http.Client, staging string) error {
	if len(t.release.Assets) != 1 {
		return errors.New("invalid FFmpeg zip manifest")
	}
	archivePath := filepath.Join(staging, "ffmpeg.zip")
	if err := t.download(ctx, client, t.release.Assets[0], archivePath, 0); err != nil {
		return err
	}
	t.setInstallState(true, true, "extracting", "正在解压 FFmpeg…", t.release.DownloadBytes)
	if err := unpackZIPPair(archivePath, staging); err != nil {
		return err
	}
	if err := os.Remove(archivePath); err != nil {
		return fmt.Errorf("remove downloaded archive: %w", err)
	}
	return nil
}

func (t *Toolchain) installZIPPair(ctx context.Context, client *http.Client, staging string) error {
	if len(t.release.Assets) != 2 {
		return errors.New("invalid FFmpeg zip-pair manifest")
	}
	ffmpegName, ffprobeName := executableNames()
	destinations := map[string]string{"ffmpeg": ffmpegName, "ffprobe": ffprobeName}
	var completed int64
	for _, asset := range t.release.Assets {
		t.setInstallState(true, true, "downloading", fmt.Sprintf("正在下载 %s…", asset.Name), completed)
		destination, ok := destinations[asset.Name]
		if !ok {
			return fmt.Errorf("unexpected FFmpeg asset %q", asset.Name)
		}
		archivePath := filepath.Join(staging, asset.Name+".zip")
		if err := t.download(ctx, client, asset, archivePath, completed); err != nil {
			return err
		}
		completed += asset.Size
		t.setInstallState(true, true, "extracting", fmt.Sprintf("正在解压 %s…", asset.Name), completed)
		if err := unpackZIPExecutable(archivePath, filepath.Join(staging, destination), destination); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(archivePath); err != nil {
			return fmt.Errorf("remove downloaded archive: %w", err)
		}
	}
	return nil
}

func (t *Toolchain) installTarXZ(ctx context.Context, client *http.Client, staging string) error {
	if len(t.release.Assets) != 1 {
		return errors.New("invalid FFmpeg tar.xz manifest")
	}
	archivePath := filepath.Join(staging, "ffmpeg.tar.xz")
	if err := t.download(ctx, client, t.release.Assets[0], archivePath, 0); err != nil {
		return err
	}
	t.setInstallState(true, true, "extracting", "正在解压 FFmpeg…", t.release.DownloadBytes)
	if err := unpackTarXZPair(archivePath, staging); err != nil {
		return err
	}
	if err := os.Remove(archivePath); err != nil {
		return fmt.Errorf("remove downloaded archive: %w", err)
	}
	return nil
}

func (t *Toolchain) download(ctx context.Context, client *http.Client, asset releaseAsset, destination string, completed int64) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			offset := existingFileSize(destination)
			t.setInstallState(true, true, "retrying", fmt.Sprintf("下载中断，正在进行第 %d 次重试…", attempt), completed+offset)
			if err := waitForRetry(ctx, time.Duration(attempt)*time.Second); err != nil {
				return err
			}
		}
		if err := t.downloadAttempt(ctx, client, asset, destination, completed); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return fmt.Errorf("download %s after 3 attempts: %w", asset.Name, lastErr)
	}
	info, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("inspect %s download: %w", asset.Name, err)
	}
	if info.Size() != asset.Size {
		return fmt.Errorf("download %s: size mismatch (received %d, expected %d)", asset.Name, info.Size(), asset.Size)
	}
	file, err := os.Open(destination)
	if err != nil {
		return fmt.Errorf("open %s download for verification: %w", asset.Name, err)
	}
	hasher := sha256.New()
	_, hashErr := io.Copy(hasher, file)
	closeErr := file.Close()
	if hashErr != nil {
		return fmt.Errorf("hash %s download: %w", asset.Name, hashErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s download: %w", asset.Name, closeErr)
	}
	digest := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(digest, asset.SHA256) {
		return fmt.Errorf("download %s: SHA-256 mismatch", asset.Name)
	}
	t.setDownloadProgress(completed+asset.Size, 0)
	return nil
}

func (t *Toolchain) downloadAttempt(ctx context.Context, client *http.Client, asset releaseAsset, destination string, completed int64) error {
	offset := existingFileSize(destination)
	if offset < 0 || offset > asset.Size {
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reset invalid partial download: %w", err)
		}
		offset = 0
	}
	if offset == asset.Size {
		return nil
	}
	t.setInstallState(true, true, "connecting", "正在连接下载源…", completed+offset)
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return fmt.Errorf("create FFmpeg download request: %w", err)
	}
	request.Header.Set("User-Agent", "SagaFlow/v0.1.0")
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if offset > 0 && response.StatusCode == http.StatusOK {
		offset = 0
	} else if offset > 0 && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("resume returned HTTP status %s", response.Status)
	} else if offset == 0 && response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	if response.ContentLength > asset.Size-offset {
		return errors.New("response exceeds the fixed manifest size")
	}
	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	file, err := os.OpenFile(destination, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open FFmpeg download file: %w", err)
	}
	started := time.Now()
	var latest atomic.Int64
	latest.Store(time.Now().UnixNano())
	var stalled atomic.Bool
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if time.Since(time.Unix(0, latest.Load())) > 30*time.Second {
					stalled.Store(true)
					cancel()
					return
				}
			case <-done:
				return
			}
		}
	}()
	t.setInstallState(true, true, "downloading", fmt.Sprintf("正在下载 %s…", asset.Name), completed+offset)
	progress := &downloadProgressReader{
		reader: response.Body,
		report: func(downloaded int64) {
			latest.Store(time.Now().UnixNano())
			elapsed := time.Since(started).Seconds()
			var speed int64
			if elapsed > 0 {
				speed = int64(float64(downloaded) / elapsed)
			}
			t.setDownloadProgress(completed+offset+downloaded, speed)
		},
	}
	written, copyErr := io.Copy(file, io.LimitReader(progress, asset.Size-offset+1))
	close(done)
	closeErr := file.Close()
	if stalled.Load() {
		return errors.New("download stalled for more than 30 seconds")
	}
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > asset.Size-offset {
		return errors.New("response exceeds the fixed manifest size")
	}
	if offset+written != asset.Size {
		return fmt.Errorf("connection closed at %d of %d bytes", offset+written, asset.Size)
	}
	return nil
}

func existingFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		return -1
	}
	if !info.Mode().IsRegular() {
		return -1
	}
	return info.Size()
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type downloadProgressReader struct {
	reader io.Reader
	read   int64
	report func(int64)
}

func (t *Toolchain) cleanupStaleInstallDirectories() {
	if t == nil || strings.TrimSpace(t.managedRoot) == "" {
		return
	}
	root, err := filepath.Abs(t.managedRoot)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".ffmpeg-install-") {
			continue
		}
		target := filepath.Join(root, entry.Name())
		if filepath.Dir(target) == root {
			_ = os.RemoveAll(target)
		}
	}
}

func (r *downloadProgressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		r.read += int64(n)
		if r.report != nil {
			r.report(r.read)
		}
	}
	return n, err
}

func unpackZIPExecutable(source, destination, expectedName string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("open zip package: %w", err)
	}
	defer archive.Close()
	expectedName = strings.ToLower(expectedName)
	for _, entry := range archive.File {
		base := strings.ToLower(filepath.Base(filepath.ToSlash(entry.Name)))
		if base != expectedName || entry.FileInfo().IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.UncompressedSize64 > 256<<20 {
			return fmt.Errorf("zip entry %s exceeds the extraction limit", entry.Name)
		}
		reader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", entry.Name, err)
		}
		writeErr := writeExecutable(destination, io.LimitReader(reader, 256<<20))
		closeErr := reader.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return fmt.Errorf("close zip entry %s: %w", entry.Name, closeErr)
		}
		return nil
	}
	return fmt.Errorf("zip package does not contain %s", expectedName)
}

func unpackTarXZPair(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open tar.xz package: %w", err)
	}
	defer input.Close()
	xzReader, err := xz.NewReader(input)
	if err != nil {
		return fmt.Errorf("read xz package: %w", err)
	}
	archive := tar.NewReader(xzReader)
	ffmpegName, ffprobeName := executableNames()
	wanted := map[string]string{
		strings.ToLower(ffmpegName):  ffmpegName,
		strings.ToLower(ffprobeName): ffprobeName,
	}
	found := make(map[string]bool, len(wanted))
	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("read tar.xz package: %w", nextErr)
		}
		base := strings.ToLower(filepath.Base(filepath.ToSlash(header.Name)))
		destinationName, ok := wanted[base]
		if !ok || found[base] || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		if header.Size < 0 || header.Size > 256<<20 {
			return fmt.Errorf("tar entry %s exceeds the extraction limit", header.Name)
		}
		if err := writeExecutable(filepath.Join(destination, destinationName), io.LimitReader(archive, header.Size)); err != nil {
			return err
		}
		found[base] = true
	}
	for name := range wanted {
		if !found[name] {
			return fmt.Errorf("tar.xz package does not contain %s", name)
		}
	}
	return nil
}

func unpackZIPPair(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("open zip package: %w", err)
	}
	defer archive.Close()
	ffmpegName, ffprobeName := executableNames()
	wanted := map[string]string{
		strings.ToLower(ffmpegName):  ffmpegName,
		strings.ToLower(ffprobeName): ffprobeName,
	}
	found := make(map[string]bool, len(wanted))
	for _, entry := range archive.File {
		base := strings.ToLower(filepath.Base(filepath.ToSlash(entry.Name)))
		destinationName, ok := wanted[base]
		if !ok || found[base] || entry.FileInfo().IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.UncompressedSize64 > 256<<20 {
			return fmt.Errorf("zip entry %s exceeds the extraction limit", entry.Name)
		}
		reader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", entry.Name, err)
		}
		writeErr := writeExecutable(filepath.Join(destination, destinationName), io.LimitReader(reader, 256<<20))
		closeErr := reader.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return fmt.Errorf("close zip entry %s: %w", entry.Name, closeErr)
		}
		found[base] = true
	}
	for name := range wanted {
		if !found[name] {
			return fmt.Errorf("zip package does not contain %s", name)
		}
	}
	return nil
}

func writeExecutable(destination string, source io.Reader) error {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create executable %s: %w", destination, err)
	}
	_, copyErr := io.Copy(output, source)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("extract executable %s: %w", destination, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close executable %s: %w", destination, closeErr)
	}
	if err := os.Chmod(destination, 0o755); err != nil {
		return fmt.Errorf("mark executable %s: %w", destination, err)
	}
	return nil
}

func executableNames() (string, string) {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe", "ffprobe.exe"
	}
	return "ffmpeg", "ffprobe"
}
