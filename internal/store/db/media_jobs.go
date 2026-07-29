package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreateMediaJobInput struct {
	ProjectID          uuid.UUID
	TargetAssetGroupID *uuid.UUID
	Tool               string
	SourceAssetIDs     []uuid.UUID
	OutputName         string
	Parameters         json.RawMessage
}

func (s *Store) CreateMediaJob(ctx context.Context, input CreateMediaJobInput) (MediaJob, error) {
	id := uuid.New()
	if len(input.Parameters) == 0 || !json.Valid(input.Parameters) {
		input.Parameters = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO media_jobs (id,project_id,target_asset_group_id,tool,source_asset_ids,output_name,parameters)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		id, input.ProjectID, input.TargetAssetGroupID, strings.TrimSpace(input.Tool), JSON(input.SourceAssetIDs),
		strings.TrimSpace(input.OutputName), input.Parameters)
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) GetMediaJob(ctx context.Context, id uuid.UUID) (MediaJob, error) {
	return one[MediaJob](s.pool.Query(ctx, `SELECT * FROM media_jobs WHERE id=$1`, id))
}

func (s *Store) ListMediaJobs(ctx context.Context, projectID uuid.UUID, page, pageSize int) (MediaJobPage, error) {
	page, pageSize = normalizePage(page, pageSize, 50, 100)
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM media_jobs WHERE project_id=$1`, projectID).Scan(&total); err != nil {
		return MediaJobPage{}, err
	}
	items, err := collectRows[MediaJob](s.pool.Query(ctx, `
		SELECT * FROM media_jobs WHERE project_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`,
		projectID, pageSize, (page-1)*pageSize))
	if err != nil {
		return MediaJobPage{}, err
	}
	return MediaJobPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Store) ClaimNextMediaJob(ctx context.Context, lease time.Duration) (MediaJob, error) {
	leaseUntil := time.Now().UTC().Add(lease).Format(time.RFC3339Nano)
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE media_jobs SET status='running',stage='preparing',progress=.02,cancel_requested=0,
		started_at=COALESCE(started_at,strftime('%Y-%m-%dT%H:%M:%fZ','now')),lease_until=$1,error_message='',
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=(SELECT id FROM media_jobs WHERE status='queued' AND available_at<=strftime('%Y-%m-%dT%H:%M:%fZ','now') ORDER BY created_at LIMIT 1)
		AND status='queued' RETURNING id`, leaseUntil).Scan(&id)
	if err == sql.ErrNoRows {
		return MediaJob{}, ErrNotFound
	}
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) InterruptRunningMediaJobs(ctx context.Context) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET status='interrupted',stage='interrupted',lease_until=NULL,
		error_message=CASE WHEN error_message='' THEN '应用重启，处理任务已中断' ELSE error_message END,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE status='running'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) UpdateMediaJobProgress(ctx context.Context, id uuid.UUID, stage string, progress float64, command []string) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if command == nil {
		command = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET stage=$2,progress=$3,command_snapshot=$4,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`,
		id, stage, progress, JSON(command))
	return err
}

func (s *Store) MarkMediaJobSucceeded(ctx context.Context, id uuid.UUID, outputAssetID, outputStagedAssetID *uuid.UUID, probe json.RawMessage) (MediaJob, error) {
	if len(probe) == 0 || !json.Valid(probe) {
		probe = json.RawMessage(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET status='succeeded',stage='completed',progress=1,output_asset_id=$2,output_staged_asset_id=$3,probe_snapshot=$4,
		lease_until=NULL,finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1`, id, outputAssetID, outputStagedAssetID, probe)
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) MarkMediaJobFailed(ctx context.Context, id uuid.UUID, message string) (MediaJob, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET status='failed',stage='failed',lease_until=NULL,error_message=$2,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1`, id, strings.TrimSpace(message))
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) MarkMediaJobCanceled(ctx context.Context, id uuid.UUID) (MediaJob, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET status='canceled',stage='canceled',lease_until=NULL,cancel_requested=1,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 AND status IN ('queued','running')`, id)
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) RequestMediaJobCancel(ctx context.Context, id uuid.UUID) (MediaJob, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE media_jobs SET cancel_requested=1,
		status=CASE WHEN status='queued' THEN 'canceled' ELSE status END,
		stage=CASE WHEN status='queued' THEN 'canceled' ELSE stage END,
		finished_at=CASE WHEN status='queued' THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE finished_at END,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 AND status IN ('queued','running')`, id)
	if err != nil {
		return MediaJob{}, err
	}
	return s.GetMediaJob(ctx, id)
}

func (s *Store) DeleteCompletedMediaJobs(ctx context.Context, projectID uuid.UUID) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM media_jobs WHERE project_id=$1 AND status IN ('succeeded','failed','canceled','interrupted')`, projectID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
