-- +goose Up
INSERT INTO model_providers (
  id,code,adapter_code,display_name,base_url,auth_type,capabilities,enabled,metadata,max_concurrency
) VALUES (
  '10000000-0000-0000-0000-000000000008','aliyun_bailian','aliyun_bailian','阿里云百炼',
  'https://dashscope.aliyuncs.com','api_key','["text","image","audio","video","multimodal"]',1,
  '{"source":"builtin","platform":"model_studio","region":"cn-beijing"}',2
)
ON CONFLICT(id) DO UPDATE SET
  code=excluded.code,
  adapter_code=excluded.adapter_code,
  display_name=excluded.display_name,
  base_url=excluded.base_url,
  auth_type=excluded.auth_type,
  capabilities=excluded.capabilities,
  enabled=excluded.enabled,
  metadata=excluded.metadata,
  max_concurrency=excluded.max_concurrency,
  updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- +goose Down
DELETE FROM model_catalog
WHERE provider_id='10000000-0000-0000-0000-000000000008';
DELETE FROM model_providers
WHERE id='10000000-0000-0000-0000-000000000008';
