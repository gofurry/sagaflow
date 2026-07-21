package service

import (
	"encoding/json"
	"testing"
)

func TestOllamaParameterSchemaFollowsModelCapabilities(t *testing.T) {
	withoutThinking := schemaProperties(t, ollamaParameterSchema([]string{"completion"}))
	if _, ok := withoutThinking["think"]; ok {
		t.Fatal("non-thinking model unexpectedly exposes think parameter")
	}
	withThinking := schemaProperties(t, ollamaParameterSchema([]string{"completion", "thinking"}))
	if _, ok := withThinking["think"]; !ok {
		t.Fatal("thinking model does not expose think parameter")
	}
	if len(withThinking) <= len(withoutThinking) {
		t.Fatal("thinking schema did not add a capability-specific parameter")
	}
}

func TestOllamaDefaultsDoNotOverrideModelfile(t *testing.T) {
	var defaults map[string]any
	if err := json.Unmarshal(ollamaDefaultParameters(), &defaults); err != nil {
		t.Fatalf("decode defaults: %v", err)
	}
	if len(defaults) != 0 {
		t.Fatalf("Ollama defaults = %#v, want empty defaults", defaults)
	}
}

func schemaProperties(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema.Properties
}
