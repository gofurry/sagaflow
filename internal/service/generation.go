package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/media"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	maxTextArtifactSize = int64(32 << 20)
	maxArtifactSize     = int64(512 << 20)
)

type GenerationService struct {
	store       *db.Store
	credentials *CredentialService
	storage     *storage.Manager
	remote      *StorageService
	executor    inference.Executor
	log         *zap.Logger
}

type generationReference struct {
	ID             uuid.UUID
	ObjectID       uuid.UUID
	Name           string
	MediaType      string
	MIMEType       string
	RemoteExportID *uuid.UUID
}

func NewGenerationService(store *db.Store, credentials *CredentialService, objectStore *storage.Manager, remote *StorageService, executor inference.Executor, log *zap.Logger) (*GenerationService, error) {
	if store == nil || credentials == nil || objectStore == nil || executor == nil {
		return nil, fmt.Errorf("generation service dependencies are required")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &GenerationService{store: store, credentials: credentials, storage: objectStore, remote: remote, executor: executor, log: log}, nil
}

func (s *GenerationService) Execute(ctx context.Context, jobID uuid.UUID) (err error) {
	job, err := s.store.MarkGenerationRunning(ctx, jobID)
	if err != nil {
		return mapStoreError(err)
	}
	var trace *generationTrace
	stagedIDs := make([]uuid.UUID, 0)
	assetIDs := make([]uuid.UUID, 0)
	defer func() {
		if err == nil {
			return
		}
		markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if traceErr := trace.fail(markCtx, err, stagedIDs, assetIDs); traceErr != nil {
			s.log.Error("mark generation invocation failed", zap.Error(traceErr))
		}
		if _, markErr := s.store.MarkGenerationFailed(markCtx, jobID, err.Error()); markErr != nil {
			s.log.Error("mark generation failed", zap.Error(markErr))
		}
	}()
	provider, target, err := s.resolveExecutionTarget(ctx, job)
	if err != nil {
		return err
	}
	secret, err := s.credentials.SecretForProvider(ctx, provider)
	if err != nil {
		return err
	}
	inputs, err := s.loadInputReferences(ctx, job, provider.AdapterCode)
	if err != nil {
		return err
	}
	parameters := map[string]any{}
	if len(job.Parameters) > 0 {
		if err := json.Unmarshal(job.Parameters, &parameters); err != nil {
			return fmt.Errorf("%w: decode generation parameters: %v", ErrInvalidInput, err)
		}
	}
	if err := s.store.UpdateGenerationStage(ctx, job.ID, "generating"); err != nil {
		return err
	}
	request := inference.Request{
		ID:      job.ID.String(),
		Runtime: inference.Runtime{ProviderCode: provider.Code, AdapterCode: provider.AdapterCode, Endpoint: secret.BaseURL, APIKey: secret.APIKey, Configuration: provider.Metadata},
		Target:  target,
		Prompt:  job.Prompt, Parameters: parameters, Inputs: inputs, ProviderRunID: job.ProviderJobID,
	}
	trace, err = beginGenerationTrace(ctx, s.store, job, request, secret)
	if err != nil {
		return fmt.Errorf("begin generation invocation: %w", err)
	}
	if err := trace.event(ctx, inference.Event{Stage: "requesting", Message: "provider request prepared"}); err != nil {
		return fmt.Errorf("record generation request: %w", err)
	}
	if err := trace.event(ctx, inference.Event{Stage: "generating", Message: "provider execution started"}); err != nil {
		return fmt.Errorf("record generation execution: %w", err)
	}
	result, err := s.executor.Execute(ctx, request, func(eventCtx context.Context, event inference.Event) error {
		if traceErr := trace.event(eventCtx, event); traceErr != nil {
			return traceErr
		}
		if event.ProviderRunID != "" && event.ProviderRunID != job.ProviderJobID {
			if err := s.store.SetGenerationProviderJob(eventCtx, job.ID, event.ProviderRunID); err != nil {
				return err
			}
			job.ProviderJobID = event.ProviderRunID
		}
		if event.Stage == "fetching" {
			return s.store.UpdateGenerationStage(eventCtx, job.ID, "fetching")
		}
		return nil
	})
	if err != nil {
		return mapInferenceError(err)
	}
	if err := trace.result(ctx, result); err != nil {
		return fmt.Errorf("record generation result: %w", err)
	}
	if err := s.store.UpdateGenerationStage(ctx, job.ID, "storing"); err != nil {
		return err
	}
	if err := trace.event(ctx, inference.Event{Stage: "storing", Message: "persisting generated artifacts"}); err != nil {
		return fmt.Errorf("record generation storage: %w", err)
	}
	stagedIDs = make([]uuid.UUID, 0, len(result.Artifacts))
	assetIDs = make([]uuid.UUID, 0, len(result.Artifacts))
	for index, artifact := range result.Artifacts {
		name := generationOutputName(job, artifact.MediaType, index, len(result.Artifacts))
		managed, mimeType, size, contentText, uploadErr := s.storeArtifact(ctx, job, index, artifact)
		if uploadErr != nil {
			return uploadErr
		}
		metadata := artifact.Metadata
		if len(metadata) == 0 {
			metadata = json.RawMessage(`{}`)
		}
		staged, createErr := s.store.CreateStagedAsset(ctx, db.CreateStagedAssetInput{
			JobID: &job.ID, ProjectID: job.ProjectID, ObjectID: managed.Record.ID, Source: "generated", Name: name, MediaType: artifact.MediaType,
			MimeType: mimeType, FileSizeBytes: size, ContentText: contentText,
			OriginalURL: artifact.SourceURL, ProviderCode: provider.Code,
			ModelIdentifier: target.ID, ParametersSnapshot: job.Parameters, InputSnapshot: job.InputSnapshot,
			Metadata: metadata,
		})
		if createErr != nil {
			_ = s.storage.DeleteManaged(ctx, managed.Record.ID)
			return createErr
		}
		stagedIDs = append(stagedIDs, staged.ID)
		if target.Capability == inference.CapabilityVideo && job.EpisodeID != nil && job.CanvasNodeID != nil {
			asset, importErr := s.store.ImportStagedVideoAsset(ctx, staged.ID, *job.EpisodeID, *job.CanvasNodeID, staged.Name)
			if importErr != nil {
				return importErr
			}
			assetIDs = append(assetIDs, asset.ID)
		}
		progress := .85 + (.1 * float64(index+1) / float64(len(result.Artifacts)))
		if err := trace.event(ctx, inference.Event{Stage: "storing", Progress: progress, Message: fmt.Sprintf("artifact %d/%d stored", index+1, len(result.Artifacts))}); err != nil {
			return fmt.Errorf("record stored artifact: %w", err)
		}
	}
	_, err = s.store.MarkGenerationSucceeded(ctx, jobID, stagedIDs)
	if err == nil {
		if traceErr := trace.complete(ctx, stagedIDs, assetIDs); traceErr != nil {
			s.log.Error("complete generation invocation", zap.Error(traceErr))
		}
		s.log.Info("generation completed", zap.String("job_id", jobID.String()), zap.String("capability", string(target.Capability)), zap.Int("results", len(stagedIDs)))
	}
	return err
}

func (s *GenerationService) resolveExecutionTarget(ctx context.Context, job db.GenerationJob) (db.ModelProvider, inference.Target, error) {
	switch job.TargetKind {
	case "model":
		if job.ModelID == nil {
			return db.ModelProvider{}, inference.Target{}, fmt.Errorf("%w: generation model target is missing", ErrInvalidInput)
		}
		model, err := s.store.GetModel(ctx, *job.ModelID)
		if err != nil {
			return db.ModelProvider{}, inference.Target{}, mapStoreError(err)
		}
		provider, err := s.store.GetModelProvider(ctx, model.ProviderID)
		if err != nil {
			return db.ModelProvider{}, inference.Target{}, mapStoreError(err)
		}
		return provider, inference.Target{
			Kind: inference.TargetModel, ID: model.ModelID,
			Capability: inference.Capability(model.Capability), Spec: model.Metadata,
		}, nil
	case "workflow":
		if job.ProviderID == nil || job.WorkflowTemplateID == nil {
			return db.ModelProvider{}, inference.Target{}, fmt.Errorf("%w: generation workflow target is missing", ErrInvalidInput)
		}
		provider, err := s.store.GetModelProvider(ctx, *job.ProviderID)
		if err != nil {
			return db.ModelProvider{}, inference.Target{}, mapStoreError(err)
		}
		code := snapshotIdentifier(job.TargetSnapshot, "code")
		version := snapshotNumber(job.TargetSnapshot, "version")
		identifier := fmt.Sprintf("%s@%d", code, version)
		return provider, inference.Target{
			Kind: inference.TargetWorkflow, ID: identifier, Capability: inference.Capability(job.Capability), Spec: job.TargetSnapshot,
		}, nil
	default:
		return db.ModelProvider{}, inference.Target{}, fmt.Errorf("%w: unknown generation target kind %q", ErrInvalidInput, job.TargetKind)
	}
}

func snapshotIdentifier(raw json.RawMessage, key string) string {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func snapshotNumber(raw json.RawMessage, key string) int64 {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return 0
	}
	value, _ := values[key].(float64)
	return int64(value)
}

func (s *GenerationService) loadInputReferences(ctx context.Context, job db.GenerationJob, adapterCode string) ([]inference.Input, error) {
	var references []db.GenerationInputReference
	if err := json.Unmarshal(job.InputReferences, &references); err != nil {
		return nil, fmt.Errorf("decode generation references: %w", err)
	}
	inputs := make([]inference.Input, 0, len(references))
	for _, item := range references {
		var reference generationReference
		switch item.Source {
		case "asset":
			asset, err := s.store.GetAsset(ctx, item.ID)
			if err != nil {
				return nil, mapStoreError(err)
			}
			if item.RemoteExportID != nil {
				exported, err := s.store.GetAssetRemoteExport(ctx, *item.RemoteExportID)
				if err != nil {
					return nil, mapStoreError(err)
				}
				if exported.AssetID != asset.ID || exported.State != "ready" {
					return nil, fmt.Errorf("%w: remote export does not belong to the referenced asset", ErrInvalidInput)
				}
			}
			reference = generationReference{
				ID: asset.ID, ObjectID: asset.ObjectID, Name: asset.Name, MediaType: asset.MediaType, MIMEType: asset.MimeType, RemoteExportID: item.RemoteExportID,
			}
		case "upload":
			upload, err := s.store.GetGenerationReferenceUpload(ctx, item.ID)
			if err != nil {
				return nil, mapStoreError(err)
			}
			if upload.JobID == nil || *upload.JobID != job.ID {
				return nil, fmt.Errorf("%w: temporary reference is not bound to this job", ErrInvalidInput)
			}
			reference = generationReference{
				ID: upload.ID, ObjectID: upload.ObjectID, Name: upload.Name, MediaType: upload.MediaType, MIMEType: upload.MimeType,
			}
		default:
			return nil, fmt.Errorf("%w: unsupported reference source %q", ErrInvalidInput, item.Source)
		}
		if providerRequiresRemoteInput(adapterCode) && reference.RemoteExportID == nil {
			return nil, fmt.Errorf("%w: cloud model reference %s must first be published to an S3 connection", ErrInvalidInput, reference.ID)
		}
		url, err := s.referenceURL(ctx, reference)
		if err != nil {
			return nil, err
		}
		object, err := s.referenceObject(ctx, reference)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, inference.Input{
			ID: reference.ID.String(), Name: reference.Name, MediaType: reference.MediaType,
			MIMEType: reference.MIMEType, URL: url,
			Content: storedContent{store: s.storage, object: object, mimeType: reference.MIMEType},
		})
	}
	return inputs, nil
}

func providerRequiresRemoteInput(adapterCode string) bool {
	return adapterCode != ProviderOllama &&
		adapterCode != ProviderComfyUI &&
		adapterCode != ProviderSiliconFlow &&
		adapterCode != ProviderZhipu &&
		adapterCode != ProviderTencentTokenHub &&
		adapterCode != ProviderMoonshot
}

func (s *GenerationService) referenceURL(ctx context.Context, reference generationReference) (string, error) {
	if reference.RemoteExportID == nil {
		return "", nil
	}
	if s.remote == nil {
		return "", fmt.Errorf("remote storage publisher is unavailable")
	}
	signed, err := s.remote.ProviderURL(ctx, *reference.RemoteExportID, 24*time.Hour)
	if err != nil {
		return "", fmt.Errorf("presign generation reference %s: %w", reference.ID, err)
	}
	return signed.URL, nil
}

func (s *GenerationService) referenceObject(ctx context.Context, reference generationReference) (storage.Object, error) {
	return s.storage.Object(ctx, reference.ObjectID)
}

func (s *GenerationService) storeArtifact(ctx context.Context, job db.GenerationJob, index int, artifact inference.Artifact) (storage.ManagedObject, string, int64, string, error) {
	reader, info, err := artifact.Content.Open(ctx)
	if err != nil {
		return storage.ManagedObject{}, "", 0, "", err
	}
	defer reader.Close()
	mimeType := strings.TrimSpace(info.MIMEType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(artifact.MIMEType)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	key := generationObjectKey(job.ID, index, mimeType)
	projectID := job.ProjectID
	if artifact.MediaType == "text" {
		data, err := io.ReadAll(io.LimitReader(reader, maxTextArtifactSize+1))
		if err != nil {
			return storage.ManagedObject{}, "", 0, "", err
		}
		if int64(len(data)) > maxTextArtifactSize {
			return storage.ManagedObject{}, "", 0, "", fmt.Errorf("text artifact exceeds size limit")
		}
		object, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{ProjectID: &projectID, Purpose: "generation", OriginalName: filepath.Base(key), UploadInput: storage.UploadInput{Data: data, ContentType: mimeType}})
		return object, mimeType, int64(len(data)), string(data), err
	}
	sizedReader, size, cleanup, err := sizedArtifactReader(ctx, reader, info.Size)
	if err != nil {
		return storage.ManagedObject{}, "", 0, "", err
	}
	defer cleanup()
	counter := &countingReader{reader: sizedReader}
	object, err := s.storage.UploadManaged(ctx, storage.ManagedUploadInput{ProjectID: &projectID, Purpose: "generation", OriginalName: filepath.Base(key), UploadInput: storage.UploadInput{Reader: counter, Size: size, ContentType: mimeType}})
	if err != nil {
		return storage.ManagedObject{}, "", 0, "", err
	}
	if counter.count != size {
		_ = s.storage.DeleteManaged(ctx, object.Record.ID)
		return storage.ManagedObject{}, "", 0, "", fmt.Errorf("artifact size changed while uploading: expected %d, copied %d", size, counter.count)
	}
	return object, mimeType, counter.count, "", nil
}

type storedContent struct {
	store    *storage.Manager
	object   storage.Object
	mimeType string
}

func (s storedContent) Open(ctx context.Context) (io.ReadCloser, inference.ContentInfo, error) {
	data, err := s.store.Read(ctx, s.object)
	if err != nil {
		return nil, inference.ContentInfo{}, err
	}
	return io.NopCloser(bytes.NewReader(data)), inference.ContentInfo{MIMEType: s.mimeType, Size: int64(len(data))}, nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}

func sizedArtifactReader(ctx context.Context, reader io.Reader, size int64) (io.Reader, int64, func(), error) {
	if size >= 0 {
		if size > maxArtifactSize {
			return nil, 0, func() {}, fmt.Errorf("artifact exceeds size limit")
		}
		return reader, size, func() {}, nil
	}
	file, err := os.CreateTemp("", "sagaflow-artifact-*")
	if err != nil {
		return nil, 0, func() {}, err
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	written, err := io.Copy(file, io.LimitReader(&contextReader{ctx: ctx, reader: reader}, maxArtifactSize+1))
	if err != nil {
		cleanup()
		return nil, 0, func() {}, err
	}
	if written > maxArtifactSize {
		cleanup()
		return nil, 0, func() {}, fmt.Errorf("artifact exceeds size limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, 0, func() {}, err
	}
	return file, written, cleanup, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func mapInferenceError(err error) error {
	if inference.IsKind(err, inference.ErrorInvalidRequest) {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return fmt.Errorf("%w: %w", ErrProviderRequest, err)
}

func generationObjectKey(jobID uuid.UUID, index int, mimeType string) string {
	suffix := media.ExtensionForMIME(mimeType)
	return filepath.ToSlash(filepath.Join("generated", jobID.String(), fmt.Sprintf("%02d%s", index+1, suffix)))
}

func generationOutputName(job db.GenerationJob, mediaType string, index, total int) string {
	base := strings.TrimSpace(job.OutputName)
	if base == "" {
		label := map[string]string{"text": "文本", "image": "图像", "audio": "音频", "video": "视频", "file": "文件"}[mediaType]
		if label == "" {
			label = "素材"
		}
		base = fmt.Sprintf("未命名%s · %s", label, job.CreatedAt.Local().Format("01-02 15:04"))
	}
	if total > 1 {
		return fmt.Sprintf("%s %02d", base, index+1)
	}
	return base
}
