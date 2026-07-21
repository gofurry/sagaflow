package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const generationSelect = `
	SELECT j.*,p.code AS provider_code,p.base_url AS provider_base_url,
		CASE WHEN j.target_kind='workflow'
			THEN COALESCE(json_extract(j.target_snapshot,'$.code'),w.code) || '@' || COALESCE(CAST(json_extract(j.target_snapshot,'$.version') AS TEXT),CAST(w.version AS TEXT))
			ELSE m.model_id END AS model_identifier
	FROM generation_jobs j
	LEFT JOIN model_catalog m ON m.id=j.model_id
	LEFT JOIN workflow_templates w ON w.id=j.workflow_template_id
	JOIN model_providers p ON p.id=COALESCE(j.provider_id,m.provider_id)`

type CreateGenerationJobInput struct {
	ProjectID          uuid.UUID
	EpisodeID          *uuid.UUID
	CanvasNodeID       *uuid.UUID
	TargetAssetGroupID *uuid.UUID
	PromptPresetID     *uuid.UUID
	ModelPresetID      *uuid.UUID
	TargetKind         string
	ProviderID         *uuid.UUID
	ModelID            *uuid.UUID
	WorkflowTemplateID *uuid.UUID
	TargetSnapshot     json.RawMessage
	Capability         string
	Prompt             string
	Parameters         json.RawMessage
	InputReferences    []GenerationInputReference
	InputSnapshot      json.RawMessage
	OutputName         string
}

func (s *Store) CreateGenerationJob(ctx context.Context, input CreateGenerationJobInput) (GenerationJob, error) {
	id := uuid.New()
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO generation_jobs (
				id,project_id,episode_id,canvas_node_id,target_asset_group_id,prompt_preset_id,model_preset_id,
				target_kind,provider_id,model_id,workflow_template_id,target_snapshot,
				capability,prompt,parameters,input_references,input_snapshot,output_name
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			id, input.ProjectID, input.EpisodeID, input.CanvasNodeID, input.TargetAssetGroupID, input.PromptPresetID, input.ModelPresetID,
			input.TargetKind, input.ProviderID, input.ModelID, input.WorkflowTemplateID, validJSON(input.TargetSnapshot), input.Capability,
			input.Prompt, validJSON(input.Parameters), JSON(input.InputReferences), validJSON(input.InputSnapshot), strings.TrimSpace(input.OutputName))
		if err != nil {
			return err
		}
		for _, reference := range input.InputReferences {
			if reference.Source != "upload" {
				continue
			}
			result, err := tx.Exec(ctx, `
				UPDATE generation_reference_uploads SET job_id=$1
				WHERE id=$2 AND project_id=$3 AND job_id IS NULL`, id, reference.ID, input.ProjectID)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				return fmt.Errorf("%w: temporary reference is already used or unavailable", ErrConflict)
			}
		}
		return nil
	})
	if err != nil {
		return GenerationJob{}, err
	}
	return s.GetGenerationJob(ctx, id)
}

func (s *Store) GetGenerationJob(ctx context.Context, id uuid.UUID) (GenerationJob, error) {
	return one[GenerationJob](s.pool.Query(ctx, generationSelect+` WHERE j.id=$1`, id))
}

func (s *Store) ListGenerationJobs(ctx context.Context, projectID uuid.UUID, episodeID *uuid.UUID) ([]GenerationJob, error) {
	return collectRows[GenerationJob](s.pool.Query(ctx, generationSelect+` WHERE j.project_id=$1 AND ($2 IS NULL OR j.episode_id=$2) ORDER BY j.created_at DESC LIMIT 100`, projectID, episodeID))
}

func (s *Store) ClaimNextGenerationJob(ctx context.Context, lease time.Duration) (GenerationJob, error) {
	leaseUntil := time.Now().UTC().Add(lease).Format(time.RFC3339Nano)
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE generation_jobs SET status='running',stage='requesting',progress=0,attempt=attempt+1,
		started_at=COALESCE(started_at,strftime('%Y-%m-%dT%H:%M:%fZ','now')),lease_until=$1,error_message='',
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=(SELECT id FROM generation_jobs WHERE status='queued' AND available_at<=strftime('%Y-%m-%dT%H:%M:%fZ','now') ORDER BY created_at LIMIT 1)
		AND status='queued' RETURNING id`, leaseUntil).Scan(&id)
	if err == sql.ErrNoRows {
		return GenerationJob{}, ErrNotFound
	}
	if err != nil {
		return GenerationJob{}, err
	}
	return s.GetGenerationJob(ctx, id)
}

func (s *Store) InterruptRunningJobs(ctx context.Context) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE generation_jobs SET status='interrupted',stage='interrupted',lease_until=NULL,
		error_message=CASE WHEN error_message='' THEN '应用重启，任务状态需要人工确认' ELSE error_message END,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE status='running'`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) MarkGenerationRunning(ctx context.Context, id uuid.UUID) (GenerationJob, error) {
	result, err := s.pool.Exec(ctx, `UPDATE generation_jobs SET status='running',stage='requesting',started_at=COALESCE(started_at,strftime('%Y-%m-%dT%H:%M:%fZ','now')),error_message='' WHERE id=$1`, id)
	if err != nil {
		return GenerationJob{}, err
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		return GenerationJob{}, ErrNotFound
	}
	return s.GetGenerationJob(ctx, id)
}

func (s *Store) UpdateGenerationStage(ctx context.Context, id uuid.UUID, stage string) error {
	progress := map[string]float64{"requesting": .15, "generating": .35, "fetching": .65, "storing": .85, "completed": 1}[stage]
	_, err := s.pool.Exec(ctx, `UPDATE generation_jobs SET stage=$2,progress=$3,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, stage, progress)
	return err
}

func (s *Store) SetGenerationProviderJob(ctx context.Context, id uuid.UUID, providerJobID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE generation_jobs SET provider_job_id=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, providerJobID)
	return err
}

func (s *Store) MarkGenerationSucceeded(ctx context.Context, id uuid.UUID, resultIDs []uuid.UUID) (GenerationJob, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_jobs SET status='succeeded',stage='completed',progress=1,output_staged_asset_ids=$2,
		lease_until=NULL,finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, JSON(resultIDs))
	if err != nil {
		return GenerationJob{}, err
	}
	return s.GetGenerationJob(ctx, id)
}

func (s *Store) MarkGenerationFailed(ctx context.Context, id uuid.UUID, message string) (GenerationJob, error) {
	_, err := s.pool.Exec(ctx, `
		UPDATE generation_jobs SET status='failed',stage='failed',lease_until=NULL,error_message=$2,
		finished_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, message)
	if err != nil {
		return GenerationJob{}, err
	}
	return s.GetGenerationJob(ctx, id)
}
