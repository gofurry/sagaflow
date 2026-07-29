package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateModelRequestAcceptsSupportedParameterControls(t *testing.T) {
	req := modelRequest{
		ProviderID: uuid.New(), ModelID: "custom-model", DisplayName: "Custom model", Capability: "text",
		InputModalities: []string{"text", "image"}, Features: []string{"completion", "vision"},
		ParameterSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"temperature":{"type":"number","title":"温度","minimum":0,"maximum":2,"multipleOf":0.1},
				"max_tokens":{"type":"integer","minimum":1,"maximum":32768},
				"response_format":{"type":"string","enum":["text","json_object"]},
				"stream":{"type":"boolean"},
				"stop":{"type":"array","items":{"type":"string"}},
				"metadata":{"type":"object"},
				"notes":{"type":"string","format":"textarea","readOnly":true}
			}
		}`),
		DefaultParameters: json.RawMessage(`{"temperature":0.7,"max_tokens":4096,"response_format":"text","stream":false,"stop":[],"metadata":{}}`),
	}
	if err := validateModelRequest(req); err != nil {
		t.Fatal(err)
	}
}

func TestValidateModelRequestRejectsUnsupportedSchemaAndDefaults(t *testing.T) {
	base := modelRequest{ProviderID: uuid.New(), ModelID: "custom", DisplayName: "Custom", Capability: "text"}
	tests := []struct {
		name     string
		schema   string
		defaults string
		want     string
	}{
		{name: "unsupported schema keyword", schema: `{"type":"object","properties":{"value":{"type":"string","oneOf":[]}}}`, defaults: `{}`, want: "unsupported field"},
		{name: "unknown default", schema: `{"type":"object","properties":{}}`, defaults: `{"temperature":1}`, want: "not declared"},
		{name: "wrong default type", schema: `{"type":"object","properties":{"count":{"type":"integer"}}}`, defaults: `{"count":1.5}`, want: "must be integer"},
		{name: "default outside range", schema: `{"type":"object","properties":{"temperature":{"type":"number","minimum":0,"maximum":1}}}`, defaults: `{"temperature":2}`, want: "at most"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := base
			req.ParameterSchema = json.RawMessage(test.schema)
			req.DefaultParameters = json.RawMessage(test.defaults)
			err := validateModelRequest(req)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestUserModelMetadataCannotClaimBuiltInOwnership(t *testing.T) {
	metadata := userModelMetadata(json.RawMessage(`{"source":"builtin","support_status":"experimental","custom":true}`))
	if catalogManagedModel(metadata) {
		t.Fatal("user-created model retained built-in ownership")
	}
	var decoded map[string]any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["source"] != "user" || decoded["custom"] != true {
		t.Fatalf("unexpected metadata: %#v", decoded)
	}
}
