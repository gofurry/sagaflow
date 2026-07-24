package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/media/ffmpeg"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var errMediaJobCanceled = errors.New("media job canceled")

type MediaToolsService struct {
	store   *db.Store
	storage *storage.Manager
	tools   *ffmpeg.Toolchain
	tempDir string
	log     *zap.Logger
}

type CreateMediaJobInput struct {
	ProjectID          uuid.UUID
	TargetAssetGroupID *uuid.UUID
	Tool               string
	SourceAssetIDs     []uuid.UUID
	OutputName         string
	Parameters         json.RawMessage
}

type mediaParameters struct {
	Format       string  `json:"format"`
	AudioMode    string  `json:"audio_mode"`
	StartSeconds float64 `json:"start_seconds"`
	Duration     float64 `json:"duration"`
	Fast         bool    `json:"fast"`
	TimeSeconds  float64 `json:"time_seconds"`
	ImageFormat  string  `json:"image_format"`
	OutputWidth  int     `json:"output_width"`
	OutputHeight int     `json:"output_height"`
	CropX        int     `json:"crop_x"`
	CropY        int     `json:"crop_y"`
	CropWidth    int     `json:"crop_width"`
	CropHeight   int     `json:"crop_height"`
}

type mediaOutput struct {
	extension string
	mimeType  string
	mediaType string
}

func NewMediaToolsService(store *db.Store, objectStore *storage.Manager, tools *ffmpeg.Toolchain, tempDir string, log *zap.Logger) *MediaToolsService {
	if log == nil {
		log = zap.NewNop()
	}
	return &MediaToolsService{store: store, storage: objectStore, tools: tools, tempDir: tempDir, log: log}
}

func (s *MediaToolsService) Status() ffmpeg.Status {
	if s == nil || s.tools == nil {
		return ffmpeg.Status{Message: "FFmpeg 服务未初始化"}
	}
	return s.tools.Status()
}

func (s *MediaToolsService) StartToolchainInstall(options ffmpeg.InstallOptions) (ffmpeg.Status, error) {
	if s == nil || s.tools == nil {
		return ffmpeg.Status{}, fmt.Errorf("%w: FFmpeg 服务未初始化", ErrInvalidInput)
	}
	status, err := s.tools.StartInstall(options)
	if errors.Is(err, ffmpeg.ErrInstallUnsupported) {
		return status, fmt.Errorf("%w: 当前系统架构暂不支持一键安装", ErrInvalidInput)
	}
	return status, err
}

func (s *MediaToolsService) RefreshToolchain() ffmpeg.Status {
	if s == nil || s.tools == nil {
		return ffmpeg.Status{Message: "FFmpeg 服务未初始化"}
	}
	return s.tools.Refresh()
}

func (s *MediaToolsService) CancelToolchainInstall() ffmpeg.Status {
	if s == nil || s.tools == nil {
		return ffmpeg.Status{Message: "FFmpeg 服务未初始化"}
	}
	return s.tools.CancelInstall()
}

func (s *MediaToolsService) InspectAsset(ctx context.Context, assetID uuid.UUID) (json.RawMessage, error) {
	if !s.Status().Available {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, s.Status().Message)
	}
	asset, err := s.store.GetAsset(ctx, assetID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if !oneOf(asset.MediaType, "image", "audio", "video") {
		return nil, fmt.Errorf("%w: 当前资产不支持媒体检查", ErrInvalidInput)
	}
	file, _, err := s.storage.Open(ctx, asset.ObjectID)
	if err != nil {
		return nil, fmt.Errorf("open source asset: %w", err)
	}
	defer file.Close()
	probe, err := s.tools.Probe(ctx, file.Name())
	if err != nil {
		return nil, err
	}
	return probe.Raw, nil
}

func (s *MediaToolsService) Create(ctx context.Context, input CreateMediaJobInput) (db.MediaJob, error) {
	if !s.Status().Available {
		return db.MediaJob{}, fmt.Errorf("%w: %s", ErrInvalidInput, s.Status().Message)
	}
	if input.ProjectID == uuid.Nil {
		return db.MediaJob{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if _, err := s.store.GetProject(ctx, input.ProjectID); err != nil {
		return db.MediaJob{}, mapStoreError(err)
	}
	if err := validateMediaTool(input.Tool, len(input.SourceAssetIDs)); err != nil {
		return db.MediaJob{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	seen := make(map[uuid.UUID]struct{}, len(input.SourceAssetIDs))
	sourceAssets := make([]db.Asset, 0, len(input.SourceAssetIDs))
	for _, id := range input.SourceAssetIDs {
		if id == uuid.Nil {
			return db.MediaJob{}, fmt.Errorf("%w: source asset id is required", ErrInvalidInput)
		}
		if _, exists := seen[id]; exists {
			return db.MediaJob{}, fmt.Errorf("%w: duplicate source asset", ErrInvalidInput)
		}
		seen[id] = struct{}{}
		asset, err := s.store.GetAsset(ctx, id)
		if err != nil || asset.ProjectID != input.ProjectID {
			return db.MediaJob{}, fmt.Errorf("%w: source asset is unavailable", ErrInvalidInput)
		}
		sourceAssets = append(sourceAssets, asset)
	}
	if input.TargetAssetGroupID != nil {
		group, err := s.store.GetAssetGroup(ctx, *input.TargetAssetGroupID)
		if err != nil || group.ProjectID != input.ProjectID {
			return db.MediaJob{}, fmt.Errorf("%w: target group is unavailable", ErrInvalidInput)
		}
	}
	var parameters mediaParameters
	if len(input.Parameters) > 0 {
		if err := json.Unmarshal(input.Parameters, &parameters); err != nil {
			return db.MediaJob{}, fmt.Errorf("%w: invalid tool parameters", ErrInvalidInput)
		}
	}
	if err := validateMediaParameters(input.Tool, parameters); err != nil {
		return db.MediaJob{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validateMediaSources(input.Tool, parameters, sourceAssets); err != nil {
		return db.MediaJob{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return s.store.CreateMediaJob(ctx, db.CreateMediaJobInput{
		ProjectID: input.ProjectID, TargetAssetGroupID: input.TargetAssetGroupID, Tool: input.Tool,
		SourceAssetIDs: input.SourceAssetIDs, OutputName: input.OutputName, Parameters: input.Parameters,
	})
}

func validateMediaSources(tool string, p mediaParameters, assets []db.Asset) error {
	if len(assets) == 0 {
		return fmt.Errorf("请选择源资产")
	}
	firstType := assets[0].MediaType
	switch tool {
	case "inspect":
		if !oneOf(firstType, "image", "audio", "video") {
			return fmt.Errorf("媒体检查仅支持图像、音频和视频")
		}
	case "transcode":
		if !oneOf(firstType, "audio", "video") {
			return fmt.Errorf("格式转换仅支持音频和视频")
		}
		if firstType == "audio" && oneOf(p.Format, "mp4", "webm") {
			return fmt.Errorf("音频源只能转换为 MP3、WAV 或 M4A")
		}
	case "trim", "screenshot":
		if firstType != "video" {
			return fmt.Errorf("当前工具仅支持视频源")
		}
	case "audio":
		if !oneOf(firstType, "audio", "video") {
			return fmt.Errorf("音频处理仅支持音频或视频源")
		}
		if p.AudioMode == "mute" && firstType != "video" {
			return fmt.Errorf("移除声音需要视频源")
		}
	case "merge":
		for _, asset := range assets {
			if asset.MediaType != "video" {
				return fmt.Errorf("顺序合片仅支持视频源")
			}
		}
	}
	return nil
}

func validateMediaTool(tool string, sourceCount int) error {
	switch tool {
	case "inspect":
		if sourceCount != 1 {
			return fmt.Errorf("媒体检查需要 1 个源资产")
		}
	case "merge":
		if sourceCount < 2 {
			return fmt.Errorf("顺序合片至少需要 2 个源资产")
		}
	case "transcode", "audio", "trim", "screenshot":
		if sourceCount != 1 {
			return fmt.Errorf("当前工具需要 1 个源资产")
		}
	default:
		return fmt.Errorf("unsupported media tool %q", tool)
	}
	return nil
}

func validateMediaParameters(tool string, p mediaParameters) error {
	switch tool {
	case "transcode":
		if !oneOf(p.Format, "mp4", "webm", "mp3", "wav", "m4a") {
			return fmt.Errorf("请选择有效的输出格式")
		}
	case "audio":
		if !oneOf(p.AudioMode, "extract", "normalize", "mute") {
			return fmt.Errorf("请选择有效的音频工具")
		}
	case "trim":
		if p.StartSeconds < 0 || p.Duration <= 0 || p.Duration > 24*60*60 {
			return fmt.Errorf("请输入有效的起始位置和片段时长")
		}
	case "screenshot":
		if p.TimeSeconds < 0 || !oneOf(p.ImageFormat, "png", "jpg") {
			return fmt.Errorf("请输入有效的截图时间和格式")
		}
		if !optionalDimension(p.OutputWidth, 16, 7680) || !optionalDimension(p.OutputHeight, 16, 7680) {
			return fmt.Errorf("截图输出尺寸必须留空，或在 16 到 7680 像素之间")
		}
		cropEnabled := p.CropWidth != 0 || p.CropHeight != 0 || p.CropX != 0 || p.CropY != 0
		if cropEnabled && (p.CropX < 0 || p.CropY < 0 || p.CropWidth < 2 || p.CropHeight < 2) {
			return fmt.Errorf("裁剪区域必须包含非负起点以及至少 2×2 像素的宽高")
		}
	}
	return nil
}

func optionalDimension(value, minimum, maximum int) bool {
	return value == 0 || (value >= minimum && value <= maximum)
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func (s *MediaToolsService) Execute(ctx context.Context, id uuid.UUID) (returnErr error) {
	job, err := s.store.GetMediaJob(ctx, id)
	if err != nil {
		return err
	}
	if job.CancelRequested {
		_, _ = s.store.MarkMediaJobCanceled(ctx, id)
		return errMediaJobCanceled
	}
	fail := func(err error) error {
		if errors.Is(err, errMediaJobCanceled) {
			_, _ = s.store.MarkMediaJobCanceled(context.Background(), id)
			return err
		}
		if errors.Is(err, context.Canceled) {
			current, getErr := s.store.GetMediaJob(context.Background(), id)
			if getErr == nil && current.CancelRequested {
				_, _ = s.store.MarkMediaJobCanceled(context.Background(), id)
			}
			// On application shutdown leave the row running; startup recovery
			// will mark it interrupted instead of claiming the user canceled it.
			return err
		}
		_, markErr := s.store.MarkMediaJobFailed(context.Background(), id, err.Error())
		if markErr != nil {
			s.log.Error("mark media job failed", zap.String("job_id", id.String()), zap.Error(markErr))
		}
		return err
	}
	assets := make([]db.Asset, 0, len(job.SourceAssetIDs))
	files := make([]*os.File, 0, len(job.SourceAssetIDs))
	paths := make([]string, 0, len(job.SourceAssetIDs))
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	for _, assetID := range job.SourceAssetIDs {
		asset, err := s.store.GetAsset(ctx, assetID)
		if err != nil {
			return fail(fmt.Errorf("load source asset: %w", err))
		}
		file, _, err := s.storage.Open(ctx, asset.ObjectID)
		if err != nil {
			return fail(fmt.Errorf("open source asset: %w", err))
		}
		assets = append(assets, asset)
		files = append(files, file)
		paths = append(paths, file.Name())
	}
	if err := s.store.UpdateMediaJobProgress(ctx, id, "probing", .08, nil); err != nil {
		return fail(err)
	}
	probe, err := s.tools.Probe(ctx, paths[0])
	if err != nil {
		return fail(err)
	}
	if job.Tool == "inspect" {
		_, err := s.store.MarkMediaJobSucceeded(ctx, id, nil, nil, probe.Raw)
		return err
	}
	processDuration := probe.Duration
	if job.Tool == "merge" {
		for _, path := range paths[1:] {
			nextProbe, probeErr := s.tools.Probe(ctx, path)
			if probeErr != nil {
				return fail(probeErr)
			}
			processDuration += nextProbe.Duration
		}
	}
	var parameters mediaParameters
	if err := json.Unmarshal(job.Parameters, &parameters); err != nil {
		return fail(fmt.Errorf("decode media parameters: %w", err))
	}
	outputSpec, args, cleanup, err := s.buildCommand(job, parameters, paths)
	if err != nil {
		return fail(err)
	}
	defer cleanup()
	outputName := normalizedOutputName(job.OutputName, assets[0].Name, job.Tool, outputSpec.extension)
	outputPath := filepath.Join(s.tempDir, id.String()+outputSpec.extension)
	defer os.Remove(outputPath)
	args = append(args, outputPath)
	displayArgs := redactMediaCommand(args, paths, job.SourceAssetIDs, outputPath, outputName)
	if err := s.store.UpdateMediaJobProgress(ctx, id, "processing", .12, displayArgs); err != nil {
		return fail(err)
	}
	lastUpdate := time.Time{}
	err = s.tools.Run(ctx, args, processDuration, func(progress float64) error {
		now := time.Now()
		if progress < 1 && now.Sub(lastUpdate) < 500*time.Millisecond {
			return nil
		}
		lastUpdate = now
		current, err := s.store.GetMediaJob(ctx, id)
		if err != nil {
			return err
		}
		if current.CancelRequested {
			return errMediaJobCanceled
		}
		return s.store.UpdateMediaJobProgress(ctx, id, "processing", .12+progress*.72, displayArgs)
	})
	if err != nil {
		return fail(err)
	}
	if err := s.store.UpdateMediaJobProgress(ctx, id, "storing", .9, displayArgs); err != nil {
		return fail(err)
	}
	outputFile, err := os.Open(outputPath)
	if err != nil {
		return fail(err)
	}
	defer outputFile.Close()
	info, err := outputFile.Stat()
	if err != nil {
		return fail(err)
	}
	projectID := job.ProjectID
	managed, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{
		ProjectID: &projectID, Purpose: "media-tools", OriginalName: outputName,
		UploadInput: storage.UploadInput{Reader: outputFile, Size: info.Size(), ContentType: outputSpec.mimeType},
	})
	if err != nil {
		return fail(err)
	}
	metadata := db.JSON(map[string]any{
		"derived_from_asset_ids": job.SourceAssetIDs,
		"media_tool":             job.Tool,
		"media_job_id":           job.ID,
		"ffmpeg_version":         s.Status().Version,
	})
	var outputAssetID, outputStagedAssetID *uuid.UUID
	if job.TargetAssetGroupID == nil {
		staged, createErr := s.store.CreateStagedAsset(ctx, db.CreateStagedAssetInput{
			ProjectID: job.ProjectID, ObjectID: managed.Record.ID, Source: "generated", Name: outputName,
			MediaType: outputSpec.mediaType, MimeType: outputSpec.mimeType, FileSizeBytes: info.Size(),
			ProviderCode: "ffmpeg", ModelIdentifier: job.Tool, ParametersSnapshot: job.Parameters,
			InputSnapshot: db.JSON(job.SourceAssetIDs), Metadata: metadata,
		})
		if createErr != nil {
			_ = s.storage.DeleteManaged(ctx, managed.Record.ID)
			return fail(createErr)
		}
		outputStagedAssetID = &staged.ID
	} else {
		asset, createErr := s.store.CreateAsset(ctx, db.CreateAssetInput{
			ProjectID: job.ProjectID, GroupID: job.TargetAssetGroupID, ObjectID: managed.Record.ID,
			Name: outputName, MediaType: outputSpec.mediaType, Source: "generated", Status: "candidate",
			MimeType: outputSpec.mimeType, FileSizeBytes: info.Size(), ProviderCode: "ffmpeg",
			ModelIdentifier: job.Tool, ParametersSnapshot: job.Parameters, InputSnapshot: db.JSON(job.SourceAssetIDs), Metadata: metadata,
		})
		if createErr != nil {
			_ = s.storage.DeleteManaged(ctx, managed.Record.ID)
			return fail(createErr)
		}
		outputAssetID = &asset.ID
	}
	if _, err := s.store.MarkMediaJobSucceeded(ctx, id, outputAssetID, outputStagedAssetID, probe.Raw); err != nil {
		return err
	}
	return nil
}

func (s *MediaToolsService) buildCommand(job db.MediaJob, p mediaParameters, paths []string) (mediaOutput, []string, func(), error) {
	cleanup := func() {}
	input := paths[0]
	switch job.Tool {
	case "transcode":
		switch p.Format {
		case "mp4":
			return mediaOutput{".mp4", "video/mp4", "video"}, append([]string{"-i", input}, h264Args()...), cleanup, nil
		case "webm":
			return mediaOutput{".webm", "video/webm", "video"}, []string{"-i", input, "-c:v", "libvpx-vp9", "-crf", "30", "-b:v", "0", "-c:a", "libopus"}, cleanup, nil
		case "mp3":
			return mediaOutput{".mp3", "audio/mpeg", "audio"}, []string{"-i", input, "-vn", "-c:a", "libmp3lame", "-q:a", "2"}, cleanup, nil
		case "wav":
			return mediaOutput{".wav", "audio/wav", "audio"}, []string{"-i", input, "-vn", "-c:a", "pcm_s16le"}, cleanup, nil
		case "m4a":
			return mediaOutput{".m4a", "audio/mp4", "audio"}, []string{"-i", input, "-vn", "-c:a", "aac", "-b:a", "192k"}, cleanup, nil
		}
	case "audio":
		switch p.AudioMode {
		case "extract":
			return mediaOutput{".mp3", "audio/mpeg", "audio"}, []string{"-i", input, "-vn", "-c:a", "libmp3lame", "-q:a", "2"}, cleanup, nil
		case "normalize":
			return mediaOutput{".wav", "audio/wav", "audio"}, []string{"-i", input, "-vn", "-af", "loudnorm=I=-16:TP=-1.5:LRA=11", "-c:a", "pcm_s16le"}, cleanup, nil
		case "mute":
			return mediaOutput{".mp4", "video/mp4", "video"}, []string{"-i", input, "-an", "-c:v", "copy", "-movflags", "+faststart"}, cleanup, nil
		}
	case "trim":
		args := []string{"-ss", decimal(p.StartSeconds), "-i", input, "-t", decimal(p.Duration)}
		if p.Fast {
			args = append(args, "-c", "copy")
		} else {
			args = append(args, h264Args()...)
		}
		return mediaOutput{".mp4", "video/mp4", "video"}, args, cleanup, nil
	case "merge":
		listFile, err := os.CreateTemp(s.tempDir, "concat-*.txt")
		if err != nil {
			return mediaOutput{}, nil, cleanup, err
		}
		for _, path := range paths {
			escaped := strings.ReplaceAll(filepath.ToSlash(path), "'", `\'`)
			if _, err := fmt.Fprintf(listFile, "file '%s'\n", escaped); err != nil {
				_ = listFile.Close()
				_ = os.Remove(listFile.Name())
				return mediaOutput{}, nil, cleanup, err
			}
		}
		if err := listFile.Close(); err != nil {
			_ = os.Remove(listFile.Name())
			return mediaOutput{}, nil, cleanup, err
		}
		cleanup = func() { _ = os.Remove(listFile.Name()) }
		args := []string{"-f", "concat", "-safe", "0", "-i", listFile.Name()}
		if p.Fast {
			args = append(args, "-c", "copy", "-movflags", "+faststart")
		} else {
			args = append(args, h264Args()...)
		}
		return mediaOutput{".mp4", "video/mp4", "video"}, args, cleanup, nil
	case "screenshot":
		extension, mimeType := ".png", "image/png"
		if p.ImageFormat == "jpg" {
			extension, mimeType = ".jpg", "image/jpeg"
		}
		args := []string{"-ss", decimal(p.TimeSeconds), "-i", input, "-frames:v", "1"}
		filters := make([]string, 0, 2)
		if p.CropWidth > 0 && p.CropHeight > 0 {
			filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", p.CropWidth, p.CropHeight, p.CropX, p.CropY))
		}
		if p.OutputWidth > 0 || p.OutputHeight > 0 {
			width, height := p.OutputWidth, p.OutputHeight
			if width == 0 {
				width = -2
			}
			if height == 0 {
				height = -2
			}
			filters = append(filters, fmt.Sprintf("scale=%d:%d", width, height))
		}
		if len(filters) > 0 {
			args = append(args, "-vf", strings.Join(filters, ","))
		}
		if p.ImageFormat == "jpg" {
			args = append(args, "-q:v", "2")
		}
		return mediaOutput{extension, mimeType, "image"}, args, cleanup, nil
	}
	return mediaOutput{}, nil, cleanup, fmt.Errorf("unsupported media tool %q", job.Tool)
}

func h264Args() []string {
	return []string{"-c:v", "libx264", "-preset", "medium", "-crf", "20", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart"}
}

func decimal(value float64) string {
	return strconv.FormatFloat(math.Max(value, 0), 'f', 3, 64)
}

func normalizedOutputName(requested, source, tool, extension string) string {
	name := strings.TrimSpace(requested)
	if name == "" {
		base := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
		name = base + "-" + tool
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !strings.EqualFold(filepath.Ext(name), extension) {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + extension
	}
	return name
}

func redactMediaCommand(args, paths []string, ids []uuid.UUID, outputPath, outputName string) []string {
	result := make([]string, len(args))
	for index, value := range args {
		result[index] = value
		for sourceIndex, path := range paths {
			if value == path {
				result[index] = "asset:" + ids[sourceIndex].String()
			}
		}
		if value == outputPath {
			result[index] = "output:" + outputName
		}
		if strings.HasSuffix(strings.ToLower(value), ".txt") && strings.Contains(strings.ToLower(value), "concat-") {
			result[index] = "generated:concat-list"
		}
	}
	return result
}
