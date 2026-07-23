package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestOpenMigratesCleanDatabaseAndEnforcesSingleAccount(t *testing.T) {
	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "sagaflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO account (id,username,display_name,password_hash) VALUES (?,?,?,?)`, uuid.NewString(), "creator", "Creator", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO account (id,username,display_name,password_hash) VALUES (?,?,?,?)`, uuid.NewString(), "other", "Other", "hash"); err == nil {
		t.Fatal("expected the database to reject a second account")
	}
	var mode string
	if err := database.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %q", mode)
	}

	var localProviders int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM model_providers
		WHERE (adapter_code='ollama' AND base_url='http://127.0.0.1:11434' AND auth_type='none')
		   OR (adapter_code='comfyui' AND base_url='http://127.0.0.1:8188' AND auth_type='none')`).Scan(&localProviders); err != nil {
		t.Fatal(err)
	}
	if localProviders != 3 {
		t.Fatalf("expected Ollama and two built-in ComfyUI connections, got %d", localProviders)
	}
	var bailianProviders int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM model_providers
		WHERE code='aliyun_bailian' AND adapter_code='aliyun_bailian'
		  AND base_url='https://dashscope.aliyuncs.com' AND auth_type='api_key'`).Scan(&bailianProviders); err != nil {
		t.Fatal(err)
	}
	if bailianProviders != 1 {
		t.Fatalf("expected built-in Bailian connection, got %d", bailianProviders)
	}
	var siliconFlowProviders int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM model_providers
		WHERE code='siliconflow' AND adapter_code='siliconflow'
		  AND base_url='https://api.siliconflow.cn/v1' AND auth_type='api_key'`).Scan(&siliconFlowProviders); err != nil {
		t.Fatal(err)
	}
	if siliconFlowProviders != 1 {
		t.Fatalf("expected built-in SiliconFlow connection, got %d", siliconFlowProviders)
	}
	var zhipuProviders int
	if err := database.QueryRow(`
		SELECT COUNT(*) FROM model_providers
		WHERE code='zhipu' AND adapter_code='zhipu'
		  AND base_url='https://open.bigmodel.cn/api/paas/v4' AND auth_type='api_key'`).Scan(&zhipuProviders); err != nil {
		t.Fatal(err)
	}
	if zhipuProviders != 1 {
		t.Fatalf("expected built-in Zhipu connection, got %d", zhipuProviders)
	}
	if _, err := database.Exec(`
		INSERT INTO workflow_templates (
			id,code,name,capability,input_modalities,workflow,parameter_schema,
			default_parameters,bindings,outputs,requirements,checksum
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		uuid.NewString(), "local-image", "Local image", "image", "[]", `{}`, `{}`, `{}`, `{}`, `[]`, `{}`, "checksum"); err != nil {
		t.Fatal(err)
	}
	var workflowID string
	if err := database.QueryRow(`SELECT id FROM workflow_templates WHERE code='local-image'`).Scan(&workflowID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO workflow_compatibilities (workflow_template_id,provider_id,status,report)
		VALUES (?,?,?,?)`, workflowID, "10000000-0000-0000-0000-000000000006", "ready", `{}`); err != nil {
		t.Fatalf("expected the current workflow compatibility status to be accepted: %v", err)
	}
	for _, table := range []string{"prompt_presets", "voice_profiles"} {
		rows, err := database.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if name == "project_id" {
				rows.Close()
				t.Fatalf("%s must be globally scoped", table)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
