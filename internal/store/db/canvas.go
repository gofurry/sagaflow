package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) GetCanvas(ctx context.Context, episodeID uuid.UUID) (Canvas, error) {
	nodes, err := collectRows[CanvasNode](s.pool.Query(ctx, `SELECT * FROM canvas_nodes WHERE episode_id=$1 ORDER BY z_index, created_at`, episodeID))
	if err != nil {
		return Canvas{}, err
	}
	edges, err := collectRows[CanvasEdge](s.pool.Query(ctx, `SELECT * FROM canvas_edges WHERE episode_id=$1 ORDER BY created_at`, episodeID))
	if err != nil {
		return Canvas{}, err
	}
	annotations, err := collectRows[CanvasAnnotation](s.pool.Query(ctx, `SELECT * FROM canvas_annotations WHERE episode_id=$1 ORDER BY z_index, created_at`, episodeID))
	if err != nil {
		return Canvas{}, err
	}
	if nodes == nil {
		nodes = []CanvasNode{}
	}
	if edges == nil {
		edges = []CanvasEdge{}
	}
	if annotations == nil {
		annotations = []CanvasAnnotation{}
	}
	return Canvas{Nodes: nodes, Edges: edges, Annotations: annotations}, nil
}

func (s *Store) GetCanvasNode(ctx context.Context, id uuid.UUID) (CanvasNode, error) {
	return one[CanvasNode](s.pool.Query(ctx, `SELECT * FROM canvas_nodes WHERE id=$1`, id))
}

func (s *Store) ListCanvasReferenceAssetIDs(ctx context.Context, videoNodeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT source.asset_id
		FROM canvas_edges edge
		JOIN canvas_nodes source ON source.id=edge.source_node_id
		JOIN canvas_nodes target ON target.id=edge.target_node_id
		WHERE target.id=$1 AND target.node_type='video' AND source.node_type='asset' AND edge.edge_type='reference'
		ORDER BY edge.created_at`, videoNodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SelectCanvasVideoAsset(ctx context.Context, videoNodeID uuid.UUID, assetID *uuid.UUID) (CanvasNode, error) {
	var updated CanvasNode
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		node, err := one[CanvasNode](tx.Query(ctx, `SELECT * FROM canvas_nodes WHERE id=$1`, videoNodeID))
		if err != nil {
			return err
		}
		if node.NodeType != "video" {
			return fmt.Errorf("selected canvas node is not a video shot")
		}
		if assetID != nil {
			var episodeID, canvasNodeID *uuid.UUID
			var mediaType string
			if err := tx.QueryRow(ctx, `SELECT episode_id,canvas_node_id,media_type FROM assets WHERE id=$1`, *assetID).Scan(&episodeID, &canvasNodeID, &mediaType); err != nil {
				return err
			}
			if mediaType != "video" || episodeID == nil || *episodeID != node.EpisodeID {
				return fmt.Errorf("%w: video asset and storyboard shot must belong to the same episode", ErrConflict)
			}
			if canvasNodeID != nil && *canvasNodeID != node.ID {
				return fmt.Errorf("%w: video asset already belongs to another storyboard shot", ErrConflict)
			}
			if canvasNodeID == nil {
				result, err := tx.Exec(ctx, `
					UPDATE assets
					SET canvas_node_id=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
					WHERE id=$1 AND canvas_node_id IS NULL`, *assetID, node.ID)
				if err != nil {
					return err
				}
				if changed, _ := result.RowsAffected(); changed != 1 {
					return fmt.Errorf("%w: video asset was bound by another storyboard shot", ErrConflict)
				}
			}
		}
		updated, err = one[CanvasNode](tx.Query(ctx, `UPDATE canvas_nodes SET selected_video_asset_id=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=$1 RETURNING *`, videoNodeID, assetID))
		return err
	})
	return updated, err
}

func (s *Store) SaveCanvas(ctx context.Context, episodeID uuid.UUID, canvas Canvas) error {
	return withTx(ctx, s.pool, func(tx *Tx) error {
		var projectID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT project_id FROM episodes WHERE id=$1`, episodeID).Scan(&projectID); err != nil {
			if err == sql.ErrNoRows {
				return ErrNotFound
			}
			return err
		}

		nodeTypes := make(map[uuid.UUID]string, len(canvas.Nodes))
		shotNumbers := make(map[int32]struct{})
		videoAssetBindings := make(map[uuid.UUID]uuid.UUID)
		for index := range canvas.Nodes {
			node := &canvas.Nodes[index]
			if node.ID == uuid.Nil {
				return fmt.Errorf("canvas node id is required")
			}
			if _, duplicate := nodeTypes[node.ID]; duplicate {
				return fmt.Errorf("duplicate canvas node id %s", node.ID)
			}
			switch node.NodeType {
			case "asset":
				if node.AssetID == nil {
					return fmt.Errorf("asset canvas node requires asset_id")
				}
				var assetProjectID uuid.UUID
				var assetName string
				if err := tx.QueryRow(ctx, `SELECT project_id,name FROM assets WHERE id=$1`, *node.AssetID).Scan(&assetProjectID, &assetName); err != nil {
					return err
				}
				if assetProjectID != projectID {
					return fmt.Errorf("canvas asset must belong to the episode project")
				}
				node.Title = assetName
				node.ShotNumber = nil
			case "video":
				if node.ShotNumber == nil || *node.ShotNumber < 1 {
					return fmt.Errorf("video canvas node requires a positive shot_number")
				}
				if _, duplicate := shotNumbers[*node.ShotNumber]; duplicate {
					return fmt.Errorf("video shot numbers must be unique within an episode")
				}
				shotNumbers[*node.ShotNumber] = struct{}{}
				if strings.TrimSpace(node.Title) == "" {
					return fmt.Errorf("video canvas node requires a title")
				}
				node.AssetID = nil
			case "note":
				node.AssetID = nil
				node.ShotNumber = nil
				if strings.TrimSpace(node.Title) == "" {
					node.Title = "备注"
				}
			default:
				return fmt.Errorf("unsupported canvas node type %q", node.NodeType)
			}
			if node.Color == "" {
				node.Color = "sand"
			}
			if node.SelectedVideoAssetID != nil {
				if node.NodeType != "video" {
					return fmt.Errorf("only video nodes can select a video asset")
				}
				var assetEpisodeID, assetNodeID *uuid.UUID
				var mediaType string
				if err := tx.QueryRow(ctx, `SELECT episode_id,canvas_node_id,media_type FROM assets WHERE id=$1`, *node.SelectedVideoAssetID).Scan(&assetEpisodeID, &assetNodeID, &mediaType); err != nil {
					return err
				}
				if mediaType != "video" || assetEpisodeID == nil || *assetEpisodeID != episodeID || (assetNodeID != nil && *assetNodeID != node.ID) {
					return fmt.Errorf("selected video asset must belong to the storyboard shot")
				}
				if boundNodeID, duplicate := videoAssetBindings[*node.SelectedVideoAssetID]; duplicate && boundNodeID != node.ID {
					return fmt.Errorf("selected video asset cannot belong to multiple storyboard shots")
				}
				videoAssetBindings[*node.SelectedVideoAssetID] = node.ID
			}
			nodeTypes[node.ID] = node.NodeType
		}

		if _, err := tx.Exec(ctx, `DELETE FROM canvas_edges WHERE episode_id=$1`, episodeID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM canvas_annotations WHERE episode_id=$1`, episodeID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id FROM canvas_nodes WHERE episode_id=$1`, episodeID)
		if err != nil {
			return err
		}
		existingNodeIDs := make([]uuid.UUID, 0)
		for rows.Next() {
			var existingNodeID uuid.UUID
			if err := rows.Scan(&existingNodeID); err != nil {
				rows.Close()
				return err
			}
			existingNodeIDs = append(existingNodeIDs, existingNodeID)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, node := range canvas.Nodes {
			result, err := tx.Exec(ctx, `
				INSERT INTO canvas_nodes (
					id,episode_id,node_type,position_x,position_y,width,height,z_index,data,
					asset_id,title,body,shot_number,target_duration_seconds,selected_video_asset_id,color
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
				ON CONFLICT(id) DO UPDATE SET
					node_type=excluded.node_type,
					position_x=excluded.position_x,
					position_y=excluded.position_y,
					width=excluded.width,
					height=excluded.height,
					z_index=excluded.z_index,
					data=excluded.data,
					asset_id=excluded.asset_id,
					title=excluded.title,
					body=excluded.body,
					shot_number=excluded.shot_number,
					target_duration_seconds=excluded.target_duration_seconds,
					selected_video_asset_id=excluded.selected_video_asset_id,
					color=excluded.color,
					updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
				WHERE canvas_nodes.episode_id=excluded.episode_id`,
				node.ID, episodeID, node.NodeType, node.PositionX, node.PositionY, node.Width, node.Height, node.ZIndex, validJSON(node.Data),
				node.AssetID, strings.TrimSpace(node.Title), node.Body, node.ShotNumber, node.TargetDurationSeconds, node.SelectedVideoAssetID, node.Color)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				return fmt.Errorf("canvas node %s belongs to another episode", node.ID)
			}
		}
		for _, existingNodeID := range existingNodeIDs {
			if _, retained := nodeTypes[existingNodeID]; retained {
				continue
			}
			if _, err := tx.Exec(ctx, `DELETE FROM canvas_nodes WHERE id=$1 AND episode_id=$2`, existingNodeID, episodeID); err != nil {
				return err
			}
		}
		for assetID, nodeID := range videoAssetBindings {
			result, err := tx.Exec(ctx, `UPDATE assets SET canvas_node_id=$2 WHERE id=$1 AND (canvas_node_id IS NULL OR canvas_node_id=$2)`, assetID, nodeID)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed == 0 {
				return fmt.Errorf("selected video asset must belong to the storyboard shot")
			}
		}

		edgeIDs := make(map[uuid.UUID]struct{}, len(canvas.Edges))
		for _, edge := range canvas.Edges {
			if edge.ID == uuid.Nil {
				return fmt.Errorf("canvas edge id is required")
			}
			if _, duplicate := edgeIDs[edge.ID]; duplicate {
				return fmt.Errorf("duplicate canvas edge id %s", edge.ID)
			}
			edgeIDs[edge.ID] = struct{}{}
			sourceType, sourceOK := nodeTypes[edge.SourceNodeID]
			targetType, targetOK := nodeTypes[edge.TargetNodeID]
			if !sourceOK || !targetOK || edge.SourceNodeID == edge.TargetNodeID {
				return fmt.Errorf("canvas edge endpoints are invalid")
			}
			switch edge.EdgeType {
			case "reference":
				if sourceType != "asset" || targetType != "video" {
					return fmt.Errorf("reference edges must connect an asset to a video shot")
				}
			case "annotation":
				if sourceType != "note" && targetType != "note" {
					return fmt.Errorf("annotation edges must connect a note")
				}
			case "relation":
				// A relation is presentation-only and may connect any two different
				// canvas nodes. It does not become a generation reference.
			default:
				return fmt.Errorf("unsupported canvas edge type %q", edge.EdgeType)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvas_edges (id,episode_id,source_node_id,target_node_id,source_handle,target_handle,edge_type,data)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, edge.ID, episodeID, edge.SourceNodeID, edge.TargetNodeID,
				edge.SourceHandle, edge.TargetHandle, edge.EdgeType, validJSON(edge.Data)); err != nil {
				return err
			}
		}

		annotationIDs := make(map[uuid.UUID]struct{}, len(canvas.Annotations))
		for index := range canvas.Annotations {
			annotation := &canvas.Annotations[index]
			if annotation.ID == uuid.Nil {
				return fmt.Errorf("canvas annotation id is required")
			}
			if _, duplicate := annotationIDs[annotation.ID]; duplicate {
				return fmt.Errorf("duplicate canvas annotation id %s", annotation.ID)
			}
			annotationIDs[annotation.ID] = struct{}{}
			if !finite(annotation.PositionX) || !finite(annotation.PositionY) || !finite(annotation.Width) || !finite(annotation.Height) {
				return fmt.Errorf("canvas annotation geometry must be finite")
			}
			switch annotation.AnnotationType {
			case "arrow", "line":
				if math.Hypot(annotation.Width, annotation.Height) < 8 {
					return fmt.Errorf("canvas line annotation is too short")
				}
			case "rectangle", "ellipse":
				if annotation.Width < 8 || annotation.Height < 8 {
					return fmt.Errorf("canvas shape annotation is too small")
				}
			default:
				return fmt.Errorf("unsupported canvas annotation type %q", annotation.AnnotationType)
			}
			if annotation.StrokeColor == "" {
				annotation.StrokeColor = "#c8753f"
			}
			if annotation.StrokeWidth == 0 {
				annotation.StrokeWidth = 2
			}
			if annotation.StrokeWidth < 1 || annotation.StrokeWidth > 8 {
				return fmt.Errorf("canvas annotation stroke width is invalid")
			}
			if annotation.LineStyle == "" {
				annotation.LineStyle = "solid"
			}
			if annotation.LineStyle != "solid" && annotation.LineStyle != "dashed" {
				return fmt.Errorf("canvas annotation line style is invalid")
			}
			if annotation.Opacity == 0 {
				annotation.Opacity = 1
			}
			if annotation.Opacity < .1 || annotation.Opacity > 1 {
				return fmt.Errorf("canvas annotation opacity is invalid")
			}
			annotation.Label = strings.TrimSpace(annotation.Label)
			if len([]rune(annotation.Label)) > 300 {
				return fmt.Errorf("canvas annotation label is too long")
			}
			if annotation.LabelPosition == "" {
				annotation.LabelPosition = "center"
			}
			switch annotation.LabelPosition {
			case "top-left", "top-center", "top-right", "middle-left", "center", "middle-right", "bottom-left", "bottom-center", "bottom-right":
			default:
				return fmt.Errorf("canvas annotation label position is invalid")
			}
		}
		for _, annotation := range canvas.Annotations {
			if _, err := tx.Exec(ctx, `
				INSERT INTO canvas_annotations (
					id,episode_id,annotation_type,position_x,position_y,width,height,stroke_color,stroke_width,line_style,opacity,label,label_position,z_index
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
				annotation.ID, episodeID, annotation.AnnotationType, annotation.PositionX, annotation.PositionY,
				annotation.Width, annotation.Height, annotation.StrokeColor, annotation.StrokeWidth,
				annotation.LineStyle, annotation.Opacity, annotation.Label, annotation.LabelPosition, annotation.ZIndex); err != nil {
				return err
			}
		}
		return nil
	})
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
