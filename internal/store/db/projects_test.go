package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/store/db"
)

func TestProjectProductionSpecsRoundTrip(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := db.New(database)
	fps := 24.0
	project, err := store.CreateProject(ctx, "门后的世界", "双分镜演示", "9:16", "1080 × 1920", &fps)
	if err != nil {
		t.Fatal(err)
	}
	if project.AspectRatio != "9:16" || project.Resolution != "1080 × 1920" || project.FrameRate == nil || *project.FrameRate != fps {
		t.Fatalf("unexpected project production specs: %#v", project)
	}

	updatedFPS := 30.0
	project, err = store.UpdateProject(ctx, project.ID, project.Title, project.Description, "16:9", "1920 × 1080", &updatedFPS)
	if err != nil {
		t.Fatal(err)
	}
	if project.AspectRatio != "16:9" || project.Resolution != "1920 × 1080" || project.FrameRate == nil || *project.FrameRate != updatedFPS {
		t.Fatalf("unexpected updated production specs: %#v", project)
	}
}
