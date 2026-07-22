-- +goose Up
CREATE TABLE workflow_compatibilities_next (
  workflow_template_id TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('unknown','ready','missing_nodes','missing_resources','incompatible','error')),
  report TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(report)),
  checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY(workflow_template_id, provider_id)
);

INSERT INTO workflow_compatibilities_next (workflow_template_id,provider_id,status,report,checked_at)
SELECT workflow_template_id,
       provider_id,
       CASE status WHEN 'compatible' THEN 'ready' ELSE status END,
       report,
       checked_at
FROM workflow_compatibilities;

DROP TABLE workflow_compatibilities;
ALTER TABLE workflow_compatibilities_next RENAME TO workflow_compatibilities;

-- +goose Down
CREATE TABLE workflow_compatibilities_previous (
  workflow_template_id TEXT NOT NULL REFERENCES workflow_templates(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL REFERENCES model_providers(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('unknown','compatible','incompatible','error')),
  report TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(report)),
  checked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY(workflow_template_id, provider_id)
);

INSERT INTO workflow_compatibilities_previous (workflow_template_id,provider_id,status,report,checked_at)
SELECT workflow_template_id,
       provider_id,
       CASE status WHEN 'ready' THEN 'compatible' WHEN 'missing_nodes' THEN 'incompatible' WHEN 'missing_resources' THEN 'incompatible' ELSE status END,
       report,
       checked_at
FROM workflow_compatibilities;

DROP TABLE workflow_compatibilities;
ALTER TABLE workflow_compatibilities_previous RENAME TO workflow_compatibilities;
