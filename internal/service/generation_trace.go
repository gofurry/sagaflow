package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type generationTrace struct {
	store    *db.Store
	id       uuid.UUID
	started  time.Time
	sequence atomic.Int32
}

func beginGenerationTrace(ctx context.Context, store *db.Store, job db.GenerationJob, request inference.Request, credential CredentialSecret) (*generationTrace, error) {
	started := time.Now()
	invocation, err := store.CreateGenerationInvocation(ctx, db.CreateGenerationInvocationInput{
		JobID: job.ID, ProviderCode: request.Runtime.ProviderCode, ModelIdentifier: request.Target.ID,
		Capability: string(request.Target.Capability), CredentialID: credential.ID,
		CredentialSource: credential.Source, RequestSnapshot: db.JSON(inference.SnapshotRequest(request)),
	})
	if err != nil {
		return nil, err
	}
	return &generationTrace{store: store, id: invocation.ID, started: started}, nil
}

func (t *generationTrace) event(ctx context.Context, event inference.Event) error {
	if t == nil {
		return nil
	}
	progress := event.Progress
	if progress <= 0 {
		progress = defaultTraceProgress(event.Stage)
	}
	if progress > 1 {
		progress = 1
	}
	return t.store.AppendGenerationInvocationEvent(ctx, db.CreateGenerationInvocationEventInput{
		InvocationID: t.id, Sequence: t.sequence.Add(1), Stage: event.Stage, Progress: progress,
		Message: event.Message, ProviderJobID: event.ProviderRunID,
		UsageSnapshot: db.JSON(event.Usage), DetailSnapshot: db.JSON(inference.SnapshotDetails(event.Details)),
		ElapsedMS: time.Since(t.started).Milliseconds(),
	})
}

func (t *generationTrace) result(ctx context.Context, result inference.Result) error {
	if t == nil {
		return nil
	}
	snapshot := inference.SnapshotResult(result)
	return t.store.SetGenerationInvocationResult(ctx, t.id, db.JSON(snapshot), db.JSON(snapshot.Usage))
}

func (t *generationTrace) complete(ctx context.Context, stagedIDs, assetIDs []uuid.UUID) error {
	if t == nil {
		return nil
	}
	eventErr := t.event(ctx, inference.Event{Stage: "completed", Progress: 1, Message: "generation outputs are ready"})
	completeErr := t.store.CompleteGenerationInvocation(ctx, db.CompleteGenerationInvocationInput{
		ID: t.id, DurationMS: time.Since(t.started).Milliseconds(),
		OutputStagedAssetIDs: stagedIDs, OutputAssetIDs: assetIDs,
	})
	if eventErr != nil {
		return eventErr
	}
	return completeErr
}

func (t *generationTrace) fail(ctx context.Context, err error, stagedIDs, assetIDs []uuid.UUID) error {
	if t == nil || err == nil {
		return nil
	}
	failure := inference.SnapshotError(err)
	_ = t.event(ctx, inference.Event{Stage: "failed", Progress: 1, Message: failure.Message})
	return t.store.FailGenerationInvocation(ctx, db.FailGenerationInvocationInput{
		ID: t.id, DurationMS: time.Since(t.started).Milliseconds(), ErrorKind: string(failure.Kind),
		ErrorMessage: failure.Message, ErrorStatusCode: failure.StatusCode, Retryable: failure.Retryable,
		OutputStagedAssetIDs: stagedIDs, OutputAssetIDs: assetIDs,
	})
}

func defaultTraceProgress(stage string) float64 {
	switch stage {
	case "requesting":
		return .1
	case "generating", "queued":
		return .3
	case "running":
		return .5
	case "fetching":
		return .7
	case "storing":
		return .85
	case "completed", "failed":
		return 1
	default:
		return 0
	}
}
