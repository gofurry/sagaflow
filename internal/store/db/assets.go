package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListAssetGroups(ctx context.Context, projectID uuid.UUID) ([]AssetGroup, error) {
	return collectRows[AssetGroup](s.pool.Query(ctx, `SELECT * FROM asset_groups WHERE project_id=$1 ORDER BY kind, sort_order, name`, projectID))
}

func (s *Store) GetAssetGroup(ctx context.Context, id uuid.UUID) (AssetGroup, error) {
	return one[AssetGroup](s.pool.Query(ctx, `SELECT * FROM asset_groups WHERE id=$1`, id))
}

func (s *Store) CreateAssetGroup(ctx context.Context, group AssetGroup) (AssetGroup, error) {
	if group.ID == uuid.Nil {
		group.ID = uuid.New()
	}
	if err := s.validateAssetGroupParent(ctx, s.pool, group); err != nil {
		return AssetGroup{}, err
	}
	return one[AssetGroup](s.pool.Query(ctx, `
		INSERT INTO asset_groups (id, project_id, parent_id, kind, name, description, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *`, group.ID, group.ProjectID, group.ParentID, group.Kind, strings.TrimSpace(group.Name), strings.TrimSpace(group.Description), group.SortOrder))
}

func (s *Store) UpdateAssetGroup(ctx context.Context, group AssetGroup) (AssetGroup, error) {
	var updated AssetGroup
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[AssetGroup](tx.Query(ctx, `SELECT * FROM asset_groups WHERE id=$1`, group.ID))
		if err != nil {
			return err
		}
		group.ProjectID = current.ProjectID
		if err := s.validateAssetGroupParent(ctx, tx, group); err != nil {
			return err
		}
		if group.ParentID != nil {
			var cycle bool
			if err := tx.QueryRow(ctx, `
				WITH RECURSIVE descendants AS (
					SELECT id FROM asset_groups WHERE parent_id=$1
					UNION ALL
					SELECT child.id FROM asset_groups child JOIN descendants parent ON child.parent_id=parent.id
				)
				SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$2)`, group.ID, *group.ParentID).Scan(&cycle); err != nil {
				return err
			}
			if cycle {
				return fmt.Errorf("asset group cannot be moved below its descendant")
			}
		}
		updated, err = one[AssetGroup](tx.Query(ctx, `
			UPDATE asset_groups SET parent_id=$2, kind=$3, name=$4, description=$5, sort_order=$6, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id=$1 RETURNING *`, group.ID, group.ParentID, group.Kind, strings.TrimSpace(group.Name), strings.TrimSpace(group.Description), group.SortOrder))
		return err
	})
	return updated, err
}

type assetGroupQuerier interface {
	QueryRow(context.Context, string, ...any) *sql.Row
}

func (s *Store) validateAssetGroupParent(ctx context.Context, query assetGroupQuerier, group AssetGroup) error {
	if group.ParentID == nil {
		return nil
	}
	if *group.ParentID == group.ID {
		return fmt.Errorf("asset group cannot be its own parent")
	}
	var projectID uuid.UUID
	var kind string
	if err := query.QueryRow(ctx, `SELECT project_id, kind FROM asset_groups WHERE id=$1`, *group.ParentID).Scan(&projectID, &kind); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if projectID != group.ProjectID || kind != group.Kind {
		return fmt.Errorf("parent asset group must belong to the same project and kind")
	}
	return nil
}

func (s *Store) ListAssetGroupSubtreeAssets(ctx context.Context, groupID uuid.UUID) ([]Asset, error) {
	return collectRows[Asset](s.pool.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM asset_groups WHERE id=$1
			UNION ALL
			SELECT child.id FROM asset_groups child JOIN subtree parent ON child.parent_id=parent.id
		)
		SELECT assets.* FROM assets JOIN subtree ON assets.group_id=subtree.id WHERE assets.deleted_at IS NULL
		ORDER BY assets.created_at DESC`, groupID))
}

func (s *Store) DeleteAssetGroup(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM asset_groups WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

type AssetFilter struct {
	ProjectID     uuid.UUID
	GroupID       *uuid.UUID
	EpisodeID     *uuid.UUID
	Status        string
	MediaType     string
	MediaTypes    []string
	ExcludeStatus string
	Name          string
	GroupKind     string
	Ungrouped     bool
	Page          int
	PageSize      int
}

func (s *Store) ListAssets(ctx context.Context, filter AssetFilter) ([]Asset, error) {
	return collectRows[Asset](s.pool.Query(ctx, `
		SELECT * FROM assets
		WHERE project_id=$1
		  AND deleted_at IS NULL
		  AND ($2 IS NULL OR group_id=$2)
		  AND ($3 IS NULL OR episode_id=$3)
		  AND ($4='' OR status=$4)
		  AND ($5='' OR media_type=$5)
		ORDER BY created_at DESC`, filter.ProjectID, filter.GroupID, filter.EpisodeID, filter.Status, filter.MediaType))
}

func (s *Store) ListAssetsPage(ctx context.Context, filter AssetFilter) (AssetPage, error) {
	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize, 24, 100)
	mediaTypes := make([]string, 0, len(filter.MediaTypes))
	for _, mediaType := range filter.MediaTypes {
		mediaType = strings.TrimSpace(mediaType)
		if mediaType != "" && !strings.Contains(mediaType, ",") {
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	mediaTypeList := strings.Join(mediaTypes, ",")
	where := `
		WHERE a.project_id=$1
		  AND a.deleted_at IS NULL
		  AND ($2 IS NULL OR a.group_id=$2)
		  AND ($3 IS NULL OR a.episode_id=$3)
		  AND ($4='' OR a.status=$4)
		  AND ($5='' OR a.media_type=$5)
		  AND ($6='' OR instr(',' || $6 || ',', ',' || a.media_type || ',') > 0)
		  AND ($7='' OR a.status<>$7)
		  AND ($8='' OR lower(a.name) LIKE '%' || lower($8) || '%')
		  AND ($9='' OR g.kind=$9)
		  AND ($10=0 OR a.group_id IS NULL)`
	args := []any{
		filter.ProjectID, filter.GroupID, filter.EpisodeID, strings.TrimSpace(filter.Status),
		strings.TrimSpace(filter.MediaType), mediaTypeList, strings.TrimSpace(filter.ExcludeStatus),
		strings.TrimSpace(filter.Name), strings.TrimSpace(filter.GroupKind), filter.Ungrouped,
	}
	var total int64
	from := ` FROM assets a LEFT JOIN asset_groups g ON g.id=a.group_id`
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return AssetPage{}, err
	}
	items, err := collectRows[Asset](s.pool.Query(ctx,
		`SELECT a.*`+from+where+` ORDER BY a.created_at DESC,a.id DESC LIMIT $11 OFFSET $12`,
		append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)...))
	if err != nil {
		return AssetPage{}, err
	}
	return AssetPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Store) ListAssetsByIDs(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) ([]Asset, error) {
	if len(ids) == 0 {
		return []Asset{}, nil
	}
	if len(ids) > 200 {
		return nil, fmt.Errorf("too many asset ids")
	}
	return collectRows[Asset](s.pool.Query(ctx, `
		SELECT * FROM assets
		WHERE project_id=$1 AND deleted_at IS NULL
		  AND id IN (SELECT value FROM json_each($2))
		ORDER BY created_at DESC`, projectID, JSON(ids)))
}

func (s *Store) AssetSummary(ctx context.Context, projectID uuid.UUID) (AssetSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(g.kind,''),COUNT(*)
		FROM assets a LEFT JOIN asset_groups g ON g.id=a.group_id
		WHERE a.project_id=$1 AND a.deleted_at IS NULL
		GROUP BY COALESCE(g.kind,'')`, projectID)
	if err != nil {
		return AssetSummary{}, err
	}
	defer rows.Close()
	summary := AssetSummary{ByGroupKind: make(map[string]int64)}
	for rows.Next() {
		var kind string
		var count int64
		if err := rows.Scan(&kind, &count); err != nil {
			return AssetSummary{}, err
		}
		summary.Total += count
		if kind != "" {
			summary.ByGroupKind[kind] = count
		}
	}
	if err := rows.Err(); err != nil {
		return AssetSummary{}, err
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM assets
		WHERE project_id=$1 AND deleted_at IS NULL AND group_id IS NULL AND media_type='video'`, projectID).Scan(&summary.UngroupedVideoTotal); err != nil {
		return AssetSummary{}, err
	}
	return summary, nil
}

func (s *Store) GetAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	return one[Asset](s.pool.Query(ctx, `SELECT * FROM assets WHERE id=$1 AND deleted_at IS NULL`, id))
}

type CreateAssetInput struct {
	ID                 uuid.UUID
	ProjectID          uuid.UUID
	GroupID            *uuid.UUID
	StagedAssetID      *uuid.UUID
	EpisodeID          *uuid.UUID
	CanvasNodeID       *uuid.UUID
	ObjectID           uuid.UUID
	Name               string
	MediaType          string
	Source             string
	Status             string
	MimeType           string
	FileSizeBytes      int64
	OriginalURL        string
	ProviderCode       string
	ModelIdentifier    string
	ParametersSnapshot json.RawMessage
	InputSnapshot      json.RawMessage
	Metadata           json.RawMessage
}

func (s *Store) CreateAsset(ctx context.Context, input CreateAssetInput) (Asset, error) {
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
	}
	if input.Source == "" {
		input.Source = "upload"
	}
	if input.Status == "" {
		input.Status = "candidate"
	}
	if input.MimeType == "" {
		input.MimeType = "application/octet-stream"
	}
	input.ParametersSnapshot = validJSON(input.ParametersSnapshot)
	input.InputSnapshot = validJSON(input.InputSnapshot)
	input.Metadata = validJSON(input.Metadata)
	return one[Asset](s.pool.Query(ctx, `
		INSERT INTO assets (id,project_id,group_id,staged_asset_id,episode_id,canvas_node_id,object_id,name,media_type,source,status,mime_type,file_size_bytes,
		 original_url,provider_code,model_identifier,
		 parameters_snapshot,input_snapshot,metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING *`,
		input.ID, input.ProjectID, input.GroupID, input.StagedAssetID, input.EpisodeID, input.CanvasNodeID, input.ObjectID, strings.TrimSpace(input.Name), input.MediaType, input.Source, input.Status, input.MimeType, input.FileSizeBytes,
		input.OriginalURL, input.ProviderCode, input.ModelIdentifier,
		input.ParametersSnapshot, input.InputSnapshot, input.Metadata))
}

func (s *Store) UpdateAssetName(ctx context.Context, id uuid.UUID, name string) (Asset, error) {
	var updated Asset
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		current, err := one[Asset](tx.Query(ctx, `SELECT * FROM assets WHERE id=$1 AND deleted_at IS NULL`, id))
		if err != nil {
			return err
		}
		updated, err = one[Asset](tx.Query(ctx, `UPDATE assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 RETURNING *`, id, strings.TrimSpace(name)))
		if err != nil {
			return err
		}
		if current.StagedAssetID != nil {
			if _, err := tx.Exec(ctx, `UPDATE staged_assets SET name=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1`, *current.StagedAssetID, strings.TrimSpace(name)); err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}

func (s *Store) UpdateAssetStatus(ctx context.Context, id uuid.UUID, status string) (Asset, error) {
	return one[Asset](s.pool.Query(ctx, `UPDATE assets SET status=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 AND deleted_at IS NULL RETURNING *`, id, status))
}

func (s *Store) DeleteAsset(ctx context.Context, id uuid.UUID) (Asset, error) {
	return one[Asset](s.pool.Query(ctx, `UPDATE assets SET deleted_at=strftime('%Y-%m-%dT%H:%M:%fZ','now'),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 AND deleted_at IS NULL RETURNING *`, id))
}

func validJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}

func ValidateAssetGroup(group AssetGroup) error {
	if group.ProjectID == uuid.Nil {
		return fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(group.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch group.Kind {
	case "character", "scene", "prop", "material":
		return nil
	}
	return fmt.Errorf("invalid asset kind")
}
