package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

func TestSelectCanvasVideoAssetBindsUnassignedVideo(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	projectID := uuid.New()
	episodeID := uuid.New()
	videoNodeID := uuid.New()
	otherVideoNodeID := uuid.New()
	objectID := uuid.New()
	assetID := uuid.New()

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO projects (id,title) VALUES ($1,'雾海归灯')`, []any{projectID}},
		{`INSERT INTO episodes (id,project_id,episode_number,title) VALUES ($1,$2,1,'雾港孤灯')`, []any{episodeID, projectID}},
		{`INSERT INTO canvas_nodes (id,episode_id,node_type,position_x,position_y) VALUES ($1,$2,'video',0,0)`, []any{videoNodeID, episodeID}},
		{`INSERT INTO canvas_nodes (id,episode_id,node_type,position_x,position_y) VALUES ($1,$2,'video',100,0)`, []any{otherVideoNodeID, episodeID}},
		{`INSERT INTO local_objects (id,project_id,object_key,original_name,purpose,mime_type,state) VALUES ($1,$2,'videos/shot.mp4','shot.mp4','asset','video/mp4','ready')`, []any{objectID, projectID}},
		{`INSERT INTO assets (id,project_id,episode_id,object_id,name,media_type,status,mime_type) VALUES ($1,$2,$3,$4,'分镜 01','video','adopted','video/mp4')`, []any{assetID, projectID, episodeID, objectID}},
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	store := db.New(database)
	node, err := store.SelectCanvasVideoAsset(ctx, videoNodeID, &assetID)
	if err != nil {
		t.Fatal(err)
	}
	if node.SelectedVideoAssetID == nil || *node.SelectedVideoAssetID != assetID {
		t.Fatalf("expected selected video %s, got %v", assetID, node.SelectedVideoAssetID)
	}

	var boundNodeID uuid.UUID
	if err := database.QueryRowContext(ctx, `SELECT canvas_node_id FROM assets WHERE id=$1`, assetID).Scan(&boundNodeID); err != nil {
		t.Fatal(err)
	}
	if boundNodeID != videoNodeID {
		t.Fatalf("expected asset to bind to %s, got %s", videoNodeID, boundNodeID)
	}

	if _, err := store.SelectCanvasVideoAsset(ctx, otherVideoNodeID, &assetID); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected conflict when rebinding video, got %v", err)
	}
}
