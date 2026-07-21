package db

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreateGenerationInvocationInput struct {
	JobID            uuid.UUID
	ProviderCode     string
	ModelIdentifier  string
	Capability       string
	CredentialID     *uuid.UUID
	CredentialSource string
	RequestSnapshot  json.RawMessage
}

type CreateGenerationInvocationEventInput struct {
	InvocationID   uuid.UUID
	Sequence       int32
	Stage          string
	Progress       float64
	Message        string
	ProviderJobID  string
	UsageSnapshot  json.RawMessage
	DetailSnapshot json.RawMessage
	ElapsedMS      int64
}

type CompleteGenerationInvocationInput struct {
	ID                   uuid.UUID
	DurationMS           int64
	OutputStagedAssetIDs []uuid.UUID
	OutputAssetIDs       []uuid.UUID
}

type FailGenerationInvocationInput struct {
	ID                   uuid.UUID
	DurationMS           int64
	ErrorKind            string
	ErrorMessage         string
	ErrorStatusCode      int
	Retryable            bool
	OutputStagedAssetIDs []uuid.UUID
	OutputAssetIDs       []uuid.UUID
}

func (s *Store) CreateGenerationInvocation(ctx context.Context, input CreateGenerationInvocationInput) (GenerationInvocation, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO generation_invocations (
			id,job_id,provider_code,model_identifier,capability,credential_id,credential_source,request_snapshot,started_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, input.JobID, strings.TrimSpace(input.ProviderCode),
		strings.TrimSpace(input.ModelIdentifier), input.Capability, input.CredentialID, input.CredentialSource,
		validJSON(input.RequestSnapshot), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return GenerationInvocation{}, err
	}
	return s.GetGenerationInvocation(ctx, id)
}

func (s *Store) GetGenerationInvocation(ctx context.Context, id uuid.UUID) (GenerationInvocation, error) {
	invocation, err := one[GenerationInvocation](s.pool.Query(ctx, `SELECT * FROM generation_invocations WHERE id=$1`, id))
	if err != nil {
		return GenerationInvocation{}, err
	}
	return s.loadGenerationInvocationEvents(ctx, invocation)
}

func (s *Store) GetGenerationInvocationByJob(ctx context.Context, jobID uuid.UUID) (GenerationInvocation, error) {
	invocation, err := one[GenerationInvocation](s.pool.Query(ctx, `SELECT * FROM generation_invocations WHERE job_id=$1`, jobID))
	if err != nil {
		return GenerationInvocation{}, err
	}
	return s.loadGenerationInvocationEvents(ctx, invocation)
}

func (s *Store) loadGenerationInvocationEvents(ctx context.Context, invocation GenerationInvocation) (GenerationInvocation, error) {
	events, err := collectRows[GenerationInvocationEvent](s.pool.Query(ctx, `SELECT * FROM generation_invocation_events WHERE invocation_id=$1 ORDER BY sequence`, invocation.ID))
	if err != nil {
		return GenerationInvocation{}, err
	}
	invocation.Events = events
	return invocation, nil
}

func (s *Store) AppendGenerationInvocationEvent(ctx context.Context, input CreateGenerationInvocationEventInput) error {
	return withTx(ctx, s.pool, func(tx *Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO generation_invocation_events (
				id,invocation_id,sequence,stage,progress,message,provider_job_id,usage_snapshot,detail_snapshot,elapsed_ms
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), input.InvocationID, input.Sequence,
			input.Stage, input.Progress, strings.TrimSpace(input.Message), input.ProviderJobID,
			validJSON(input.UsageSnapshot), validJSON(input.DetailSnapshot), input.ElapsedMS); err != nil {
			return err
		}
		if strings.TrimSpace(input.ProviderJobID) != "" {
			_, err := tx.Exec(ctx, `UPDATE generation_invocations SET provider_job_id=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, input.InvocationID, input.ProviderJobID)
			return err
		}
		return nil
	})
}

func (s *Store) SetGenerationInvocationResult(ctx context.Context, id uuid.UUID, response, usage json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `UPDATE generation_invocations SET response_snapshot=$2,usage_snapshot=$3,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, validJSON(response), validJSON(usage))
	return err
}

func (s *Store) CompleteGenerationInvocation(ctx context.Context, input CompleteGenerationInvocationInput) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_invocations SET status='succeeded',output_staged_asset_ids=$2,output_asset_ids=$3,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),duration_ms=$4,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1`, input.ID, JSON(input.OutputStagedAssetIDs), JSON(input.OutputAssetIDs), input.DurationMS)
	return err
}

func (s *Store) FailGenerationInvocation(ctx context.Context, input FailGenerationInvocationInput) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_invocations SET status='failed',error_kind=$2,error_message=$3,error_status_code=$4,retryable=$5,
		output_staged_asset_ids=$6,output_asset_ids=$7,finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),duration_ms=$8,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, input.ID, input.ErrorKind, input.ErrorMessage,
		input.ErrorStatusCode, input.Retryable, JSON(input.OutputStagedAssetIDs), JSON(input.OutputAssetIDs), input.DurationMS)
	return err
}
