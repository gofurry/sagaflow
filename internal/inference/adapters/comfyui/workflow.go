package comfyui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gofurry/sagaflow/internal/inference"
)

func DecodeWorkflowSpec(raw json.RawMessage) (WorkflowSpec, error) {
	var spec WorkflowSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return WorkflowSpec{}, fmt.Errorf("decode workflow target: %w", err)
	}
	if len(spec.Workflow) == 0 {
		return WorkflowSpec{}, fmt.Errorf("workflow graph is required")
	}
	for id, node := range spec.Workflow {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(node.ClassType) == "" || node.Inputs == nil {
			return WorkflowSpec{}, fmt.Errorf("workflow node %q is incomplete", id)
		}
	}
	for name, binding := range spec.Bindings {
		if strings.TrimSpace(binding.NodeID) == "" || strings.TrimSpace(binding.Input) == "" {
			return WorkflowSpec{}, fmt.Errorf("binding %q must define node_id and input", name)
		}
		if _, ok := spec.Workflow[binding.NodeID]; !ok {
			return WorkflowSpec{}, fmt.Errorf("binding %q references missing node %q", name, binding.NodeID)
		}
		switch binding.Source {
		case "prompt":
		case "parameter":
			if strings.TrimSpace(binding.Parameter) == "" {
				return WorkflowSpec{}, fmt.Errorf("parameter binding %q must define parameter", name)
			}
		case "input":
			if binding.InputIndex == nil || *binding.InputIndex < 0 {
				return WorkflowSpec{}, fmt.Errorf("input binding %q must define a non-negative input_index", name)
			}
		default:
			return WorkflowSpec{}, fmt.Errorf("binding %q has unsupported source %q", name, binding.Source)
		}
	}
	if len(spec.Outputs) == 0 {
		return WorkflowSpec{}, fmt.Errorf("at least one workflow output is required")
	}
	for index, output := range spec.Outputs {
		if _, ok := spec.Workflow[output.NodeID]; !ok {
			return WorkflowSpec{}, fmt.Errorf("output %d references missing node %q", index, output.NodeID)
		}
		if strings.TrimSpace(output.Key) == "" || !validMediaType(output.MediaType) {
			return WorkflowSpec{}, fmt.Errorf("output %d must define key and valid media_type", index)
		}
	}
	return spec, nil
}

func EvaluateCompatibility(spec WorkflowSpec, discovery Discovery) (string, CompatibilityReport) {
	availableNodes := make(map[string]struct{}, len(discovery.Nodes))
	for _, name := range discovery.Nodes {
		availableNodes[name] = struct{}{}
	}
	requiredNodes := make(map[string]struct{}, len(spec.Requirements.Nodes)+len(spec.Workflow))
	for _, name := range spec.Requirements.Nodes {
		if name = strings.TrimSpace(name); name != "" {
			requiredNodes[name] = struct{}{}
		}
	}
	for _, node := range spec.Workflow {
		requiredNodes[node.ClassType] = struct{}{}
	}
	report := CompatibilityReport{CheckedAt: time.Now().UTC()}
	for name := range requiredNodes {
		report.RequiredNodes = append(report.RequiredNodes, name)
		if _, ok := availableNodes[name]; !ok {
			report.MissingNodes = append(report.MissingNodes, name)
		}
	}
	for _, resource := range spec.Requirements.Models {
		if !contains(discovery.Models[resource.Folder], resource.Name) {
			report.MissingResources = append(report.MissingResources, resource)
		}
	}
	sort.Strings(report.RequiredNodes)
	sort.Strings(report.MissingNodes)
	if len(report.MissingNodes) > 0 {
		return "missing_nodes", report
	}
	if len(report.MissingResources) > 0 {
		return "missing_resources", report
	}
	return "ready", report
}

func bindWorkflow(spec WorkflowSpec, request inference.Request, uploads []uploadResponse) (map[string]WorkflowNode, error) {
	for name, binding := range spec.Bindings {
		var value any
		var present bool
		switch binding.Source {
		case "prompt":
			value, present = request.Prompt, strings.TrimSpace(request.Prompt) != ""
		case "parameter":
			value, present = request.Parameters[binding.Parameter]
		case "input":
			if binding.InputIndex != nil && *binding.InputIndex < len(uploads) {
				upload := uploads[*binding.InputIndex]
				value = upload.Name
				if upload.Subfolder != "" {
					value = strings.Trim(upload.Subfolder, "/\\") + "/" + upload.Name
				}
				present = true
			}
		}
		if !present {
			if binding.Required {
				return nil, fmt.Errorf("required workflow binding %q has no value", name)
			}
			continue
		}
		node := spec.Workflow[binding.NodeID]
		node.Inputs[binding.Input] = value
		spec.Workflow[binding.NodeID] = node
	}
	return spec.Workflow, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validMediaType(value string) bool {
	switch value {
	case "text", "image", "audio", "video", "file":
		return true
	}
	return false
}
