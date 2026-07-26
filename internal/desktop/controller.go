package desktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gofurry/sagaflow/internal/runtimecontract"
)

const DefaultURL = "http://127.0.0.1:18848"

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

type Snapshot struct {
	Status    Status
	Message   string
	Version   string
	URL       string
	CorePath  string
	DataDir   string
	LogFile   string
	PID       int
	Managed   bool
	StartedAt time.Time
}

type Options struct {
	BaseURL           string
	CorePath          string
	RuntimeFile       string
	LogFile           string
	HTTPClient        *http.Client
	CatalogURL        string
	CatalogHTTPClient *http.Client
	OnChange          func(Snapshot)
}

// Controller owns only core processes it started. It can attach to an already
// running compatible core, but deliberately refuses to stop that external
// process.
type Controller struct {
	mu          sync.Mutex
	baseURL     string
	corePath    string
	runtimeFile string
	logFile     string
	httpClient  *http.Client
	onChange    func(Snapshot)
	snapshot    Snapshot
	cmd         *exec.Cmd
	token       string
	done        chan struct{}
	waitErr     error
	logHandle   io.Closer
	resources   *resourceManager
}

type healthEnvelope struct {
	Data struct {
		Status            string `json:"status"`
		Version           string `json:"version"`
		APIVersion        int    `json:"api_version"`
		DataSchemaVersion int    `json:"data_schema_version"`
	} `json:"data"`
}

func NewController(options Options) (*Controller, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultURL
	}
	corePath, err := ResolveCorePath(options.CorePath)
	if err != nil && strings.TrimSpace(options.CorePath) != "" {
		return nil, err
	}
	dataDir := dataDirectory(corePath)
	runtimeFile := options.RuntimeFile
	if runtimeFile == "" {
		runtimeFile = filepath.Join(dataDir, "desktop-runtime.json")
	}
	logFile := options.LogFile
	if logFile == "" {
		logFile = filepath.Join(dataDir, "logs", "sagaflow-core.log")
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	controller := &Controller{
		baseURL: baseURL, corePath: corePath, runtimeFile: runtimeFile, logFile: logFile,
		httpClient: client, onChange: options.OnChange,
		resources: newResourceManager(dataDir, options.CatalogURL, options.CatalogHTTPClient),
	}
	controller.snapshot = Snapshot{
		Status: StatusStopped, Message: "内核尚未启动", URL: baseURL,
		CorePath: corePath, DataDir: dataDir, LogFile: logFile,
	}
	if corePath == "" {
		controller.snapshot.Message = "未找到 sagaflow 内核"
	}
	return controller, nil
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot
}

func (c *Controller) SetOnChange(onChange func(Snapshot)) {
	c.mu.Lock()
	c.onChange = onChange
	snapshot := c.snapshot
	c.mu.Unlock()
	if onChange != nil {
		onChange(snapshot)
	}
}

func (c *Controller) Refresh(ctx context.Context) Snapshot {
	health, err := c.health(ctx)
	c.mu.Lock()
	busy := c.snapshot.Status == StatusStarting || c.snapshot.Status == StatusStopping
	if !busy {
		if err == nil {
			if compatibilityErr := compatibleHealth(health); compatibilityErr != nil {
				c.snapshot.Status = StatusError
				c.snapshot.Message = compatibilityErr.Error()
				c.snapshot.Version = health.Data.Version
			} else {
				c.snapshot.Status = StatusRunning
				c.snapshot.Message = "SagaFlow 内核运行正常"
				c.snapshot.Version = health.Data.Version
				if c.cmd == nil {
					c.snapshot.Managed = false
					c.snapshot.PID = 0
				}
			}
		} else if c.cmd == nil {
			c.snapshot.Status = StatusStopped
			c.snapshot.Message = "内核尚未启动"
			c.snapshot.Version = ""
			c.snapshot.PID = 0
			c.snapshot.Managed = false
		}
	}
	snapshot, onChange := c.snapshot, c.onChange
	c.mu.Unlock()
	if onChange != nil {
		onChange(snapshot)
	}
	return snapshot
}

func (c *Controller) Start(ctx context.Context) error {
	if health, err := c.health(ctx); err == nil {
		if err := compatibleHealth(health); err != nil {
			c.setState(StatusError, err.Error())
			return err
		}
		c.mu.Lock()
		c.snapshot.Status = StatusRunning
		c.snapshot.Message = "已连接正在运行的 SagaFlow 内核"
		c.snapshot.Version = health.Data.Version
		c.snapshot.Managed = c.cmd != nil
		snapshot, onChange := c.snapshot, c.onChange
		c.mu.Unlock()
		notify(onChange, snapshot)
		return nil
	}

	c.mu.Lock()
	if c.cmd != nil {
		c.mu.Unlock()
		return errors.New("SagaFlow 内核正在启动")
	}
	corePath := c.corePath
	if corePath == "" {
		c.mu.Unlock()
		var err error
		corePath, err = ResolveCorePath("")
		if err != nil {
			c.setState(StatusError, err.Error())
			return err
		}
		c.mu.Lock()
		c.corePath = corePath
		c.snapshot.CorePath = corePath
		c.snapshot.DataDir = dataDirectory(corePath)
	}
	c.snapshot.Status = StatusStarting
	c.snapshot.Message = "正在启动 SagaFlow 内核…"
	dataDir := c.snapshot.DataDir
	snapshot, onChange := c.snapshot, c.onChange
	c.mu.Unlock()
	notify(onChange, snapshot)

	token, err := randomToken()
	if err != nil {
		c.setState(StatusError, "无法创建桌面控制令牌")
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.logFile), 0o755); err != nil {
		c.setState(StatusError, "无法创建日志目录")
		return err
	}
	logHandle := newCoreLogWriter(c.logFile)
	command := exec.Command(corePath, "serve", "--runtime-file", c.runtimeFile)
	command.Env = environmentWith(map[string]string{
		"SAGAFLOW_HOST":          "127.0.0.1",
		"SAGAFLOW_PORT":          "18848",
		"SAGAFLOW_DESKTOP_TOKEN": token,
		"SAGAFLOW_DATA_DIR":      dataDir,
	})
	command.Stdout = logHandle
	command.Stderr = logHandle
	configureProcess(command)
	if err := command.Start(); err != nil {
		_ = logHandle.Close()
		c.setState(StatusError, fmt.Sprintf("内核启动失败：%v", err))
		return err
	}

	done := make(chan struct{})
	c.mu.Lock()
	c.cmd = command
	c.token = token
	c.done = done
	c.waitErr = nil
	c.logHandle = logHandle
	c.snapshot.Managed = true
	c.snapshot.PID = command.Process.Pid
	c.snapshot.StartedAt = time.Now()
	c.mu.Unlock()
	go c.wait(command, done)

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = c.Stop(context.Background())
			return ctx.Err()
		case <-done:
			c.mu.Lock()
			waitErr := c.waitErr
			c.mu.Unlock()
			if waitErr == nil {
				waitErr = errors.New("内核在完成启动前退出")
			}
			return fmt.Errorf("内核启动失败：%w；请检查日志 %s", waitErr, c.logFile)
		case <-ticker.C:
			health, err := c.health(ctx)
			if err != nil {
				continue
			}
			if err := compatibleHealth(health); err != nil {
				_ = c.Stop(context.Background())
				c.setState(StatusError, err.Error())
				return err
			}
			c.mu.Lock()
			c.snapshot.Status = StatusRunning
			c.snapshot.Message = "SagaFlow 内核运行正常"
			c.snapshot.Version = health.Data.Version
			snapshot, onChange := c.snapshot, c.onChange
			c.mu.Unlock()
			notify(onChange, snapshot)
			return nil
		case <-timer.C:
			_ = c.Stop(context.Background())
			err := fmt.Errorf("内核启动超时，请检查日志：%s", c.logFile)
			c.setState(StatusError, err.Error())
			return err
		}
	}
}

func (c *Controller) Stop(ctx context.Context) error {
	c.mu.Lock()
	command, token, done := c.cmd, c.token, c.done
	if command == nil {
		c.mu.Unlock()
		if _, err := c.health(ctx); err == nil {
			return errors.New("当前内核并非由此启动器启动，无法安全停止")
		}
		c.setState(StatusStopped, "内核已经停止")
		return nil
	}
	c.snapshot.Status = StatusStopping
	c.snapshot.Message = "正在安全停止 SagaFlow 内核…"
	snapshot, onChange := c.snapshot, c.onChange
	c.mu.Unlock()
	notify(onChange, snapshot)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/_desktop/shutdown", nil)
	if err == nil {
		request.Header.Set(runtimecontract.DesktopTokenHeader, token)
		response, requestErr := c.httpClient.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusAccepted {
				err = fmt.Errorf("内核拒绝安全停止请求：HTTP %d", response.StatusCode)
			}
		} else {
			err = requestErr
		}
	}

	select {
	case <-done:
		c.setState(StatusStopped, "SagaFlow 内核已停止")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(8 * time.Second):
		if killErr := command.Process.Kill(); killErr != nil {
			if err != nil {
				return fmt.Errorf("%v；强制停止也失败：%w", err, killErr)
			}
			return fmt.Errorf("强制停止内核：%w", killErr)
		}
		<-done
		c.setState(StatusStopped, "内核未及时响应，已强制停止")
		return nil
	}
}

func (c *Controller) Restart(ctx context.Context) error {
	if err := c.Stop(ctx); err != nil {
		return err
	}
	return c.Start(ctx)
}

func (c *Controller) Doctor(ctx context.Context) (string, error) {
	if err := c.ensureMaintenanceTarget(); err != nil {
		return "", err
	}
	output, err := c.runCoreCommand(ctx, "doctor", "--json")
	if err != nil {
		return strings.TrimSpace(output), fmt.Errorf("诊断失败：%w", err)
	}
	var result struct {
		DataDir             string `json:"data_dir"`
		Database            string `json:"database"`
		AccountInitialized  bool   `json:"account_initialized"`
		ProviderCredentials int    `json:"provider_credentials"`
		S3Credentials       int    `json:"s3_credentials"`
		FFmpegAvailable     bool   `json:"ffmpeg_available"`
		FFmpegVersion       string `json:"ffmpeg_version"`
		FFmpegPath          string `json:"ffmpeg_path"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return strings.TrimSpace(output), nil
	}
	ffmpeg := "未安装"
	if result.FFmpegAvailable {
		ffmpeg = "已就绪"
		if version := conciseFFmpegVersion(result.FFmpegVersion); version != "" {
			ffmpeg += "（" + version + "）"
		}
		if result.FFmpegPath != "" {
			ffmpeg += "\nFFmpeg 路径：" + result.FFmpegPath
		}
	}
	storage := "未配置"
	if result.S3Credentials > 0 {
		storage = fmt.Sprintf("已保存 %d 个凭证", result.S3Credentials)
	}
	logUsage := "无法读取"
	if bytes, usageErr := c.LogUsage(); usageErr == nil {
		logUsage = humanBytes(bytes)
	}
	return fmt.Sprintf(
		"工作台数据：正常\n账号状态：%s\n模型服务：已保存 %d 个凭证\n云端存储：%s\nFFmpeg：%s\n日志占用：%s\n\n数据位置：%s",
		yesNo(result.AccountInitialized), result.ProviderCredentials, storage, ffmpeg, logUsage, result.DataDir,
	), nil
}

func (c *Controller) Backup(ctx context.Context) (string, error) {
	if err := c.ensureMaintenanceTarget(); err != nil {
		return "", err
	}
	output, err := c.runCoreCommand(ctx, "backup", "create")
	if err != nil {
		return strings.TrimSpace(output), fmt.Errorf("备份失败：%w", err)
	}
	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(output), "backup written to "))
	if path == "" {
		return "备份已完成", nil
	}
	return "备份已完成。\n\n文件保存在：\n" + path, nil
}

func (c *Controller) OpenWorkbench() error {
	return Open(c.baseURL)
}

func (c *Controller) OpenDataDirectory() error {
	path := c.Snapshot().DataDir
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建数据目录：%w", err)
	}
	return Open(path)
}

func (c *Controller) OpenLogDirectory() error {
	path := filepath.Dir(c.Snapshot().LogFile)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建日志目录：%w", err)
	}
	return Open(path)
}

func (c *Controller) OpenBackupDirectory() error {
	path := filepath.Join(c.Snapshot().DataDir, "backups")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建备份目录：%w", err)
	}
	return Open(path)
}

func (c *Controller) wait(command *exec.Cmd, done chan struct{}) {
	err := command.Wait()
	c.mu.Lock()
	if c.cmd != command {
		c.mu.Unlock()
		close(done)
		return
	}
	if c.logHandle != nil {
		_ = c.logHandle.Close()
		c.logHandle = nil
	}
	c.waitErr = err
	c.cmd = nil
	c.token = ""
	c.done = nil
	c.snapshot.Managed = false
	c.snapshot.PID = 0
	c.snapshot.StartedAt = time.Time{}
	if c.snapshot.Status != StatusStopping {
		c.snapshot.Status = StatusError
		c.snapshot.Message = "SagaFlow 内核意外退出"
		if err != nil {
			c.snapshot.Message += "：" + err.Error() + "；请检查日志"
		}
	}
	snapshot, onChange := c.snapshot, c.onChange
	c.mu.Unlock()
	close(done)
	notify(onChange, snapshot)
}

func (c *Controller) runCoreCommand(ctx context.Context, arguments ...string) (string, error) {
	c.mu.Lock()
	corePath := c.corePath
	c.mu.Unlock()
	if corePath == "" {
		var err error
		corePath, err = ResolveCorePath("")
		if err != nil {
			return "", err
		}
	}
	command := exec.CommandContext(ctx, corePath, arguments...)
	command.Env = environmentWith(map[string]string{"SAGAFLOW_DATA_DIR": c.Snapshot().DataDir})
	configureProcess(command)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return stdout.String(), fmt.Errorf("%w：%s", err, detail)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func (c *Controller) ensureMaintenanceTarget() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot.Status == StatusRunning && !c.snapshot.Managed {
		return errors.New("当前连接的是外部内核；请在其原始启动位置执行维护操作")
	}
	return nil
}

func (c *Controller) health(ctx context.Context) (healthEnvelope, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return healthEnvelope{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return healthEnvelope{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return healthEnvelope{}, fmt.Errorf("健康检查返回 HTTP %d", response.StatusCode)
	}
	var health healthEnvelope
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health); err != nil {
		return healthEnvelope{}, err
	}
	return health, nil
}

func compatibleHealth(health healthEnvelope) error {
	if health.Data.Status != "ok" {
		return errors.New("内核健康检查未返回 ok")
	}
	if health.Data.APIVersion != runtimecontract.APIVersion {
		return fmt.Errorf("启动器需要 API %d，当前内核提供 API %d", runtimecontract.APIVersion, health.Data.APIVersion)
	}
	return nil
}

func ResolveCorePath(configured string) (string, error) {
	binaryName := "sagaflow"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	var candidates []string
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("SAGAFLOW_CORE")); fromEnv != "" {
		candidates = append(candidates, fromEnv)
	}
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, packagedCoreCandidates(executableDir)...)
		if runtime.GOOS == "darwin" {
			// Keep compatibility with early portable packages that placed the core
			// beside SagaFlow.app instead of under Contents/Helpers.
			candidates = append(candidates, filepath.Join(executableDir, "..", "..", "..", binaryName))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, binaryName), filepath.Join(cwd, "bin", binaryName))
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err == nil && !info.IsDir() {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("未找到 SagaFlow 内核，桌面发行包可能不完整")
}

func packagedCoreCandidates(executableDir string) []string {
	legacyName := "sagaflow"
	coreName := "sagaflow-core"
	if runtime.GOOS == "windows" {
		legacyName += ".exe"
		coreName += ".exe"
	}
	switch runtime.GOOS {
	case "windows":
		return []string{filepath.Join(executableDir, "runtime", coreName), filepath.Join(executableDir, legacyName)}
	case "linux":
		return []string{filepath.Join(executableDir, "libexec", coreName), filepath.Join(executableDir, legacyName)}
	case "darwin":
		return []string{filepath.Join(executableDir, "..", "Helpers", coreName), filepath.Join(executableDir, legacyName)}
	default:
		return []string{filepath.Join(executableDir, legacyName)}
	}
}

func dataDirectory(corePath string) string {
	if configured := strings.TrimSpace(os.Getenv("SAGAFLOW_DATA_DIR")); configured != "" {
		return filepath.Clean(configured)
	}
	if runtime.GOOS == "darwin" {
		if configDir, err := os.UserConfigDir(); err == nil {
			return filepath.Join(configDir, "SagaFlow")
		}
	}
	if corePath != "" {
		coreDir := filepath.Dir(corePath)
		if directory := filepath.Base(coreDir); directory == "runtime" || directory == "libexec" {
			return filepath.Join(filepath.Dir(coreDir), "data")
		}
		return filepath.Join(coreDir, "data")
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "data")
	}
	return "data"
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func environmentWith(overrides map[string]string) []string {
	environment := os.Environ()
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		overridden := false
		for key := range overrides {
			if name == key || (runtime.GOOS == "windows" && strings.EqualFold(name, key)) {
				overridden = true
				break
			}
		}
		if !overridden {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func (c *Controller) setState(status Status, message string) {
	c.mu.Lock()
	c.snapshot.Status = status
	c.snapshot.Message = message
	snapshot, onChange := c.snapshot, c.onChange
	c.mu.Unlock()
	notify(onChange, snapshot)
}

func notify(onChange func(Snapshot), snapshot Snapshot) {
	if onChange != nil {
		onChange(snapshot)
	}
}

func yesNo(value bool) string {
	if value {
		return "已配置"
	}
	return "尚未配置"
}

func conciseFFmpegVersion(value string) string {
	fields := strings.Fields(value)
	for index, field := range fields {
		if strings.EqualFold(field, "version") && index+1 < len(fields) {
			return fields[index+1]
		}
	}
	return ""
}
