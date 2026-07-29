// Package ffmpeg provides a small, typed process boundary around FFmpeg and
// FFprobe. Callers supply argument slices; no shell is ever involved.
package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Available        bool             `json:"available"`
	FFmpegPath       string           `json:"ffmpeg_path"`
	FFprobePath      string           `json:"ffprobe_path"`
	Version          string           `json:"version"`
	Source           string           `json:"source"`
	Message          string           `json:"message"`
	Platform         string           `json:"platform"`
	InstallSupported bool             `json:"install_supported"`
	InstallVersion   string           `json:"install_version"`
	InstallDirectory string           `json:"install_directory"`
	DownloadBytes    int64            `json:"download_bytes"`
	DownloadedBytes  int64            `json:"downloaded_bytes"`
	DownloadSpeed    int64            `json:"download_speed_bytes"`
	ETASeconds       int64            `json:"eta_seconds"`
	InstallProgress  float64          `json:"install_progress"`
	InstallStage     string           `json:"install_stage"`
	InstallMode      string           `json:"install_mode"`
	Installing       bool             `json:"installing"`
	CanCancel        bool             `json:"can_cancel"`
	ManualDownloads  []ManualDownload `json:"manual_downloads"`
	ManualFiles      []string         `json:"manual_files"`
}

type ManualDownload struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type ProbeResult struct {
	Raw      json.RawMessage
	Duration float64
}

type Toolchain struct {
	stateMu       sync.RWMutex
	installMu     sync.Mutex
	ffmpeg        string
	ffprobe       string
	status        Status
	managedRoot   string
	release       platformRelease
	httpClient    *http.Client
	installCancel context.CancelFunc
	installUnlock func()
}

func Discover(managedRoots ...string) *Toolchain {
	managedRoot := ""
	if len(managedRoots) > 0 {
		managedRoot = strings.TrimSpace(managedRoots[0])
	}
	release, _ := releaseFor(runtime.GOOS, runtime.GOARCH)
	tool := &Toolchain{
		managedRoot: managedRoot,
		release:     release,
	}
	tool.cleanupStaleInstallDirectories()
	tool.refresh()
	return tool
}

func (t *Toolchain) refresh() {
	ffmpegName, ffprobeName := "ffmpeg", "ffprobe"
	if runtime.GOOS == "windows" {
		ffmpegName, ffprobeName = "ffmpeg.exe", "ffprobe.exe"
	}
	type candidate struct {
		dir    string
		source string
	}
	candidates := make([]candidate, 0, 5)
	if t.managedRoot != "" {
		if t.release.BundleID != "" {
			candidates = append(candidates, candidate{dir: filepath.Join(t.managedRoot, t.release.BundleID), source: "managed"})
		}
		candidates = append(candidates, candidate{dir: t.managedRoot, source: "managed_directory"})
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, candidate{dir: dir, source: "executable_directory"})
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, candidate{dir: cwd, source: "working_directory"})
	}
	for _, item := range candidates {
		ffmpegPath := filepath.Join(item.dir, ffmpegName)
		ffprobePath := filepath.Join(item.dir, ffprobeName)
		if regularFile(ffmpegPath) && regularFile(ffprobePath) {
			status := inspectToolchain(ffmpegPath, ffprobePath, item.source)
			t.updatePaths(ffmpegPath, ffprobePath, status)
			return
		}
	}
	ffmpegPath, ffmpegErr := exec.LookPath(ffmpegName)
	ffprobePath, ffprobeErr := exec.LookPath(ffprobeName)
	if ffmpegErr == nil && ffprobeErr == nil {
		status := inspectToolchain(ffmpegPath, ffprobePath, "path")
		t.updatePaths(ffmpegPath, ffprobePath, status)
		return
	}
	message := fmt.Sprintf("未找到 FFmpeg；可一键安装固定版本，或将 %s 和 %s 放在 SagaFlow 同目录或系统 PATH", ffmpegName, ffprobeName)
	t.updatePaths("", "", t.decorateStatus(Status{Message: message}))
}

func inspectToolchain(ffmpegPath, ffprobePath, source string) Status {
	status := Status{Available: true, FFmpegPath: ffmpegPath, FFprobePath: ffprobePath, Source: source}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffmpegPath, "-version").Output()
	if err != nil {
		status.Available = false
		status.Message = "FFmpeg 无法启动：" + err.Error()
	} else {
		first, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
		status.Version = strings.TrimSpace(first)
		status.Message = "FFmpeg 已就绪"
	}
	return status
}

func (t *Toolchain) decorateStatus(status Status) Status {
	status.Platform = runtime.GOOS + "/" + runtime.GOARCH
	status.InstallSupported = t != nil && t.managedRoot != "" && t.release.BundleID != ""
	status.InstallVersion = t.release.Version
	status.DownloadBytes = t.release.DownloadBytes
	if status.InstallSupported {
		status.InstallDirectory = filepath.Join(t.managedRoot, t.release.BundleID)
	}
	status.ManualDownloads = make([]ManualDownload, 0, len(t.release.Assets))
	for _, asset := range t.release.Assets {
		status.ManualDownloads = append(status.ManualDownloads, ManualDownload{
			Name: filepath.Base(asset.URL), URL: asset.URL, SHA256: asset.SHA256, Size: asset.Size,
		})
	}
	ffmpegName, ffprobeName := executableNames()
	status.ManualFiles = []string{ffmpegName, ffprobeName}
	return status
}

// Refresh repeats local executable discovery after a manual installation.
func (t *Toolchain) Refresh() Status {
	if t == nil {
		return Status{Message: "FFmpeg 服务未初始化"}
	}
	t.refresh()
	return t.Status()
}

func (t *Toolchain) updatePaths(ffmpegPath, ffprobePath string, status Status) {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	t.ffmpeg = ffmpegPath
	t.ffprobe = ffprobePath
	t.status = t.decorateStatus(status)
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (t *Toolchain) Status() Status {
	if t == nil {
		return Status{Message: "FFmpeg 服务未初始化"}
	}
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.status
}

func (t *Toolchain) executablePaths() (string, string, Status) {
	if t == nil {
		return "", "", Status{}
	}
	t.stateMu.RLock()
	defer t.stateMu.RUnlock()
	return t.ffmpeg, t.ffprobe, t.status
}

func (t *Toolchain) Probe(ctx context.Context, path string) (ProbeResult, error) {
	_, ffprobePath, status := t.executablePaths()
	if !status.Available {
		return ProbeResult{}, errors.New("FFmpeg is unavailable")
	}
	output, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error", "-show_format", "-show_streams", "-of", "json", path).Output()
	if err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe: %w", err)
	}
	var document struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return ProbeResult{}, fmt.Errorf("decode ffprobe output: %w", err)
	}
	duration, _ := strconv.ParseFloat(document.Format.Duration, 64)
	return ProbeResult{Raw: json.RawMessage(output), Duration: duration}, nil
}

// Run starts FFmpeg and reports a 0..1 media progress value. Returning an
// error from onProgress cancels the child process.
func (t *Toolchain) Run(ctx context.Context, args []string, duration float64, onProgress func(float64) error) error {
	ffmpegPath, _, status := t.executablePaths()
	if !status.Available {
		return errors.New("FFmpeg is unavailable")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	progressArgs := append([]string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-nostats"}, args...)
	command := exec.CommandContext(runCtx, ffmpegPath, progressArgs...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &limitedBuffer{limit: 64 << 10}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	var callbackErr error
	var callbackMu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			key, value, ok := strings.Cut(scanner.Text(), "=")
			if !ok {
				continue
			}
			progress := -1.0
			switch key {
			case "out_time_us":
				microseconds, _ := strconv.ParseFloat(value, 64)
				if duration > 0 {
					progress = microseconds / 1_000_000 / duration
				}
			case "progress":
				if value == "end" {
					progress = 1
				}
			}
			if progress < 0 || onProgress == nil {
				continue
			}
			if progress > 1 {
				progress = 1
			}
			if err := onProgress(progress); err != nil {
				callbackMu.Lock()
				callbackErr = err
				callbackMu.Unlock()
				cancel()
				return
			}
		}
	}()
	waitErr := command.Wait()
	<-done
	callbackMu.Lock()
	reportErr := callbackErr
	callbackMu.Unlock()
	if reportErr != nil {
		return reportErr
	}
	if waitErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("ffmpeg: %s", detail)
		}
		return fmt.Errorf("ffmpeg: %w", waitErr)
	}
	return nil
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.buffer.Len() < b.limit {
		remaining := b.limit - b.buffer.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buffer.Write(p)
	}
	return original, nil
}

func (b *limitedBuffer) String() string { return b.buffer.String() }
