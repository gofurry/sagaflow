-- +goose Up
INSERT INTO model_providers (
  id,code,adapter_code,display_name,base_url,auth_type,capabilities,enabled,metadata,max_concurrency
) VALUES (
  '10000000-0000-0000-0000-000000000012','moonshot','moonshot','Kimi · Moonshot',
  'https://api.moonshot.cn/v1','api_key','["text","multimodal"]',1,
  '{"source":"builtin","platform":"moonshot","dynamic_catalog":true}',2
)
ON CONFLICT(id) DO UPDATE SET
  code=excluded.code,
  adapter_code=excluded.adapter_code,
  display_name=excluded.display_name,
  base_url=excluded.base_url,
  auth_type=excluded.auth_type,
  capabilities=excluded.capabilities,
  metadata=excluded.metadata,
  max_concurrency=excluded.max_concurrency,
  updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- +goose Down
DELETE FROM model_providers
WHERE id='10000000-0000-0000-0000-000000000012'
  AND NOT EXISTS (SELECT 1 FROM provider_credentials WHERE provider_id='10000000-0000-0000-0000-000000000012')
  AND NOT EXISTS (SELECT 1 FROM generation_jobs WHERE provider_id='10000000-0000-0000-0000-000000000012');
