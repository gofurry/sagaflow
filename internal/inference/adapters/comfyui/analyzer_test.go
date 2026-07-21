package comfyui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnalyzeWorkflowInfersBindingsParametersOutputsAndModels(t *testing.T) {
	workflow := map[string]any{
		"1":  map[string]any{"class_type": "CheckpointLoaderSimple", "inputs": map[string]any{"ckpt_name": "anything.safetensors"}},
		"2":  map[string]any{"class_type": "LoraLoader", "inputs": map[string]any{"model": []any{"1", 0}, "clip": []any{"1", 1}, "lora_name": "portrait.safetensors", "strength_model": 0.8, "strength_clip": 0.7}},
		"3":  map[string]any{"class_type": "KSampler", "inputs": map[string]any{"model": []any{"2", 0}, "positive": []any{"6", 0}, "negative": []any{"7", 0}, "latent_image": []any{"4", 0}, "seed": 42, "steps": 20, "cfg": 7.0, "sampler_name": "euler", "scheduler": "normal", "denoise": 0.75}},
		"4":  map[string]any{"class_type": "VAEEncode", "inputs": map[string]any{"pixels": []any{"10", 0}, "vae": []any{"1", 2}}},
		"6":  map[string]any{"class_type": "CLIPTextEncode", "inputs": map[string]any{"clip": []any{"2", 1}, "text": "old positive"}, "_meta": map[string]any{"title": "正向提示词"}},
		"7":  map[string]any{"class_type": "CLIPTextEncode", "inputs": map[string]any{"clip": []any{"2", 1}, "text": "low quality"}, "_meta": map[string]any{"title": "Negative Prompt"}},
		"9":  map[string]any{"class_type": "SaveImage", "inputs": map[string]any{"images": []any{"11", 0}, "filename_prefix": "SagaFlow"}},
		"10": map[string]any{"class_type": "LoadImage", "inputs": map[string]any{"image": "input.png"}},
		"11": map[string]any{"class_type": "VAEDecode", "inputs": map[string]any{"samples": []any{"3", 0}, "vae": []any{"1", 2}}},
	}
	discovery := &Discovery{
		Nodes:  []string{"CheckpointLoaderSimple", "LoraLoader", "KSampler", "VAEEncode", "CLIPTextEncode", "SaveImage", "LoadImage", "VAEDecode"},
		Models: map[string][]string{"checkpoints": {"anything.safetensors"}, "loras": {"portrait.safetensors"}},
	}
	analysis, err := AnalyzeWorkflow(mustAnalyzerJSON(t, workflow), discovery)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Capability != "image" || analysis.Summary.NodeCount != 9 || analysis.Summary.InputCount != 1 || analysis.Summary.OutputCount != 1 {
		t.Fatalf("unexpected summary: %#v", analysis)
	}
	if binding := analysis.Bindings["prompt"]; binding.NodeID != "6" || binding.Source != "prompt" || !binding.Required {
		t.Fatalf("positive prompt was not inferred: %#v", binding)
	}
	if binding := analysis.Bindings["negative_prompt"]; binding.NodeID != "7" || binding.Parameter != "negative_prompt" {
		t.Fatalf("negative prompt was not inferred: %#v", binding)
	}
	if binding := analysis.Bindings["input_image"]; binding.NodeID != "10" || binding.InputIndex == nil || *binding.InputIndex != 0 {
		t.Fatalf("image input was not inferred: %#v", binding)
	}
	for _, parameter := range []string{"negative_prompt", "seed", "steps", "cfg", "sampler_name", "scheduler", "denoise", "strength_model", "strength_clip"} {
		if _, ok := analysis.DefaultParameters[parameter]; !ok {
			t.Fatalf("missing suggested parameter %q: %#v", parameter, analysis.DefaultParameters)
		}
	}
	if output := analysis.Outputs[0]; output.NodeID != "9" || output.Key != "images" || output.MediaType != "image" {
		t.Fatalf("unexpected output selector: %#v", output)
	}
	if len(analysis.Requirements.Models) != 2 {
		t.Fatalf("model dependencies were not inferred: %#v", analysis.Requirements.Models)
	}
	for _, issue := range analysis.Issues {
		if issue.Level == "warning" {
			t.Fatalf("valid graph unexpectedly produced warning: %#v", issue)
		}
	}
}

func TestAnalyzeWorkflowAcceptsPromptWrapperAndSplitsStageDefaults(t *testing.T) {
	graph := map[string]any{
		"1": map[string]any{"class_type": "KSampler", "inputs": map[string]any{"steps": 12}},
		"2": map[string]any{"class_type": "KSampler", "inputs": map[string]any{"steps": 30}},
		"3": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{}},
	}
	analysis, err := AnalyzeWorkflow(mustAnalyzerJSON(t, map[string]any{"prompt": graph, "client_id": "desktop"}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := analysis.DefaultParameters["node_1_steps"]; !ok {
		t.Fatalf("first stage parameter was not split: %#v", analysis.DefaultParameters)
	}
	if _, ok := analysis.DefaultParameters["node_2_steps"]; !ok {
		t.Fatalf("second stage parameter was not split: %#v", analysis.DefaultParameters)
	}
	if !hasAnalysisIssue(analysis.Issues, "split_stage_parameter") || !hasAnalysisIssue(analysis.Issues, "prompt_not_found") {
		t.Fatalf("expected ambiguity issues: %#v", analysis.Issues)
	}
}

func TestAnalyzeWorkflowRejectsEditorFormat(t *testing.T) {
	_, err := AnalyzeWorkflow(json.RawMessage(`{"nodes":[],"links":[]}`), nil)
	if err == nil || !strings.Contains(err.Error(), "API format") {
		t.Fatalf("expected API format guidance, got %v", err)
	}
}

func TestAnalyzeWorkflowUsesLiveNodeDefinitionsForCustomParameters(t *testing.T) {
	graph := map[string]any{
		"1": map[string]any{"class_type": "CustomEnhancer", "inputs": map[string]any{"quality": "high", "iterations": 3}},
		"2": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{}},
	}
	discovery := &Discovery{
		Nodes: []string{"CustomEnhancer", "SaveImage"}, Models: map[string][]string{},
		NodeDefinitions: map[string]json.RawMessage{
			"CustomEnhancer": json.RawMessage(`{"input":{"required":{"quality":[["low","high"]],"iterations":["INT",{"min":1,"max":10,"tooltip":"Enhancement passes"}]}},"output_node":false}`),
			"SaveImage":      json.RawMessage(`{"input":{"required":{}},"output_node":true}`),
		},
	}
	analysis, err := AnalyzeWorkflow(mustAnalyzerJSON(t, graph), discovery)
	if err != nil {
		t.Fatal(err)
	}
	for _, parameter := range []string{"quality", "iterations"} {
		if _, ok := analysis.DefaultParameters[parameter]; !ok {
			t.Fatalf("custom parameter %q was not inferred: %#v", parameter, analysis.DefaultParameters)
		}
	}
	quality := properties(analysis.ParameterSchema)["quality"].(map[string]any)
	if values, ok := quality["enum"].([]any); !ok || len(values) != 2 {
		t.Fatalf("custom enum was not preserved: %#v", quality)
	}
	iterations := properties(analysis.ParameterSchema)["iterations"].(map[string]any)
	if iterations["minimum"] != float64(1) || iterations["maximum"] != float64(10) {
		t.Fatalf("custom numeric bounds were not preserved: %#v", iterations)
	}
}

func TestAnalyzeWorkflowWarnsAboutDanglingConnections(t *testing.T) {
	graph := map[string]any{
		"1": map[string]any{"class_type": "KSampler", "inputs": map[string]any{"model": []any{"404", 0}, "steps": 12}},
		"2": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{}},
	}
	analysis, err := AnalyzeWorkflow(mustAnalyzerJSON(t, graph), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAnalysisIssue(analysis.Issues, "dangling_connections") {
		t.Fatalf("dangling connection was not reported: %#v", analysis.Issues)
	}
}

func mustAnalyzerJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hasAnalysisIssue(issues []AnalysisIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
