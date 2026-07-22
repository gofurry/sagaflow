-- +goose Up
ALTER TABLE canvas_annotations
ADD COLUMN label_position TEXT NOT NULL DEFAULT 'center'
CHECK (label_position IN (
  'top-left','top-center','top-right',
  'middle-left','center','middle-right',
  'bottom-left','bottom-center','bottom-right'
));

-- +goose Down
ALTER TABLE canvas_annotations DROP COLUMN label_position;
