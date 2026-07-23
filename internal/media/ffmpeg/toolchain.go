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
	Available   bool   `json:"available"`
	FFmpegPath  string `json:"ffmpeg_path"`
	FFprobePath string `json:"ffprobe_path"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	Message     string `json:"message"`
}

type ProbeResult struct {
	Raw      json.RawMessage
	Duration float64
}

type Toolchain struct {
	ffmpeg  string
	ffprobe string
	status  Status
}

func Discover() *Toolchain {
	ffmpegName, ffprobeName := "ffmpeg", "ffprobe"
	if runtime.GOOS == "windows" {
		ffmpegName, ffprobeName = "ffmpeg.exe", "ffprobe.exe"
	}
	type candidate struct {
		dir    string
		source string
	}
	candidates := make([]candidate, 0, 8)
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates,
			candidate{dir: dir, source: "executable_directory"},
			candidate{dir: filepath.Join(dir, "tools", "ffmpeg", runtime.GOOS+"-"+runtime.GOARCH), source: "bundled_tools"},
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			candidate{dir: cwd, source: "working_directory"},
			candidate{dir: filepath.Join(cwd, "tools", "ffmpeg", runtime.GOOS+"-"+runtime.GOARCH), source: "repository_tools"},
		)
	}
	for _, item := range candidates {
		ffmpegPath := filepath.Join(item.dir, ffmpegName)
		ffprobePath := filepath.Join(item.dir, ffprobeName)
		if regularFile(ffmpegPath) && regularFile(ffprobePath) {
			return newToolchain(ffmpegPath, ffprobePath, item.source)
		}
	}
	ffmpegPath, ffmpegErr := exec.LookPath(ffmpegName)
	ffprobePath, ffprobeErr := exec.LookPath(ffprobeName)
	if ffmpegErr == nil && ffprobeErr == nil {
		return newToolchain(ffmpegPath, ffprobePath, "path")
	}
	message := "未找到 FFmpeg；请将 ffmpeg 和 ffprobe 放在 SagaFlow 同目录"
	return &Toolchain{status: Status{Message: message}}
}

func newToolchain(ffmpegPath, ffprobePath, source string) *Toolchain {
	tool := &Toolchain{ffmpeg: ffmpegPath, ffprobe: ffprobePath}
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
	tool.status = status
	return tool
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (t *Toolchain) Status() Status { return t.status }

func (t *Toolchain) Probe(ctx context.Context, path string) (ProbeResult, error) {
	if t == nil || !t.status.Available {
		return ProbeResult{}, errors.New("FFmpeg is unavailable")
	}
	output, err := exec.CommandContext(ctx, t.ffprobe,
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
	if t == nil || !t.status.Available {
		return errors.New("FFmpeg is unavailable")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	progressArgs := append([]string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-nostats"}, args...)
	command := exec.CommandContext(runCtx, t.ffmpeg, progressArgs...)
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
