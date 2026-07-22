-- +goose Up
UPDATE model_providers
SET base_url='http://127.0.0.1:8188', updated_at=CURRENT_TIMESTAMP
WHERE code='comfyui-aki-local'
  AND base_url='http://127.0.0.1:8189';

-- +goose Down
UPDATE model_providers
SET base_url='http://127.0.0.1:8189', updated_at=CURRENT_TIMESTAMP
WHERE code='comfyui-aki-local'
  AND base_url='http://127.0.0.1:8188';
