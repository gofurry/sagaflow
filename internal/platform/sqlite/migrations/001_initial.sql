-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE account (
  id TEXT PRIMARY KEY,
  singleton INTEGER NOT NULL DEFAULT 1 UNIQUE CHECK (singleton = 1),
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  display_name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  last_login_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX sessions_active_idx ON sessions(token_hash, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL CHECK (length(trim(title)) > 0),
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE episodes (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  episode_number INTEGER NOT NULL CHECK (episode_number > 0),
  title TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  target_duration_seconds INTEGER,
  target_shot_count INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(project_id, episode_number)
);

CREATE TABLE episode_scripts (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate','adopted','archived')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(episode_id, version)
);
CREATE UNIQUE INDEX episode_scripts_adopted_idx ON episode_scripts(episode_id) WHERE status = 'adopted';

CREATE TABLE asset_groups (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  parent_id TEXT REFERENCES asset_groups(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('character','scene','prop','material')),
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(project_id, parent_id, name)
);

CREATE TABLE local_objects (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  original_name TEXT NOT NULL,
  purpose TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','ready','deleting','deleted','failed')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX local_objects_project_idx ON local_objects(project_id, created_at DESC);
CREATE INDEX local_objects_hash_idx ON local_objects(sha256) WHERE state = 'ready';
CREATE INDEX local_objects_key_idx ON local_objects(object_key) WHERE state = 'ready';

CREATE TABLE model_providers (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  adapter_code TEXT NOT NULL,
  display_name TEXT NOT NULL,
  base_url TEXT NOT NULL DEFAULT '',
  auth_type TEXT NOT NULL DEFAULT 'api_key',
  capabilities TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(capabilities)),
  enabled INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  max_concurrency INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrency > 0),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE provider_credentials (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  encrypted_api_key TEXT NOT NULL,
  key_hint TEXT NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  last_used_at TEXT
);
CREATE UNIQUE INDEX provider_credentials_active_idx ON provider_credentials(provider_id) WHERE is_active = 1;

CREATE TABLE model_catalog (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL,
  display_name TEXT NOT NULL,
  capability TEXT NOT NULL CHECK (capability IN ('text','image','audio','video','multimodal')),
  input_modalities TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(input_modalities)),
  features TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(features)),
  parameter_schema TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameter_schema)),
  default_parameters TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(default_parameters)),
  enabled INTEGER NOT NULL DEFAULT 1,
  available INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(provider_id, model_id, capability)
);
CREATE INDEX model_catalog_capability_idx ON model_catalog(capability, enabled);

CREATE TABLE model_presets (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL REFERENCES model_catalog(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  parameters TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters)),
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(model_id, name)
);
CREATE UNIQUE INDEX model_presets_default_idx ON model_presets(model_id) WHERE is_default = 1;

CREATE TABLE workflow_templates (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  capability TEXT NOT NULL CHECK (capability IN ('image','audio','video','multimodal')),
  input_modalities TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(input_modalities)),
  workflow TEXT NOT NULL CHECK (json_valid(workflow)),
  parameter_schema TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameter_schema)),
  default_parameters TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(default_parameters)),
  bindings TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(bindings)),
  outputs TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(outputs)),
  requirements TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(requirements)),
  enabled INTEGER NOT NULL DEFAULT 1,
  version INTEGER NOT NULL DEFAULT 1,
  checksum TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE workflow_compatibilities (
  workflow_template_id TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('unknown','compatible','incompatible','error')),
  report TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(report)),
  checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY(workflow_template_id, provider_id)
);

CREATE TABLE prompt_presets (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  model_id TEXT REFERENCES model_catalog(id) ON DELETE SET NULL,
  model_preset_id TEXT REFERENCES model_presets(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  capability TEXT NOT NULL CHECK (capability IN ('text','image','audio','video')),
  content TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(project_id, capability, name)
);

CREATE TABLE canvas_nodes (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
  node_type TEXT NOT NULL,
  position_x REAL NOT NULL,
  position_y REAL NOT NULL,
  width REAL,
  height REAL,
  z_index INTEGER NOT NULL DEFAULT 0,
  data TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data)),
  asset_id TEXT,
  title TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  shot_number INTEGER,
  target_duration_seconds INTEGER,
  selected_video_asset_id TEXT,
  color TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX canvas_nodes_episode_idx ON canvas_nodes(episode_id);

CREATE TABLE canvas_edges (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
  source_node_id TEXT NOT NULL REFERENCES canvas_nodes(id) ON DELETE CASCADE,
  target_node_id TEXT NOT NULL REFERENCES canvas_nodes(id) ON DELETE CASCADE,
  source_handle TEXT,
  target_handle TEXT,
  edge_type TEXT NOT NULL DEFAULT 'default',
  data TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX canvas_edges_episode_idx ON canvas_edges(episode_id);

CREATE TABLE canvas_annotations (
  id TEXT PRIMARY KEY,
  episode_id TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
  annotation_type TEXT NOT NULL,
  position_x REAL NOT NULL,
  position_y REAL NOT NULL,
  width REAL NOT NULL,
  height REAL NOT NULL,
  stroke_color TEXT NOT NULL DEFAULT '',
  stroke_width REAL NOT NULL DEFAULT 1,
  line_style TEXT NOT NULL DEFAULT 'solid',
  opacity REAL NOT NULL DEFAULT 1,
  label TEXT NOT NULL DEFAULT '',
  z_index INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX canvas_annotations_episode_idx ON canvas_annotations(episode_id);

CREATE TABLE generation_jobs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
  canvas_node_id TEXT REFERENCES canvas_nodes(id) ON DELETE SET NULL,
  target_asset_group_id TEXT REFERENCES asset_groups(id) ON DELETE SET NULL,
  prompt_preset_id TEXT REFERENCES prompt_presets(id) ON DELETE SET NULL,
  model_preset_id TEXT REFERENCES model_presets(id) ON DELETE SET NULL,
  target_kind TEXT NOT NULL DEFAULT 'model' CHECK (target_kind IN ('model','workflow')),
  provider_id TEXT REFERENCES model_providers(id) ON DELETE SET NULL,
  model_id TEXT REFERENCES model_catalog(id) ON DELETE SET NULL,
  workflow_template_id TEXT REFERENCES workflow_templates(id) ON DELETE SET NULL,
  target_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(target_snapshot)),
  capability TEXT NOT NULL,
  prompt TEXT NOT NULL DEFAULT '',
  parameters TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters)),
  input_references TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(input_references)),
  input_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input_snapshot)),
  output_name TEXT NOT NULL DEFAULT '',
  output_staged_asset_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(output_staged_asset_ids)),
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','succeeded','failed','canceled','interrupted')),
  stage TEXT NOT NULL DEFAULT 'queued',
  progress REAL NOT NULL DEFAULT 0,
  provider_job_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  attempt INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 1,
  available_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  lease_until TEXT,
  started_at TEXT,
  finished_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX generation_jobs_queue_idx ON generation_jobs(status, available_at, created_at);
CREATE INDEX generation_jobs_project_idx ON generation_jobs(project_id, created_at DESC);

CREATE TABLE generation_invocations (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE REFERENCES generation_jobs(id) ON DELETE CASCADE,
  provider_code TEXT NOT NULL,
  model_identifier TEXT NOT NULL,
  capability TEXT NOT NULL,
  credential_id TEXT REFERENCES provider_credentials(id) ON DELETE SET NULL,
  credential_source TEXT NOT NULL DEFAULT 'database',
  request_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(request_snapshot)),
  response_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(response_snapshot)),
  usage_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(usage_snapshot)),
  status TEXT NOT NULL DEFAULT 'running',
  provider_job_id TEXT NOT NULL DEFAULT '',
  error_kind TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  error_status_code INTEGER NOT NULL DEFAULT 0,
  retryable INTEGER NOT NULL DEFAULT 0,
  output_staged_asset_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(output_staged_asset_ids)),
  output_asset_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(output_asset_ids)),
  started_at TEXT NOT NULL,
  finished_at TEXT,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE generation_invocation_events (
  id TEXT PRIMARY KEY,
  invocation_id TEXT NOT NULL REFERENCES generation_invocations(id) ON DELETE CASCADE,
  sequence INTEGER NOT NULL,
  stage TEXT NOT NULL,
  progress REAL NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT '',
  provider_job_id TEXT NOT NULL DEFAULT '',
  usage_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(usage_snapshot)),
  detail_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(detail_snapshot)),
  elapsed_ms INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(invocation_id, sequence)
);

CREATE TABLE generation_reference_uploads (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  job_id TEXT REFERENCES generation_jobs(id) ON DELETE CASCADE,
  object_id TEXT NOT NULL REFERENCES local_objects(id),
  name TEXT NOT NULL,
  media_type TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  file_size_bytes INTEGER NOT NULL DEFAULT 0,
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE staged_assets (
  id TEXT PRIMARY KEY,
  job_id TEXT REFERENCES generation_jobs(id) ON DELETE SET NULL,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  object_id TEXT NOT NULL REFERENCES local_objects(id),
  source TEXT NOT NULL DEFAULT 'generated' CHECK (source IN ('generated','upload')),
  name TEXT NOT NULL,
  media_type TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  file_size_bytes INTEGER NOT NULL DEFAULT 0,
  content_text TEXT NOT NULL DEFAULT '',
  original_url TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_identifier TEXT NOT NULL DEFAULT '',
  parameters_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters_snapshot)),
  input_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input_snapshot)),
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX staged_assets_project_idx ON staged_assets(project_id, created_at DESC);

CREATE TABLE assets (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  group_id TEXT REFERENCES asset_groups(id) ON DELETE CASCADE,
  staged_asset_id TEXT UNIQUE REFERENCES staged_assets(id) ON DELETE SET NULL,
  episode_id TEXT REFERENCES episodes(id) ON DELETE SET NULL,
  canvas_node_id TEXT REFERENCES canvas_nodes(id) ON DELETE SET NULL,
  object_id TEXT NOT NULL REFERENCES local_objects(id),
  name TEXT NOT NULL,
  media_type TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'upload' CHECK (source IN ('upload','generated')),
  status TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate','adopted','discarded')),
  mime_type TEXT NOT NULL,
  file_size_bytes INTEGER NOT NULL DEFAULT 0,
  original_url TEXT NOT NULL DEFAULT '',
  provider_code TEXT NOT NULL DEFAULT '',
  model_identifier TEXT NOT NULL DEFAULT '',
  parameters_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(parameters_snapshot)),
  input_snapshot TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(input_snapshot)),
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  deleted_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX assets_project_idx ON assets(project_id, created_at DESC);
CREATE INDEX assets_group_idx ON assets(group_id, created_at DESC);

CREATE TABLE voice_profiles (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  model_id TEXT NOT NULL REFERENCES model_catalog(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  voice_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'ready',
  source_name TEXT NOT NULL DEFAULT '',
  source_mime_type TEXT NOT NULL DEFAULT '',
  source_file_size_bytes INTEGER NOT NULL DEFAULT 0,
  prompt_text TEXT NOT NULL DEFAULT '',
  preview_mime_type TEXT NOT NULL DEFAULT '',
  preview_file_size_bytes INTEGER NOT NULL DEFAULT 0,
  source_object_id TEXT NOT NULL REFERENCES local_objects(id),
  prompt_object_id TEXT REFERENCES local_objects(id),
  preview_object_id TEXT NOT NULL REFERENCES local_objects(id),
  provider_file_id TEXT NOT NULL DEFAULT '',
  provider_prompt_file_id TEXT NOT NULL DEFAULT '',
  activated_at TEXT,
  metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(metadata)),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(provider_id, voice_id)
);

CREATE TABLE s3_connections (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  provider TEXT NOT NULL DEFAULT 's3',
  endpoint TEXT NOT NULL,
  public_endpoint TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL,
  bucket TEXT NOT NULL,
  prefix TEXT NOT NULL DEFAULT 'sagaflow',
  force_path_style INTEGER NOT NULL DEFAULT 0,
  encrypted_credentials TEXT NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX s3_connections_default_idx ON s3_connections(is_default) WHERE is_default = 1;

CREATE TABLE asset_remote_exports (
  id TEXT PRIMARY KEY,
  asset_id TEXT NOT NULL REFERENCES assets(id),
  connection_id TEXT NOT NULL REFERENCES s3_connections(id),
  object_key TEXT NOT NULL,
  etag TEXT NOT NULL DEFAULT '',
  public_url TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'ready' CHECK (state IN ('uploading','ready','failed','deleting')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE(asset_id, connection_id, object_key)
);

INSERT INTO model_providers (id, code, adapter_code, display_name, base_url, auth_type, capabilities, max_concurrency) VALUES
  ('10000000-0000-0000-0000-000000000001','deepseek','deepseek','DeepSeek','https://api.deepseek.com','api_key','["text"]',2),
  ('10000000-0000-0000-0000-000000000002','seedream','volcengine','Seedream','https://ark.cn-beijing.volces.com/api/v3','api_key','["image"]',1),
  ('10000000-0000-0000-0000-000000000003','seedance','volcengine','Seedance','https://ark.cn-beijing.volces.com/api/v3','api_key','["video"]',1),
  ('10000000-0000-0000-0000-000000000004','minimax','minimax','MiniMax','https://api.minimaxi.com','api_key','["audio"]',1);

INSERT INTO model_catalog (id, provider_id, model_id, display_name, capability, input_modalities, features, parameter_schema, default_parameters) VALUES
  ('20000000-0000-0000-0000-000000000001','10000000-0000-0000-0000-000000000001','deepseek-v4-flash','DeepSeek V4 Flash','text','["text"]','["thinking","tools"]','{"type":"object","properties":{"thinking":{"type":"string","enum":["enabled","disabled"]},"max_tokens":{"type":"integer","minimum":1,"maximum":32768},"temperature":{"type":"number","minimum":0,"maximum":2},"top_p":{"type":"number","minimum":0,"maximum":1}}}','{"thinking":"enabled","max_tokens":4096,"temperature":1,"top_p":1}'),
  ('20000000-0000-0000-0000-000000000002','10000000-0000-0000-0000-000000000002','doubao-seedream-5-0-260128','Seedream 5.0','image','["text","image"]','["image_generation"]','{"type":"object","properties":{"size":{"type":"string"},"seed":{"type":"integer"},"max_images":{"type":"integer","minimum":1,"maximum":15},"watermark":{"type":"boolean"}}}','{"size":"2K","seed":-1,"max_images":1,"watermark":false}'),
  ('20000000-0000-0000-0000-000000000003','10000000-0000-0000-0000-000000000003','doubao-seedance-2-0-mini-260615','Seedance 2.0 Mini','video','["text","image","video"]','["video_generation"]','{"type":"object","properties":{"ratio":{"type":"string","enum":["16:9","9:16","1:1"]},"duration":{"type":"integer"},"generate_audio":{"type":"boolean"},"watermark":{"type":"boolean"}}}','{"ratio":"16:9","duration":5,"generate_audio":true,"watermark":false}'),
  ('20000000-0000-0000-0000-000000000004','10000000-0000-0000-0000-000000000004','speech-2.8-hd','MiniMax Speech 2.8 HD','audio','["text","audio"]','["speech_generation","voice_clone"]','{"type":"object","properties":{"voice_id":{"type":"string"},"speed":{"type":"number"},"volume":{"type":"number"},"pitch":{"type":"integer"},"format":{"type":"string"}}}','{"voice_id":"Chinese (Mandarin)_Lyrical_Voice","speed":1,"volume":1,"pitch":0,"format":"mp3"}');

-- +goose Down
DROP TABLE IF EXISTS asset_remote_exports;
DROP TABLE IF EXISTS s3_connections;
DROP TABLE IF EXISTS voice_profiles;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS staged_assets;
DROP TABLE IF EXISTS generation_reference_uploads;
DROP TABLE IF EXISTS generation_invocation_events;
DROP TABLE IF EXISTS generation_invocations;
DROP TABLE IF EXISTS generation_jobs;
DROP TABLE IF EXISTS canvas_annotations;
DROP TABLE IF EXISTS canvas_edges;
DROP TABLE IF EXISTS canvas_nodes;
DROP TABLE IF EXISTS prompt_presets;
DROP TABLE IF EXISTS workflow_compatibilities;
DROP TABLE IF EXISTS workflow_templates;
DROP TABLE IF EXISTS model_presets;
DROP TABLE IF EXISTS model_catalog;
DROP TABLE IF EXISTS provider_credentials;
DROP TABLE IF EXISTS model_providers;
DROP TABLE IF EXISTS local_objects;
DROP TABLE IF EXISTS asset_groups;
DROP TABLE IF EXISTS episode_scripts;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS account;
