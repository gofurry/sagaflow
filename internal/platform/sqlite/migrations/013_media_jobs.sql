-- +goose Up
CREATE TABLE media_jobs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  target_asset_group_id TEXT REFERENCES asset_groups(id) ON DELETE SET NULL,
  tool TEXT NOT NULL CHECK (tool IN ('inspect','transcode','aspect','audio','trim','merge','screenshot')),
  source_asset_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(source_asset_ids)),
  output_name TEXT NOT NULL DEFAULT '',
  parameters TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters)),
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed','canceled','interrupted')),
  stage TEXT NOT NULL DEFAULT 'queued',
  progress REAL NOT NULL DEFAULT 0,
  output_asset_id TEXT REFERENCES assets(id) ON DELETE SET NULL,
  command_snapshot TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(command_snapshot)),
  probe_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(probe_snapshot)),
  error_message TEXT NOT NULL DEFAULT '',
  cancel_requested INTEGER NOT NULL DEFAULT 0,
  available_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  lease_until TEXT,
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX media_jobs_project_idx ON media_jobs(project_id, created_at DESC);
CREATE INDEX media_jobs_queue_idx ON media_jobs(status, available_at);

-- +goose Down
DROP TABLE IF EXISTS media_jobs;
