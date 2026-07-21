package comfyui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// WorkflowAnalysis is an editable import suggestion. The analyzer is
// deliberately conservative: it never rewrites the graph and reports
// ambiguous decisions so the caller can ask the user to confirm them.
type WorkflowAnalysis struct {
	Workflow          map[string]WorkflowNode `json:"workflow"`
	Capability        string                  `json:"capability"`
	InputModalities   []string                `json:"input_modalities"`
	ParameterSchema   map[string]any          `json:"parameter_schema"`
	DefaultParameters map[string]any          `json:"default_parameters"`
	Bindings          map[string]Binding      `json:"bindings"`
	Outputs           []OutputSelector        `json:"outputs"`
	Requirements      Requirements            `json:"requirements"`
	Issues            []AnalysisIssue         `json:"issues"`
	Summary           AnalysisSummary         `json:"summary"`
}

type AnalysisIssue struct {
	Level   string   `json:"level"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

type AnalysisSummary struct {
	NodeCount      int `json:"node_count"`
	ParameterCount int `json:"parameter_count"`
	BindingCount   int `json:"binding_count"`
	InputCount     int `json:"input_count"`
	OutputCount    int `json:"output_count"`
	ModelCount     int `json:"model_count"`
}

type parameterOccurrence struct {
	nodeID string
	input  string
	value  any
	title  string
	schema map[string]any
}

// AnalyzeWorkflow accepts a ComfyUI API-format graph or a /prompt request
// wrapper. Discovery is optional and improves missing-node and model-folder
// detection when a live ComfyUI connection is available.
func AnalyzeWorkflow(raw json.RawMessage, discovery *Discovery) (WorkflowAnalysis, error) {
	graph, err := decodeWorkflowGraph(raw)
	if err != nil {
		return WorkflowAnalysis{}, err
	}
	ids := sortedNodeIDs(graph)
	analysis := WorkflowAnalysis{
		Workflow: graph, Capability: "image",
		ParameterSchema:   map[string]any{"type": "object", "properties": map[string]any{}},
		DefaultParameters: map[string]any{}, Bindings: map[string]Binding{},
		Outputs: []OutputSelector{}, Requirements: Requirements{Nodes: []string{}, Models: []ModelRequirement{}},
		Issues: []AnalysisIssue{},
	}

	nodeSet := map[string]struct{}{}
	for _, id := range ids {
		nodeSet[graph[id].ClassType] = struct{}{}
	}
	for name := range nodeSet {
		analysis.Requirements.Nodes = append(analysis.Requirements.Nodes, name)
	}
	sort.Strings(analysis.Requirements.Nodes)
	missingLinks := map[string]struct{}{}
	for _, id := range ids {
		for _, input := range graph[id].Inputs {
			linkedID, linked := strictConnectionNodeID(input)
			if linked {
				if _, exists := graph[linkedID]; !exists {
					missingLinks[linkedID] = struct{}{}
				}
			}
		}
	}
	if missing := sortedSet(missingLinks); len(missing) > 0 {
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "dangling_connections", Message: "工作流连线引用了不存在的节点：" + strings.Join(missing, "、"), NodeIDs: missing})
	}

	positive, negative := promptNodes(graph, ids)
	claimed := map[string]struct{}{}
	for index, nodeID := range positive {
		name := uniqueBindingName(analysis.Bindings, "prompt", index)
		analysis.Bindings[name] = Binding{NodeID: nodeID, Input: "text", Source: "prompt", Required: true}
		claimed[nodeID+".text"] = struct{}{}
	}
	if len(positive) == 0 {
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "prompt_not_found", Message: "没有可靠识别到正向 Prompt 节点，请手工补充 Prompt 绑定。"})
	} else if len(positive) > 1 {
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "info", Code: "multiple_prompt_nodes", Message: "识别到多个正向 Prompt 节点，已让它们共用生成页 Prompt。", NodeIDs: positive})
	}
	if len(negative) > 0 {
		defaultValue := ""
		if value, ok := graph[negative[0]].Inputs["text"].(string); ok {
			defaultValue = value
		}
		properties(analysis.ParameterSchema)["negative_prompt"] = map[string]any{"type": "string", "title": "反向 Prompt", "format": "textarea", "description": "写入工作流识别到的反向文本编码节点。"}
		analysis.DefaultParameters["negative_prompt"] = defaultValue
		for index, nodeID := range negative {
			name := uniqueBindingName(analysis.Bindings, "negative_prompt", index)
			analysis.Bindings[name] = Binding{NodeID: nodeID, Input: "text", Source: "parameter", Parameter: "negative_prompt"}
			claimed[nodeID+".text"] = struct{}{}
		}
	}

	inputIndex := 0
	for _, id := range ids {
		node := graph[id]
		className := strings.ToLower(node.ClassType)
		switch {
		case strings.Contains(className, "loadimage"):
			if value, ok := node.Inputs["image"]; ok && !isConnection(value) {
				index := inputIndex
				name := uniqueBindingName(analysis.Bindings, "input_image", inputIndex)
				analysis.Bindings[name] = Binding{NodeID: id, Input: "image", Source: "input", InputIndex: &index, Required: true}
				claimed[id+".image"] = struct{}{}
				inputIndex++
			}
		case strings.Contains(className, "loadaudio"), strings.Contains(className, "loadvideo"):
			analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "unsupported_input_media", Message: fmt.Sprintf("节点 %s（%s）需要非图像输入，当前 Adapter 不能自动上传这种媒介。", id, node.ClassType), NodeIDs: []string{id}})
		}
	}
	if inputIndex > 0 {
		analysis.InputModalities = []string{"image"}
		if inputIndex > 1 {
			analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "info", Code: "ordered_image_inputs", Message: fmt.Sprintf("识别到 %d 个图像输入，生成页参考图将按绑定顺序依次写入。", inputIndex)})
		}
	}

	occurrences := map[string][]parameterOccurrence{}
	for _, id := range ids {
		node := graph[id]
		for input, value := range node.Inputs {
			if _, ok := claimed[id+"."+input]; ok || isConnection(value) {
				continue
			}
			schema, ok := suggestedParameter(input, value, node, discovery)
			if !ok {
				continue
			}
			occurrences[input] = append(occurrences[input], parameterOccurrence{
				nodeID: id, input: input, value: value, title: nodeTitle(node, id), schema: schema,
			})
		}
	}
	parameterNames := make([]string, 0, len(occurrences))
	for name := range occurrences {
		parameterNames = append(parameterNames, name)
	}
	sort.Strings(parameterNames)
	for _, name := range parameterNames {
		items := occurrences[name]
		if equalOccurrenceDefaults(items) {
			addSuggestedParameter(&analysis, name, items[0].value, items[0].schema, items)
			continue
		}
		for _, item := range items {
			key := uniqueParameterName(analysis.DefaultParameters, "node_"+sanitizeIdentifier(item.nodeID)+"_"+sanitizeIdentifier(name))
			schema := cloneMap(item.schema)
			schema["title"] = fmt.Sprintf("%s · %s", item.title, parameterTitle(name))
			addSuggestedParameter(&analysis, key, item.value, schema, []parameterOccurrence{item})
		}
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "info", Code: "split_stage_parameter", Message: fmt.Sprintf("多个节点的 %s 默认值不同，已拆分为独立参数。", name)})
	}

	for _, id := range ids {
		if selector, ok := suggestedOutput(id, graph[id]); ok {
			analysis.Outputs = append(analysis.Outputs, selector)
		}
	}
	if discovery != nil {
		selectedOutputs := map[string]struct{}{}
		for _, output := range analysis.Outputs {
			selectedOutputs[output.NodeID] = struct{}{}
		}
		for _, id := range ids {
			if _, selected := selectedOutputs[id]; selected || !declaredOutputNode(discovery.NodeDefinitions[graph[id].ClassType]) {
				continue
			}
			analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "unknown_output_node", Message: fmt.Sprintf("节点 %s（%s）是输出节点，但助手无法确定历史输出键，请手工确认输出选择器。", id, graph[id].ClassType), NodeIDs: []string{id}})
		}
	}
	analysis.Capability = outputCapability(analysis.Outputs)
	if len(analysis.Outputs) == 0 {
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "output_not_found", Message: "没有识别到标准保存节点，保存模板前必须手工配置输出选择器。"})
	} else if len(analysis.Outputs) > 1 {
		analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "info", Code: "multiple_outputs", Message: "识别到多个可能的最终输出，请确认是否都需要回存到暂存区。", NodeIDs: outputNodeIDs(analysis.Outputs)})
	}

	analysis.Requirements.Models = inferModelRequirements(graph, ids, discovery, &analysis.Issues)
	if discovery != nil {
		available := map[string]struct{}{}
		for _, name := range discovery.Nodes {
			available[name] = struct{}{}
		}
		missing := []string{}
		for _, name := range analysis.Requirements.Nodes {
			if _, ok := available[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			analysis.Issues = append(analysis.Issues, AnalysisIssue{Level: "warning", Code: "missing_nodes", Message: "所选连接缺少节点：" + strings.Join(missing, "、")})
		}
	}

	analysis.Summary = AnalysisSummary{
		NodeCount: len(graph), ParameterCount: len(analysis.DefaultParameters), BindingCount: len(analysis.Bindings),
		InputCount: inputIndex, OutputCount: len(analysis.Outputs), ModelCount: len(analysis.Requirements.Models),
	}
	return analysis, nil
}

func decodeWorkflowGraph(raw json.RawMessage) (map[string]WorkflowNode, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode ComfyUI workflow JSON: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("workflow must be a non-empty ComfyUI API-format graph")
	}
	var editorNodes []any
	if nodes, ok := root["nodes"]; ok && json.Unmarshal(nodes, &editorNodes) == nil {
		return nil, fmt.Errorf("this is a ComfyUI editor workflow; export the workflow in API format before importing")
	}
	graphRaw := raw
	for _, wrapper := range []string{"prompt", "workflow"} {
		if candidate, ok := root[wrapper]; ok {
			var graph map[string]WorkflowNode
			if json.Unmarshal(candidate, &graph) == nil && looksLikeGraph(graph) {
				graphRaw = candidate
				break
			}
		}
	}
	var graph map[string]WorkflowNode
	if err := json.Unmarshal(graphRaw, &graph); err != nil || !looksLikeGraph(graph) {
		return nil, fmt.Errorf("workflow must be a non-empty ComfyUI API-format graph")
	}
	for id, node := range graph {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(node.ClassType) == "" || node.Inputs == nil {
			return nil, fmt.Errorf("workflow node %q is incomplete; use ComfyUI API-format JSON", id)
		}
	}
	return graph, nil
}

func looksLikeGraph(graph map[string]WorkflowNode) bool {
	if len(graph) == 0 {
		return false
	}
	for _, node := range graph {
		if strings.TrimSpace(node.ClassType) == "" || node.Inputs == nil {
			return false
		}
	}
	return true
}

func promptNodes(graph map[string]WorkflowNode, ids []string) ([]string, []string) {
	positiveSet, negativeSet := map[string]struct{}{}, map[string]struct{}{}
	for _, id := range ids {
		node := graph[id]
		className := strings.ToLower(node.ClassType)
		if !strings.Contains(className, "sampler") {
			continue
		}
		for _, candidate := range upstreamTextNodes(graph, node.Inputs["positive"]) {
			positiveSet[candidate] = struct{}{}
		}
		for _, candidate := range upstreamTextNodes(graph, node.Inputs["negative"]) {
			negativeSet[candidate] = struct{}{}
		}
	}
	if len(positiveSet) == 0 && len(negativeSet) == 0 {
		for _, id := range ids {
			node := graph[id]
			if !isTextEncoder(node) {
				continue
			}
			label := strings.ToLower(nodeTitle(node, id) + " " + analyzerStringValue(node.Inputs["text"]))
			if strings.Contains(label, "negative") || strings.Contains(label, "负面") || strings.Contains(label, "反向") {
				negativeSet[id] = struct{}{}
			} else {
				positiveSet[id] = struct{}{}
			}
		}
	}
	return sortedSet(positiveSet), sortedSet(negativeSet)
}

func upstreamTextNodes(graph map[string]WorkflowNode, value any) []string {
	start, ok := connectionNodeID(value)
	if !ok {
		return nil
	}
	queue, visited, found := []string{start}, map[string]struct{}{}, map[string]struct{}{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}
		node, ok := graph[id]
		if !ok {
			continue
		}
		if isTextEncoder(node) {
			found[id] = struct{}{}
			continue
		}
		for _, input := range node.Inputs {
			if upstream, linked := connectionNodeID(input); linked {
				queue = append(queue, upstream)
			}
		}
	}
	return sortedSet(found)
}

func isTextEncoder(node WorkflowNode) bool {
	className := strings.ToLower(node.ClassType)
	_, hasText := node.Inputs["text"]
	return hasText && (strings.Contains(className, "textencode") || strings.Contains(className, "cliptext"))
}

func suggestedParameter(input string, value any, node WorkflowNode, discovery *Discovery) (map[string]any, bool) {
	if !isPrimitive(value) {
		return nil, false
	}
	rules := map[string]map[string]any{
		"seed":           {"type": "integer", "minimum": 0, "title": "随机种子"},
		"noise_seed":     {"type": "integer", "minimum": 0, "title": "噪声种子"},
		"steps":          {"type": "integer", "minimum": 1, "maximum": 200, "title": "采样步数"},
		"cfg":            {"type": "number", "minimum": 0, "maximum": 30, "title": "CFG"},
		"denoise":        {"type": "number", "minimum": 0, "maximum": 1, "title": "重绘强度"},
		"sampler_name":   {"type": "string", "title": "采样器"},
		"scheduler":      {"type": "string", "title": "调度器"},
		"width":          {"type": "integer", "minimum": 64, "maximum": 8192, "title": "宽度"},
		"height":         {"type": "integer", "minimum": 64, "maximum": 8192, "title": "高度"},
		"batch_size":     {"type": "integer", "minimum": 1, "maximum": 64, "title": "批次数量"},
		"guidance":       {"type": "number", "minimum": 0, "maximum": 30, "title": "引导强度"},
		"strength":       {"type": "number", "minimum": 0, "maximum": 2, "title": "控制强度"},
		"strength_model": {"type": "number", "minimum": -2, "maximum": 2, "title": "LoRA 模型强度"},
		"strength_clip":  {"type": "number", "minimum": -2, "maximum": 2, "title": "LoRA CLIP 强度"},
		"start_percent":  {"type": "number", "minimum": 0, "maximum": 1, "title": "开始比例"},
		"end_percent":    {"type": "number", "minimum": 0, "maximum": 1, "title": "结束比例"},
		"left":           {"type": "integer", "minimum": 0, "maximum": 8192, "title": "左侧扩展"},
		"right":          {"type": "integer", "minimum": 0, "maximum": 8192, "title": "右侧扩展"},
		"top":            {"type": "integer", "minimum": 0, "maximum": 8192, "title": "顶部扩展"},
		"bottom":         {"type": "integer", "minimum": 0, "maximum": 8192, "title": "底部扩展"},
		"feathering":     {"type": "integer", "minimum": 0, "maximum": 2048, "title": "羽化"},
	}
	schema, ok := rules[input]
	if !ok && discovery != nil && !technicalInput(input) && !isModelLiteral(input, value, node.ClassType, discovery) {
		schema, ok = schemaFromNodeDefinition(discovery.NodeDefinitions[node.ClassType], input)
	}
	if !ok {
		return nil, false
	}
	result := cloneMap(schema)
	result["description"] = fmt.Sprintf("%s · %s.%s", nodeTitle(node, ""), node.ClassType, input)
	return result, true
}

func schemaFromNodeDefinition(raw json.RawMessage, input string) (map[string]any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var definition struct {
		Input map[string]map[string]json.RawMessage `json:"input"`
	}
	if json.Unmarshal(raw, &definition) != nil {
		return nil, false
	}
	var spec []json.RawMessage
	for _, group := range []string{"required", "optional"} {
		if candidate, ok := definition.Input[group][input]; ok && json.Unmarshal(candidate, &spec) == nil && len(spec) > 0 {
			break
		}
	}
	if len(spec) == 0 {
		return nil, false
	}
	result := map[string]any{"title": parameterTitle(input)}
	var kind string
	if json.Unmarshal(spec[0], &kind) == nil {
		switch strings.ToUpper(kind) {
		case "INT":
			result["type"] = "integer"
		case "FLOAT", "NUMBER":
			result["type"] = "number"
		case "BOOLEAN", "BOOL":
			result["type"] = "boolean"
		case "STRING":
			result["type"] = "string"
		default:
			return nil, false
		}
	} else {
		var options []any
		if json.Unmarshal(spec[0], &options) != nil || len(options) == 0 || len(options) > 100 {
			return nil, false
		}
		result["type"] = primitiveJSONType(options[0])
		result["enum"] = options
	}
	if len(spec) > 1 {
		var config map[string]any
		if json.Unmarshal(spec[1], &config) == nil {
			for _, key := range []string{"minimum", "maximum"} {
				source := strings.TrimSuffix(key, "imum")
				if value, ok := config[source]; ok {
					result[key] = value
				}
			}
			if tooltip, ok := config["tooltip"].(string); ok && strings.TrimSpace(tooltip) != "" {
				result["description"] = strings.TrimSpace(tooltip)
			}
			if multiline, _ := config["multiline"].(bool); multiline {
				result["format"] = "textarea"
			}
		}
	}
	return result, true
}

func declaredOutputNode(raw json.RawMessage) bool {
	var definition struct {
		OutputNode bool `json:"output_node"`
	}
	return len(raw) > 0 && json.Unmarshal(raw, &definition) == nil && definition.OutputNode
}

func technicalInput(input string) bool {
	switch input {
	case "filename_prefix", "image", "audio", "video", "model", "clip", "vae", "latent", "latent_image", "samples", "pixels", "conditioning", "positive", "negative", "mask", "control_net":
		return true
	default:
		return false
	}
}

func isModelLiteral(input string, value any, classType string, discovery *Discovery) bool {
	if modelFolder(input, classType) != "" {
		return true
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return false
	}
	for _, models := range discovery.Models {
		if contains(models, text) {
			return true
		}
	}
	return false
}

func primitiveJSONType(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	case float32, float64:
		return "number"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return "number"
	default:
		return "string"
	}
}

func addSuggestedParameter(analysis *WorkflowAnalysis, name string, value any, schema map[string]any, items []parameterOccurrence) {
	name = uniqueParameterName(analysis.DefaultParameters, sanitizeIdentifier(name))
	properties(analysis.ParameterSchema)[name] = schema
	analysis.DefaultParameters[name] = value
	for index, item := range items {
		bindingName := uniqueBindingName(analysis.Bindings, name, index)
		analysis.Bindings[bindingName] = Binding{NodeID: item.nodeID, Input: item.input, Source: "parameter", Parameter: name}
	}
}

func suggestedOutput(id string, node WorkflowNode) (OutputSelector, bool) {
	className := strings.ToLower(node.ClassType)
	switch {
	case strings.Contains(className, "vhs_videocombine"):
		return OutputSelector{NodeID: id, Key: "gifs", MediaType: "video", MIMEType: "video/mp4"}, true
	case strings.Contains(className, "saveanimatedwebp"):
		return OutputSelector{NodeID: id, Key: "images", MediaType: "image", MIMEType: "image/webp"}, true
	case strings.Contains(className, "video") && (strings.Contains(className, "save") || strings.Contains(className, "combine")):
		return OutputSelector{NodeID: id, Key: "videos", MediaType: "video", MIMEType: "video/mp4"}, true
	case strings.Contains(className, "audio") && (strings.Contains(className, "save") || strings.Contains(className, "preview")):
		return OutputSelector{NodeID: id, Key: "audio", MediaType: "audio"}, true
	case strings.Contains(className, "image") && (strings.Contains(className, "save") || strings.Contains(className, "preview")):
		return OutputSelector{NodeID: id, Key: "images", MediaType: "image"}, true
	case strings.Contains(className, "text") && (strings.Contains(className, "save") || strings.Contains(className, "show") || strings.Contains(className, "preview")):
		return OutputSelector{NodeID: id, Key: "text", MediaType: "text", MIMEType: "text/plain; charset=utf-8"}, true
	default:
		return OutputSelector{}, false
	}
}

func inferModelRequirements(graph map[string]WorkflowNode, ids []string, discovery *Discovery, issues *[]AnalysisIssue) []ModelRequirement {
	seen := map[string]ModelRequirement{}
	for _, id := range ids {
		node := graph[id]
		for input, raw := range node.Inputs {
			value, ok := raw.(string)
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			folder := modelFolder(input, node.ClassType)
			if folder == "" && discovery != nil {
				matches := []string{}
				for candidate, models := range discovery.Models {
					if contains(models, value) {
						matches = append(matches, candidate)
					}
				}
				if len(matches) == 1 {
					folder = matches[0]
				}
			}
			if folder == "" {
				continue
			}
			requirement := ModelRequirement{Folder: folder, Name: value}
			seen[folder+"\x00"+value] = requirement
		}
	}
	result := make([]ModelRequirement, 0, len(seen))
	for _, requirement := range seen {
		result = append(result, requirement)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Folder == result[j].Folder {
			return result[i].Name < result[j].Name
		}
		return result[i].Folder < result[j].Folder
	})
	if discovery != nil {
		missing := []string{}
		for _, requirement := range result {
			if !contains(discovery.Models[requirement.Folder], requirement.Name) {
				missing = append(missing, requirement.Name)
			}
		}
		if len(missing) > 0 {
			*issues = append(*issues, AnalysisIssue{Level: "warning", Code: "missing_models", Message: "所选连接缺少模型：" + strings.Join(missing, "、")})
		}
	}
	return result
}

func modelFolder(input, classType string) string {
	switch input {
	case "ckpt_name":
		return "checkpoints"
	case "unet_name":
		return "diffusion_models"
	case "clip_name", "clip_name1", "clip_name2", "clip_name3":
		return "text_encoders"
	case "vae_name":
		return "vae"
	case "lora_name":
		return "loras"
	case "control_net_name":
		return "controlnet"
	case "upscale_model":
		return "upscale_models"
	case "clip_vision":
		return "clip_vision"
	case "style_model_name":
		return "style_models"
	case "gligen_name":
		return "gligen"
	}
	className := strings.ToLower(classType)
	if strings.Contains(className, "checkpointloader") && strings.Contains(input, "name") {
		return "checkpoints"
	}
	return ""
}

func outputCapability(outputs []OutputSelector) string {
	for _, mediaType := range []string{"video", "audio", "image", "text"} {
		for _, output := range outputs {
			if output.MediaType == mediaType {
				return mediaType
			}
		}
	}
	return "image"
}

func properties(schema map[string]any) map[string]any {
	value, _ := schema["properties"].(map[string]any)
	if value == nil {
		value = map[string]any{}
		schema["properties"] = value
	}
	return value
}

func connectionNodeID(value any) (string, bool) {
	items, ok := value.([]any)
	if !ok || len(items) < 2 {
		return "", false
	}
	switch id := items[0].(type) {
	case string:
		return id, id != ""
	case float64:
		return strconv.FormatFloat(id, 'f', -1, 64), true
	case json.Number:
		return id.String(), true
	default:
		return "", false
	}
}

func strictConnectionNodeID(value any) (string, bool) {
	items, ok := value.([]any)
	if !ok || len(items) < 2 {
		return "", false
	}
	id, ok := items[0].(string)
	return id, ok && id != ""
}

func isConnection(value any) bool {
	_, ok := connectionNodeID(value)
	return ok
}

func isPrimitive(value any) bool {
	switch value.(type) {
	case string, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return true
	default:
		return false
	}
}

func sortedNodeIDs(graph map[string]WorkflowNode) []string {
	ids := make([]string, 0, len(graph))
	for id := range graph {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, leftErr := strconv.Atoi(ids[i])
		right, rightErr := strconv.Atoi(ids[j])
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return ids[i] < ids[j]
	})
	return ids
}

func sortedSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		left, leftErr := strconv.Atoi(result[i])
		right, rightErr := strconv.Atoi(result[j])
		if leftErr == nil && rightErr == nil {
			return left < right
		}
		return result[i] < result[j]
	})
	return result
}

func equalOccurrenceDefaults(items []parameterOccurrence) bool {
	for index := 1; index < len(items); index++ {
		if !reflect.DeepEqual(items[0].value, items[index].value) {
			return false
		}
	}
	return true
}

func uniqueBindingName(bindings map[string]Binding, base string, index int) string {
	name := base
	if index > 0 {
		name = fmt.Sprintf("%s_%d", base, index+1)
	}
	for suffix := index + 2; ; suffix++ {
		if _, exists := bindings[name]; !exists {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, suffix)
	}
}

func uniqueParameterName(parameters map[string]any, base string) string {
	if base == "" {
		base = "parameter"
	}
	name := base
	for suffix := 2; ; suffix++ {
		if _, exists := parameters[name]; !exists {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, suffix)
	}
}

func sanitizeIdentifier(value string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore && builder.Len() > 0 {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func nodeTitle(node WorkflowNode, fallback string) string {
	if title, ok := node.Meta["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if fallback == "" {
		return node.ClassType
	}
	return fmt.Sprintf("%s #%s", node.ClassType, fallback)
}

func parameterTitle(name string) string {
	if schema, ok := map[string]string{
		"seed": "随机种子", "noise_seed": "噪声种子", "steps": "采样步数", "cfg": "CFG", "denoise": "重绘强度",
		"sampler_name": "采样器", "scheduler": "调度器", "width": "宽度", "height": "高度", "batch_size": "批次数量",
		"guidance": "引导强度", "strength": "控制强度", "strength_model": "LoRA 模型强度", "strength_clip": "LoRA CLIP 强度",
		"start_percent": "开始比例", "end_percent": "结束比例", "left": "左侧扩展", "right": "右侧扩展",
		"top": "顶部扩展", "bottom": "底部扩展", "feathering": "羽化",
	}[name]; ok {
		return schema
	}
	return name
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func analyzerStringValue(value any) string {
	text, _ := value.(string)
	return text
}

func outputNodeIDs(outputs []OutputSelector) []string {
	result := make([]string, 0, len(outputs))
	for _, output := range outputs {
		result = append(result, output.NodeID)
	}
	return result
}
