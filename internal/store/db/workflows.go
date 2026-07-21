package db

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) ListWorkflowTemplates(ctx context.Context, capability string) ([]WorkflowTemplate, error) {
	return collectRows[WorkflowTemplate](s.pool.Query(ctx, `
		SELECT * FROM workflow_templates
		WHERE ($1='' OR capability=$1)
		ORDER BY capability,name`, strings.TrimSpace(capability)))
}

func (s *Store) GetWorkflowTemplate(ctx context.Context, id uuid.UUID) (WorkflowTemplate, error) {
	return one[WorkflowTemplate](s.pool.Query(ctx, `SELECT * FROM workflow_templates WHERE id=$1`, id))
}

func (s *Store) CreateWorkflowTemplate(ctx context.Context, workflow WorkflowTemplate) (WorkflowTemplate, error) {
	workflow.InputModalities = nonNilStrings(workflow.InputModalities)
	if workflow.ID == uuid.Nil {
		workflow.ID = uuid.New()
	}
	return one[WorkflowTemplate](s.pool.Query(ctx, `
		INSERT INTO workflow_templates (
			id,code,name,description,capability,input_modalities,workflow,parameter_schema,
			default_parameters,bindings,outputs,requirements,enabled,checksum
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *`, workflow.ID,
		strings.ToLower(strings.TrimSpace(workflow.Code)), strings.TrimSpace(workflow.Name), strings.TrimSpace(workflow.Description),
		workflow.Capability, JSON(workflow.InputModalities), validJSON(workflow.Workflow), validJSON(workflow.ParameterSchema),
		validJSON(workflow.DefaultParameters), validJSON(workflow.Bindings), validJSON(workflow.Outputs),
		validJSON(workflow.Requirements), workflow.Enabled, workflow.Checksum))
}

func (s *Store) UpdateWorkflowTemplate(ctx context.Context, workflow WorkflowTemplate) (WorkflowTemplate, error) {
	workflow.InputModalities = nonNilStrings(workflow.InputModalities)
	var updated WorkflowTemplate
	err := withTx(ctx, s.pool, func(tx *Tx) error {
		var err error
		updated, err = one[WorkflowTemplate](tx.Query(ctx, `
			UPDATE workflow_templates SET
				code=$2,name=$3,description=$4,capability=$5,input_modalities=$6,workflow=$7,
				parameter_schema=$8,default_parameters=$9,bindings=$10,outputs=$11,requirements=$12,
				enabled=$13,version=version+1,checksum=$14,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE id=$1 RETURNING *`, workflow.ID, strings.ToLower(strings.TrimSpace(workflow.Code)), strings.TrimSpace(workflow.Name),
			strings.TrimSpace(workflow.Description), workflow.Capability, JSON(workflow.InputModalities), validJSON(workflow.Workflow),
			validJSON(workflow.ParameterSchema), validJSON(workflow.DefaultParameters), validJSON(workflow.Bindings),
			validJSON(workflow.Outputs), validJSON(workflow.Requirements), workflow.Enabled, workflow.Checksum))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM workflow_compatibilities WHERE workflow_template_id=$1`, workflow.ID)
		return err
	})
	return updated, err
}

func (s *Store) SetWorkflowTemplateEnabled(ctx context.Context, id uuid.UUID, enabled bool) (WorkflowTemplate, error) {
	return one[WorkflowTemplate](s.pool.Query(ctx, `
		UPDATE workflow_templates SET enabled=$2,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE id=$1 RETURNING *`, id, enabled))
}

func (s *Store) DeleteWorkflowTemplate(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM workflow_templates WHERE id=$1`, id)
	if err == nil {
		if changed, _ := result.RowsAffected(); changed == 0 {
			return ErrNotFound
		}
	}
	return err
}

func (s *Store) ListWorkflowCompatibilities(ctx context.Context, workflowID *uuid.UUID, providerID *uuid.UUID) ([]WorkflowCompatibility, error) {
	return collectRows[WorkflowCompatibility](s.pool.Query(ctx, `
		SELECT * FROM workflow_compatibilities
		WHERE ($1 IS NULL OR workflow_template_id=$1)
		  AND ($2 IS NULL OR provider_id=$2)
		ORDER BY checked_at DESC`, workflowID, providerID))
}

func (s *Store) UpsertWorkflowCompatibility(ctx context.Context, item WorkflowCompatibility) (WorkflowCompatibility, error) {
	return one[WorkflowCompatibility](s.pool.Query(ctx, `
		INSERT INTO workflow_compatibilities (workflow_template_id,provider_id,status,report)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (workflow_template_id,provider_id) DO UPDATE
		SET status=EXCLUDED.status,report=EXCLUDED.report,checked_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
		RETURNING *`, item.WorkflowTemplateID, item.ProviderID, item.Status, validJSON(item.Report)))
}
