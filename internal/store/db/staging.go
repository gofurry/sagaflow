package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreateStagedAssetInput struct {
	ID                 uuid.UUID
	JobID              *uuid.UUID
	ProjectID          uuid.UUID
	ObjectID           uuid.UUID
	Source             string
	Name               string
	MediaType          string
	MimeType           string
	FileSizeBytes      int64
	ContentText        string
	OriginalURL        string
	ProviderCode       string
	ModelIdentifier    string
	ParametersSnapshot json.RawMessage
	InputSnapshot      json.RawMessage
	Metadata           json.RawMessage
}

type StagedAssetFilter struct {
	ProjectID   uuid.UUID
	Source      string
	Processed   string
	Capability  string
	MediaType   string
	Name        string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Sort        string
	Page        int
	PageSize    int
}

const stagedAssetSelect = `
	SELECT s.*,
	       a.id AS asset_id,
	       a.group_id AS asset_group_id,
	       COALESCE(a.name, '') AS asset_name,
	       COALESCE(j.status, '') AS job_status,
	       COALESCE(j.capability, '') AS capability,
	       COALESCE(j.prompt, '') AS prompt,
	       COALESCE(j.error_message, '') AS error_message
	FROM staged_assets s
	LEFT JOIN generation_jobs j ON j.id=s.job_id
	LEFT JOIN assets a ON a.staged_asset_id=s.id AND a.deleted_at IS NULL`

func (s *Store) CreateStagedAsset(ctx context.Context, input CreateStagedAssetInput) (StagedAsset, error) {
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
	}
	if input.Source == "" {
		input.Source = "generated"
	}
	if input.MimeType == "" {
		input.MimeType = "application/octet-stream"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO staged_assets (
			id,job_id,project_id,object_id,source,name,media_type,mime_type,file_size_bytes,content_text,
			original_url,
			provider_code,model_identifier,parameters_snapshot,input_snapshot,metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		input.ID, input.JobID, input.ProjectID, input.ObjectID, input.Source, strings.TrimSpace(input.Name), input.MediaType,
		input.MimeType, input.FileSizeBytes, input.ContentText, input.OriginalURL, input.ProviderCode, input.ModelIdentifier,
		validJSON(input.ParametersSnapshot), validJSON(input.InputSnapshot), validJSON(input.Metadata))
	if err != nil {
		return StagedAsset{}, err
	}
	return s.GetStagedAsset(ctx, input.ID)
}

func (s *Store) GetStagedAsset(ctx context.Context, id uuid.UUID) (StagedAsset, error) {
	return one[StagedAsset](s.pool.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, id))
}

func (s *Store) ListStagedAssets(ctx context.Context, filter StagedAssetFilter) (StagedAssetPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 24
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	where := `
		WHERE s.project_id=$1
		  AND ($2='' OR s.source=$2)
		  AND ($3='' OR ($3='imported' AND a.id IS NOT NULL) OR ($3='unprocessed' AND a.id IS NULL))
		  AND ($4='' OR j.capability=$4)
		  AND ($5='' OR s.media_type=$5)
		  AND ($6='' OR s.name LIKE '%' || $6 || '%' COLLATE NOCASE OR COALESCE(a.name, '') LIKE '%' || $6 || '%' COLLATE NOCASE)
		  AND ($7 IS NULL OR s.created_at >= $7)
		  AND ($8 IS NULL OR s.created_at <= $8)`
	args := []any{filter.ProjectID, filter.Source, filter.Processed, filter.Capability, filter.MediaType, strings.TrimSpace(filter.Name), filter.CreatedFrom, filter.CreatedTo}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM staged_assets s LEFT JOIN generation_jobs j ON j.id=s.job_id LEFT JOIN assets a ON a.staged_asset_id=s.id AND a.deleted_at IS NULL`+where, args...).Scan(&total); err != nil {
		return StagedAssetPage{}, err
	}
	order := "s.created_at DESC"
	switch filter.Sort {
	case "created_asc":
		order = "s.created_at ASC"
	case "name_asc":
		order = "LOWER(COALESCE(NULLIF(a.name, ''), s.name)) ASC, s.created_at DESC"
	case "name_desc":
		order = "LOWER(COALESCE(NULLIF(a.name, ''), s.name)) DESC, s.created_at DESC"
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	items, err := collectRows[StagedAsset](s.pool.Query(ctx, stagedAssetSelect+where+` ORDER BY `+order+` LIMIT $9 OFFSET $10`, args...))
	if err != nil {
		return StagedAssetPage{}, err
	}
	return StagedAssetPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Store) StagedAssetSummary(ctx context.Context, projectID uuid.UUID) (StagedAssetSummary, error) {
	summary := StagedAssetSummary{ByCapability: map[string]int64{}}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE a.id IS NULL),
		       COUNT(*) FILTER (WHERE a.id IS NOT NULL)
		FROM staged_assets s
		LEFT JOIN assets a ON a.staged_asset_id=s.id AND a.deleted_at IS NULL
		WHERE s.project_id=$1`, projectID).Scan(&summary.Total, &summary.Unprocessed, &summary.Imported); err != nil {
		return StagedAssetSummary{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT j.capability, COUNT(*)
		FROM staged_assets s
		JOIN generation_jobs j ON j.id=s.job_id
		WHERE s.project_id=$1 AND s.source='generated'
		GROUP BY j.capability`, projectID)
	if err != nil {
		return StagedAssetSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var capability string
		var count int64
		if err := rows.Scan(&capability, &count); err != nil {
			return StagedAssetSummary{}, err
		}
		summary.ByCapability[capability] = count
	}
	return summary, rows.Err()
}

func (s *Store) ListProjectStagedAssets(ctx context.Context, projectID uuid.UUID) ([]StagedAsset, error) {
	return collectRows[StagedAsset](s.pool.Query(ctx, stagedAssetSelect+` WHERE s.project_id=$1 ORDER BY s.created_at DESC`, projectID))
}

func (s *Store) UpdateStagedAssetName(ctx context.Context, id uuid.UUID, name string) (StagedAsset, error) {
	var updated StagedAsset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[StagedAsset](tx.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, id))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE staged_assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, id, strings.TrimSpace(name)); err != nil {
			return err
		}
		if current.AssetID != nil {
			if _, err := tx.Exec(ctx, `UPDATE assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, *current.AssetID, strings.TrimSpace(name)); err != nil {
				return err
			}
		}
		updated, err = one[StagedAsset](tx.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, id))
		return err
	})
	return updated, err
}

func (s *Store) ImportStagedAsset(ctx context.Context, stagedID, groupID uuid.UUID, name string) (Asset, error) {
	var created Asset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		staged, err := one[StagedAsset](tx.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, stagedID))
		if err != nil {
			return err
		}
		if staged.AssetID != nil {
			return fmt.Errorf("%w: staged asset already belongs to an asset group", ErrConflict)
		}
		group, err := one[AssetGroup](tx.Query(ctx, `SELECT * FROM asset_groups WHERE id=$1`, groupID))
		if err != nil {
			return err
		}
		if group.ProjectID != staged.ProjectID {
			return fmt.Errorf("%w: staged asset and asset group must belong to the same project", ErrConflict)
		}
		assetName := strings.TrimSpace(name)
		if assetName == "" {
			assetName = staged.Name
		}
		created, err = one[Asset](tx.Query(ctx, `
			INSERT INTO assets (
				id,project_id,group_id,staged_asset_id,object_id,name,media_type,source,status,mime_type,file_size_bytes,
				original_url,provider_code,model_identifier,
				parameters_snapshot,input_snapshot,metadata
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'candidate',$9,$10,$11,$12,$13,$14,$15,$16)
			RETURNING *`,
			uuid.New(), staged.ProjectID, groupID, staged.ID, staged.ObjectID, assetName, staged.MediaType, staged.Source, staged.MimeType,
			staged.FileSizeBytes, staged.OriginalURL, staged.ProviderCode, staged.ModelIdentifier,
			validJSON(staged.ParametersSnapshot), validJSON(staged.InputSnapshot), validJSON(staged.Metadata)))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE staged_assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, staged.ID, assetName)
		return err
	})
	return created, err
}

func (s *Store) ImportStagedVideoAsset(ctx context.Context, stagedID, episodeID, canvasNodeID uuid.UUID, name string) (Asset, error) {
	var created Asset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		staged, err := one[StagedAsset](tx.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, stagedID))
		if err != nil {
			return err
		}
		if staged.AssetID != nil {
			return fmt.Errorf("%w: staged asset has already been imported", ErrConflict)
		}
		if staged.MediaType != "video" {
			return fmt.Errorf("%w: only generated videos can be attached to a storyboard shot", ErrConflict)
		}
		var projectID uuid.UUID
		var nodeEpisodeID uuid.UUID
		var nodeType string
		if err := tx.QueryRow(ctx, `
			SELECT e.project_id, n.episode_id, n.node_type
			FROM canvas_nodes n
			JOIN episodes e ON e.id=n.episode_id
			WHERE n.id=$1`, canvasNodeID).Scan(&projectID, &nodeEpisodeID, &nodeType); err != nil {
			return err
		}
		if nodeType != "video" || nodeEpisodeID != episodeID || projectID != staged.ProjectID {
			return fmt.Errorf("%w: storyboard shot and staged video must belong to the same episode", ErrConflict)
		}
		assetName := strings.TrimSpace(name)
		if assetName == "" {
			assetName = staged.Name
		}
		created, err = one[Asset](tx.Query(ctx, `
			INSERT INTO assets (
				id,project_id,group_id,staged_asset_id,episode_id,canvas_node_id,object_id,name,media_type,source,status,mime_type,file_size_bytes,
				original_url,provider_code,model_identifier,
				parameters_snapshot,input_snapshot,metadata
			)
			VALUES ($1,$2,NULL,$3,$4,$5,$6,$7,'video',$8,'candidate',$9,$10,$11,$12,$13,$14,$15,$16)
			RETURNING *`,
			uuid.New(), staged.ProjectID, staged.ID, episodeID, canvasNodeID, staged.ObjectID, assetName, staged.Source, staged.MimeType,
			staged.FileSizeBytes, staged.OriginalURL, staged.ProviderCode, staged.ModelIdentifier,
			validJSON(staged.ParametersSnapshot), validJSON(staged.InputSnapshot), validJSON(staged.Metadata)))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE staged_assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, staged.ID, assetName)
		return err
	})
	return created, err
}

func (s *Store) DeleteStagedAsset(ctx context.Context, id uuid.UUID) (StagedAsset, error) {
	var deleted StagedAsset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[StagedAsset](tx.Query(ctx, stagedAssetSelect+` WHERE s.id=$1`, id))
		if err != nil {
			return err
		}
		if current.AssetID != nil {
			return fmt.Errorf("%w: imported staged asset cannot be deleted", ErrConflict)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM staged_assets WHERE id=$1`, id); err != nil {
			return err
		}
		deleted = current
		return nil
	})
	return deleted, err
}
