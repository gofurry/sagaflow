-- +goose Up
UPDATE model_catalog
SET model_id='deepseek-v4-flash',display_name='DeepSeek V4 Flash',enabled=1,available=1,
    metadata='{"source":"builtin","support_status":"verified","manifest_version":2,"documentation_url":"https://api-docs.deepseek.com/quick_start/pricing/"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000001';

UPDATE model_catalog
SET model_id='deepseek-v4-pro',display_name='DeepSeek V4 Pro',enabled=1,available=1,
    metadata='{"source":"builtin","support_status":"verified","manifest_version":2,"documentation_url":"https://api-docs.deepseek.com/quick_start/pricing/"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000005';

UPDATE model_catalog
SET enabled=0,available=0,
    metadata='{"source":"legacy","support_status":"retired","retired_at":"2026-07-24T15:59:00Z"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000006'
  AND model_id='deepseek-reasoner';

-- +goose Down
UPDATE model_catalog
SET model_id='deepseek-v4-flash',display_name='DeepSeek V4 Flash',enabled=0,available=0,
    metadata='{"source":"legacy","support_status":"unsupported"}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000001';

UPDATE model_catalog
SET model_id='deepseek-chat',display_name='DeepSeek Chat',enabled=1,available=1,
    metadata='{"source":"builtin","support_status":"verified","manifest_version":1}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000005';

UPDATE model_catalog
SET enabled=1,available=1,
    metadata='{"source":"builtin","support_status":"verified","manifest_version":1}',
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE id='20000000-0000-0000-0000-000000000006'
  AND model_id='deepseek-reasoner';
