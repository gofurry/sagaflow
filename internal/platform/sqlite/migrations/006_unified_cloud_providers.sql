-- +goose Up
UPDATE model_providers
SET code='volcengine', display_name='火山方舟',
    capabilities='["text","image","video","multimodal"]',
    metadata='{"source":"builtin","platform":"ark"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='10000000-0000-0000-0000-000000000002';

UPDATE model_providers
SET capabilities='["text","image","audio","video"]',
    metadata='{"source":"builtin","platform":"minimax"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='10000000-0000-0000-0000-000000000004';

UPDATE model_providers
SET metadata='{"source":"builtin","platform":"deepseek"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='10000000-0000-0000-0000-000000000001';

INSERT INTO provider_credentials (
  id,provider_id,name,encrypted_api_key,key_hint,is_active,created_at,updated_at,last_used_at
)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' ||
  lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
  lower(hex(randomblob(6))),
  '10000000-0000-0000-0000-000000000002',
  name,encrypted_api_key,key_hint,1,created_at,updated_at,last_used_at
FROM provider_credentials
WHERE provider_id='10000000-0000-0000-0000-000000000003'
  AND NOT EXISTS (
    SELECT 1 FROM provider_credentials
    WHERE provider_id='10000000-0000-0000-0000-000000000002' AND is_active=1
  )
ORDER BY is_active DESC,updated_at DESC
LIMIT 1;

UPDATE model_catalog
SET provider_id='10000000-0000-0000-0000-000000000002'
WHERE provider_id='10000000-0000-0000-0000-000000000003';

UPDATE model_catalog
SET enabled=0,available=0,
    metadata='{"source":"legacy","support_status":"unsupported"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000001'
  AND model_id='deepseek-v4-flash';

DELETE FROM model_providers
WHERE id='10000000-0000-0000-0000-000000000003';

-- +goose Down
INSERT INTO model_providers (
  id,code,adapter_code,display_name,base_url,auth_type,capabilities,enabled,metadata,max_concurrency
) VALUES (
  '10000000-0000-0000-0000-000000000003','seedance','volcengine','Seedance',
  'https://ark.cn-beijing.volces.com/api/v3','api_key','["video"]',1,'{}',1
)
ON CONFLICT(id) DO NOTHING;

UPDATE model_catalog
SET provider_id='10000000-0000-0000-0000-000000000003'
WHERE id='20000000-0000-0000-0000-000000000003';

UPDATE model_providers
SET code='seedream',display_name='Seedream',capabilities='["image"]',metadata='{}'
WHERE id='10000000-0000-0000-0000-000000000002';

UPDATE model_providers
SET capabilities='["audio"]',metadata='{}'
WHERE id='10000000-0000-0000-0000-000000000004';
