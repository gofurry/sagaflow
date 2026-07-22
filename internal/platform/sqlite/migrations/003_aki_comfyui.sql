-- +goose Up
INSERT INTO model_providers (
  id, code, adapter_code, display_name, base_url, auth_type, capabilities, max_concurrency
) VALUES (
  '10000000-0000-0000-0000-000000000007',
  'comfyui-aki-local',
  'comfyui',
  '本机秋叶 ComfyUI',
  'http://127.0.0.1:8189',
  'none',
  '["image","audio","video","multimodal"]',
  1
)
ON CONFLICT(code) DO NOTHING;

-- +goose Down
DELETE FROM model_providers WHERE code='comfyui-aki-local';
