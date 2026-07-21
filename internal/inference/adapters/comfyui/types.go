// Package comfyui implements SagaFlow's provider-neutral ComfyUI workflow adapter.
package comfyui

import (
	"encoding/json"
	"net/http"
	"time"
)

type Config struct {
	HTTPClient   HTTPDoer
	PollInterval time.Duration
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ServerInfo struct {
	Version    string         `json:"version"`
	System     map[string]any `json:"system"`
	Devices    []DeviceInfo   `json:"devices"`
	Features   map[string]any `json:"features"`
	NodeCount  int            `json:"node_count"`
	ModelCount int            `json:"model_count"`
}

type DeviceInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Index     *int   `json:"index"`
	VRAMTotal int64  `json:"vram_total"`
	VRAMFree  int64  `json:"vram_free"`
}

type Discovery struct {
	Server          ServerInfo                 `json:"server"`
	Nodes           []string                   `json:"nodes"`
	Models          map[string][]string        `json:"models"`
	NodeDefinitions map[string]json.RawMessage `json:"-"`
}

type WorkflowSpec struct {
	Workflow     map[string]WorkflowNode `json:"workflow"`
	Bindings     map[string]Binding      `json:"bindings"`
	Outputs      []OutputSelector        `json:"outputs"`
	Requirements Requirements            `json:"requirements"`
	Version      int32                   `json:"version,omitempty"`
	Checksum     string                  `json:"checksum,omitempty"`
}

type WorkflowNode struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

type Binding struct {
	NodeID     string `json:"node_id"`
	Input      string `json:"input"`
	Source     string `json:"source"`
	Parameter  string `json:"parameter,omitempty"`
	InputIndex *int   `json:"input_index,omitempty"`
	Required   bool   `json:"required,omitempty"`
}

type OutputSelector struct {
	NodeID    string `json:"node_id"`
	Key       string `json:"key"`
	MediaType string `json:"media_type"`
	MIMEType  string `json:"mime_type,omitempty"`
}

type Requirements struct {
	Nodes  []string           `json:"nodes"`
	Models []ModelRequirement `json:"models"`
}

type ModelRequirement struct {
	Folder string `json:"folder"`
	Name   string `json:"name"`
}

type CompatibilityReport struct {
	MissingNodes     []string           `json:"missing_nodes"`
	MissingResources []ModelRequirement `json:"missing_resources"`
	RequiredNodes    []string           `json:"required_nodes"`
	CheckedAt        time.Time          `json:"checked_at"`
}

type systemStatsResponse struct {
	System  map[string]any `json:"system"`
	Devices []DeviceInfo   `json:"devices"`
}

type promptResponse struct {
	PromptID   string                     `json:"prompt_id"`
	Number     float64                    `json:"number"`
	Error      map[string]any             `json:"error"`
	NodeErrors map[string]json.RawMessage `json:"node_errors"`
}

type uploadResponse struct {
	Name      string `json:"name"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}
