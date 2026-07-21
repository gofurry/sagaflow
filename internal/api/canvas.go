package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

type canvasRequest struct {
	Nodes       []canvasNodeRequest       `json:"nodes"`
	Edges       []canvasEdgeRequest       `json:"edges"`
	Annotations []canvasAnnotationRequest `json:"annotations"`
}

type canvasNodeRequest struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Width  *float64        `json:"width"`
	Height *float64        `json:"height"`
	ZIndex int32           `json:"z_index"`
	Data   json.RawMessage `json:"data"`
}

type canvasNodeDataRequest struct {
	Kind                  string     `json:"kind"`
	Title                 string     `json:"title"`
	Body                  string     `json:"body"`
	AssetID               *uuid.UUID `json:"asset_id"`
	ShotNumber            *int32     `json:"shot_number"`
	TargetDurationSeconds *int32     `json:"target_duration_seconds"`
	SelectedVideoAssetID  *uuid.UUID `json:"selected_video_asset_id"`
	Color                 string     `json:"color"`
}

type canvasEdgeRequest struct {
	ID           uuid.UUID       `json:"id"`
	Source       uuid.UUID       `json:"source"`
	Target       uuid.UUID       `json:"target"`
	SourceHandle *string         `json:"source_handle"`
	TargetHandle *string         `json:"target_handle"`
	Type         string          `json:"type"`
	Data         json.RawMessage `json:"data"`
}

type canvasAnnotationRequest struct {
	ID       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	StrokeColor string  `json:"stroke_color"`
	StrokeWidth float64 `json:"stroke_width"`
	LineStyle   string  `json:"line_style"`
	Opacity     float64 `json:"opacity"`
	Label       string  `json:"label"`
	ZIndex      int32   `json:"z_index"`
}

type canvasResponse struct {
	Nodes       []map[string]any `json:"nodes"`
	Edges       []map[string]any `json:"edges"`
	Annotations []map[string]any `json:"annotations"`
}

func (s *Server) getCanvas(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	canvas, err := s.store.GetCanvas(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, mapCanvas(canvas))
}

func (s *Server) saveCanvas(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req canvasRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	canvas := db.Canvas{
		Nodes: make([]db.CanvasNode, 0, len(req.Nodes)), Edges: make([]db.CanvasEdge, 0, len(req.Edges)),
		Annotations: make([]db.CanvasAnnotation, 0, len(req.Annotations)),
	}
	for _, node := range req.Nodes {
		if node.ID == uuid.Nil || strings.TrimSpace(node.Type) == "" {
			return fmt.Errorf("%w: every node needs id and type", service.ErrInvalidInput)
		}
		var data canvasNodeDataRequest
		if len(node.Data) > 0 {
			if err := json.Unmarshal(node.Data, &data); err != nil {
				return fmt.Errorf("%w: invalid canvas node data", service.ErrInvalidInput)
			}
		}
		if data.Kind != "" && data.Kind != node.Type {
			return fmt.Errorf("%w: canvas node kind and type must match", service.ErrInvalidInput)
		}
		canvas.Nodes = append(canvas.Nodes, db.CanvasNode{
			ID: node.ID, EpisodeID: id, NodeType: node.Type, PositionX: node.Position.X, PositionY: node.Position.Y,
			Width: node.Width, Height: node.Height, ZIndex: node.ZIndex, Data: node.Data,
			AssetID: data.AssetID, Title: data.Title, Body: data.Body, ShotNumber: data.ShotNumber,
			TargetDurationSeconds: data.TargetDurationSeconds, SelectedVideoAssetID: data.SelectedVideoAssetID, Color: data.Color,
		})
	}
	for _, edge := range req.Edges {
		if edge.ID == uuid.Nil || strings.TrimSpace(edge.Type) == "" {
			return fmt.Errorf("%w: every edge needs id and type", service.ErrInvalidInput)
		}
		canvas.Edges = append(canvas.Edges, db.CanvasEdge{
			ID: edge.ID, EpisodeID: id, SourceNodeID: edge.Source, TargetNodeID: edge.Target,
			SourceHandle: edge.SourceHandle, TargetHandle: edge.TargetHandle, EdgeType: edge.Type, Data: edge.Data,
		})
	}
	for _, annotation := range req.Annotations {
		canvas.Annotations = append(canvas.Annotations, db.CanvasAnnotation{
			ID: annotation.ID, EpisodeID: id, AnnotationType: annotation.Type,
			PositionX: annotation.Position.X, PositionY: annotation.Position.Y, Width: annotation.Width, Height: annotation.Height,
			StrokeColor: annotation.StrokeColor, StrokeWidth: annotation.StrokeWidth, LineStyle: annotation.LineStyle,
			Opacity: annotation.Opacity, Label: annotation.Label, ZIndex: annotation.ZIndex,
		})
	}
	if err := s.store.SaveCanvas(c.Context(), id, canvas); err != nil {
		return err
	}
	stored, err := s.store.GetCanvas(c.Context(), id)
	if err != nil {
		return err
	}
	return writeOK(c, mapCanvas(stored))
}

func (s *Server) selectCanvasVideo(c fiber.Ctx) error {
	id, err := idParam(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		AssetID *uuid.UUID `json:"asset_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(400, "invalid JSON body")
	}
	node, err := s.store.SelectCanvasVideoAsset(c.Context(), id, req.AssetID)
	if err != nil {
		return err
	}
	return writeOK(c, mapCanvas(db.Canvas{Nodes: []db.CanvasNode{node}}).Nodes[0])
}

func mapCanvas(canvas db.Canvas) canvasResponse {
	out := canvasResponse{
		Nodes: make([]map[string]any, 0, len(canvas.Nodes)), Edges: make([]map[string]any, 0, len(canvas.Edges)),
		Annotations: make([]map[string]any, 0, len(canvas.Annotations)),
	}
	for _, node := range canvas.Nodes {
		data := map[string]any{}
		_ = json.Unmarshal(node.Data, &data)
		data["kind"] = node.NodeType
		data["title"] = node.Title
		data["body"] = node.Body
		data["asset_id"] = node.AssetID
		data["shot_number"] = node.ShotNumber
		data["target_duration_seconds"] = node.TargetDurationSeconds
		data["selected_video_asset_id"] = node.SelectedVideoAssetID
		data["color"] = node.Color
		out.Nodes = append(out.Nodes, map[string]any{
			"id": node.ID, "type": node.NodeType, "position": map[string]float64{"x": node.PositionX, "y": node.PositionY},
			"width": node.Width, "height": node.Height, "z_index": node.ZIndex, "data": data,
		})
	}
	for _, edge := range canvas.Edges {
		data := map[string]any{}
		_ = json.Unmarshal(edge.Data, &data)
		data["relation"] = edge.EdgeType
		out.Edges = append(out.Edges, map[string]any{
			"id": edge.ID, "source": edge.SourceNodeID, "target": edge.TargetNodeID,
			"source_handle": edge.SourceHandle, "target_handle": edge.TargetHandle, "type": edge.EdgeType, "data": data,
		})
	}
	for _, annotation := range canvas.Annotations {
		out.Annotations = append(out.Annotations, map[string]any{
			"id": annotation.ID, "type": annotation.AnnotationType,
			"position": map[string]float64{"x": annotation.PositionX, "y": annotation.PositionY},
			"width":    annotation.Width, "height": annotation.Height, "stroke_color": annotation.StrokeColor,
			"stroke_width": annotation.StrokeWidth, "line_style": annotation.LineStyle,
			"opacity": annotation.Opacity, "label": annotation.Label, "z_index": annotation.ZIndex,
		})
	}
	return out
}
