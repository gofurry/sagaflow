-- +goose Up
ALTER TABLE media_jobs
  ADD COLUMN output_staged_asset_id TEXT REFERENCES staged_assets(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE media_jobs DROP COLUMN output_staged_asset_id;
