package db_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	storedb "github.com/gofurry/sagaflow/internal/store/db"
	"github.com/google/uuid"
)

func TestAssetAndGenerationJobPagination(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "pagination.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := storedb.New(database)
	projectID := uuid.New()
	groupID := uuid.New()
	if _, err := database.Exec(`INSERT INTO projects (id,title) VALUES (?,?)`, projectID, "Pagination"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO asset_groups (id,project_id,kind,name) VALUES (?,?,?,?)`, groupID, projectID, "character", "Characters"); err != nil {
		t.Fatal(err)
	}

	expectedAvailable := int64(0)
	for index := range 55 {
		objectID := uuid.New()
		assetID := uuid.New()
		status := "adopted"
		if index%10 == 0 {
			status = "discarded"
		} else {
			expectedAvailable++
		}
		createdAt := time.Date(2026, 7, 24, 0, index, 0, 0, time.UTC).Format(time.RFC3339Nano)
		if _, err := database.Exec(`
			INSERT INTO local_objects (id,project_id,object_key,original_name,purpose,state,created_at)
			VALUES (?,?,?,?,?,'ready',?)`, objectID, projectID, assetID.String(), fmt.Sprintf("asset-%02d.png", index), "test", createdAt); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`
			INSERT INTO assets (id,project_id,group_id,object_id,name,media_type,status,mime_type,created_at)
			VALUES (?,?,?,?,?,? ,?,'image/png',?)`,
			assetID, projectID, groupID, objectID, fmt.Sprintf("asset-%02d", index), "image", status, createdAt); err != nil {
			t.Fatal(err)
		}
	}

	assetPage, err := store.ListAssetsPage(ctx, storedb.AssetFilter{
		ProjectID: projectID, GroupKind: "character", ExcludeStatus: "discarded", Page: 2, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assetPage.Total != expectedAvailable || len(assetPage.Items) != 20 || assetPage.Page != 2 {
		t.Fatalf("unexpected asset page: total=%d items=%d page=%d", assetPage.Total, len(assetPage.Items), assetPage.Page)
	}
	searchPage, err := store.ListAssetsPage(ctx, storedb.AssetFilter{ProjectID: projectID, Name: "asset-54", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if searchPage.Total != 1 || len(searchPage.Items) != 1 || searchPage.Items[0].Name != "asset-54" {
		t.Fatalf("unexpected asset search result: %#v", searchPage)
	}

	providerID := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	for index := range 125 {
		status := "succeeded"
		if index%2 == 0 {
			status = "failed"
		}
		if _, err := database.Exec(`
			INSERT INTO generation_jobs (id,project_id,provider_id,capability,prompt,status)
			VALUES (?,?,?,?,?,?)`, uuid.New(), projectID, providerID, "text", fmt.Sprintf("prompt-%03d", index), status); err != nil {
			t.Fatal(err)
		}
	}
	jobPage, err := store.ListGenerationJobs(ctx, storedb.GenerationJobFilter{
		ProjectID: projectID, Status: "succeeded", Capability: "text", Page: 4, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if jobPage.Total != 62 || len(jobPage.Items) != 2 {
		t.Fatalf("unexpected generation job page: total=%d items=%d", jobPage.Total, len(jobPage.Items))
	}
	searchJobs, err := store.ListGenerationJobs(ctx, storedb.GenerationJobFilter{
		ProjectID: projectID, Search: "prompt-124", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if searchJobs.Total != 1 || len(searchJobs.Items) != 1 {
		t.Fatalf("unexpected generation search result: %#v", searchJobs)
	}
}
