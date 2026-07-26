package desktop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	mediaffmpeg "github.com/gofurry/sagaflow/internal/media/ffmpeg"
	"github.com/gofurry/sagaflow/internal/modelcatalog"
	"github.com/gofurry/sagaflow/internal/providercatalog"
)

type FFmpegStatus = mediaffmpeg.Status
type FFmpegInstallOptions = mediaffmpeg.InstallOptions

type CatalogUpdateCheck struct {
	CurrentVersion  string
	RemoteVersion   string
	CurrentModels   int
	RemoteModels    int
	Added           int
	Updated         int
	Retired         int
	Removed         int
	UpdateAvailable bool

	manifest modelcatalog.Manifest
}

type resourceManager struct {
	ffmpeg        *mediaffmpeg.Toolchain
	catalogPath   string
	catalogURL    string
	catalogClient *http.Client
}

func newResourceManager(dataDir, catalogURL string, client *http.Client) *resourceManager {
	if strings.TrimSpace(catalogURL) == "" {
		catalogURL = modelcatalog.DefaultManifestURL
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &resourceManager{
		ffmpeg:        mediaffmpeg.Discover(filepath.Join(dataDir, "tools", "ffmpeg")),
		catalogPath:   filepath.Join(dataDir, "catalog", "model-catalog.json"),
		catalogURL:    catalogURL,
		catalogClient: client,
	}
}

func (c *Controller) FFmpegStatus() FFmpegStatus {
	if c == nil || c.resources == nil {
		return FFmpegStatus{Message: "FFmpeg 服务未初始化"}
	}
	return c.resources.ffmpeg.Status()
}

func (c *Controller) RefreshFFmpeg() FFmpegStatus {
	if c == nil || c.resources == nil {
		return FFmpegStatus{Message: "FFmpeg 服务未初始化"}
	}
	return c.resources.ffmpeg.Refresh()
}

func (c *Controller) StartFFmpegInstall(options FFmpegInstallOptions) (FFmpegStatus, error) {
	if c == nil || c.resources == nil {
		return FFmpegStatus{}, errors.New("FFmpeg 服务未初始化")
	}
	return c.resources.ffmpeg.StartInstall(options)
}

func (c *Controller) CancelFFmpegInstall() FFmpegStatus {
	if c == nil || c.resources == nil {
		return FFmpegStatus{Message: "FFmpeg 服务未初始化"}
	}
	return c.resources.ffmpeg.CancelInstall()
}

func (c *Controller) OpenFFmpegDirectory() error {
	status := c.FFmpegStatus()
	path := status.InstallDirectory
	if path == "" {
		return errors.New("当前平台没有可用的 FFmpeg 安装目录")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("创建 FFmpeg 安装目录：%w", err)
	}
	return Open(path)
}

func (c *Controller) CheckModelCatalog(ctx context.Context) (CatalogUpdateCheck, error) {
	if c == nil || c.resources == nil {
		return CatalogUpdateCheck{}, errors.New("模型目录服务未初始化")
	}
	current, err := modelcatalog.EffectiveManifest(c.resources.catalogPath)
	if err != nil {
		return CatalogUpdateCheck{}, fmt.Errorf("读取当前模型目录：%w", err)
	}
	remote, err := modelcatalog.FetchManifest(ctx, c.resources.catalogClient, c.resources.catalogURL)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return CatalogUpdateCheck{}, errors.New("官方模型目录当前没有可用更新，请稍后重试")
		}
		return CatalogUpdateCheck{}, fmt.Errorf("检查模型目录更新：%w", err)
	}
	diff, err := modelcatalog.CompareManifests(current, remote)
	if err != nil {
		return CatalogUpdateCheck{}, fmt.Errorf("比较模型目录：%w", err)
	}
	return CatalogUpdateCheck{
		CurrentVersion: current.CatalogVersion, RemoteVersion: remote.CatalogVersion,
		CurrentModels: len(current.Models), RemoteModels: len(remote.Models),
		Added: diff.Added, Updated: diff.Updated, Retired: diff.Retired, Removed: diff.Removed,
		UpdateAvailable: catalogIsNewer(current, remote),
		manifest:        remote,
	}, nil
}

func (c *Controller) ApplyModelCatalog(check CatalogUpdateCheck) (modelcatalog.ManifestInfo, error) {
	if c == nil || c.resources == nil {
		return modelcatalog.ManifestInfo{}, errors.New("模型目录服务未初始化")
	}
	if !check.UpdateAvailable || check.manifest.CatalogVersion == "" {
		return modelcatalog.ManifestInfo{}, errors.New("没有可安装的模型目录更新")
	}
	supported := providercatalog.SupportedCodes()
	for _, model := range check.manifest.Models {
		if _, ok := supported[model.ProviderCode]; !ok {
			return modelcatalog.ManifestInfo{}, fmt.Errorf("更新包包含当前版本不支持的服务商 %q", model.ProviderCode)
		}
	}
	return modelcatalog.InstallManifestDocument(check.manifest, c.resources.catalogPath)
}

func catalogIsNewer(current, remote modelcatalog.Manifest) bool {
	if remote.CatalogVersion == "" || remote.CatalogVersion == current.CatalogVersion {
		return false
	}
	currentTime, currentErr := time.Parse(time.RFC3339, current.PublishedAt)
	remoteTime, remoteErr := time.Parse(time.RFC3339, remote.PublishedAt)
	if currentErr == nil && remoteErr == nil && !remoteTime.Equal(currentTime) {
		return remoteTime.After(currentTime)
	}
	return remote.CatalogVersion > current.CatalogVersion
}
