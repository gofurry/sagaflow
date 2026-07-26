package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

var featureNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func validFeatureName(value string) bool {
	return featureNamePattern.MatchString(strings.TrimSpace(value))
}

func catalogManagedModel(raw json.RawMessage) bool {
	var metadata map[string]any
	return json.Unmarshal(raw, &metadata) == nil && metadata["source"] == "builtin"
}

func userModelMetadata(raw json.RawMessage) json.RawMessage {
	metadata := map[string]any{}
	_ = json.Unmarshal(raw, &metadata)
	metadata["source"] = "user"
	if _, ok := metadata["support_status"]; !ok {
		metadata["support_status"] = "compatible"
	}
	if _, ok := metadata["lifecycle_status"]; !ok {
		metadata["lifecycle_status"] = "active"
	}
	encoded, _ := json.Marshal(metadata)
	return encoded
}

func validateParameterSchema(schemaRaw, defaultsRaw json.RawMessage) error {
	schema, err := decodeJSONObject(schemaRaw)
	if err != nil {
		return fmt.Errorf("parameter_schema %w", err)
	}
	for key := range schema {
		if key != "type" && key != "properties" {
			return fmt.Errorf("parameter_schema contains unsupported top-level field %q", key)
		}
	}
	if typeName, ok := schema["type"]; ok && typeName != "object" {
		return fmt.Errorf("parameter_schema type must be object")
	}
	properties := map[string]any{}
	if rawProperties, ok := schema["properties"]; ok {
		var valid bool
		properties, valid = rawProperties.(map[string]any)
		if !valid {
			return fmt.Errorf("parameter_schema properties must be an object")
		}
	}
	for name, rawProperty := range properties {
		property, ok := rawProperty.(map[string]any)
		if !ok {
			return fmt.Errorf("parameter %q must be an object", name)
		}
		if err := validateParameterProperty(name, property); err != nil {
			return err
		}
	}

	defaults, err := decodeJSONObject(defaultsRaw)
	if err != nil {
		return fmt.Errorf("default_parameters %w", err)
	}
	for name, value := range defaults {
		rawProperty, ok := properties[name]
		if !ok {
			return fmt.Errorf("default parameter %q is not declared in parameter_schema", name)
		}
		if err := validateParameterValue(name, value, rawProperty.(map[string]any)); err != nil {
			return err
		}
	}
	return nil
}

func decodeJSONObject(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("must be valid JSON: %v", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a JSON object")
	}
	return object, nil
}

func validateParameterProperty(name string, property map[string]any) error {
	allowed := map[string]bool{
		"type": true, "enum": true, "minimum": true, "maximum": true, "multipleOf": true,
		"title": true, "description": true, "format": true, "readOnly": true, "default": true, "items": true,
	}
	for key := range property {
		if !allowed[key] {
			return fmt.Errorf("parameter %q contains unsupported field %q", name, key)
		}
	}
	typeName, _ := property["type"].(string)
	switch typeName {
	case "string", "number", "integer", "boolean", "object", "array":
	default:
		return fmt.Errorf("parameter %q has unsupported type %q", name, typeName)
	}
	for _, key := range []string{"title", "description"} {
		if value, ok := property[key]; ok {
			if _, valid := value.(string); !valid {
				return fmt.Errorf("parameter %q field %s must be a string", name, key)
			}
		}
	}
	if format, ok := property["format"]; ok && format != "textarea" {
		return fmt.Errorf("parameter %q only supports format textarea", name)
	}
	if readOnly, ok := property["readOnly"]; ok {
		if _, valid := readOnly.(bool); !valid {
			return fmt.Errorf("parameter %q readOnly must be a boolean", name)
		}
	}
	for _, key := range []string{"minimum", "maximum", "multipleOf"} {
		if value, ok := property[key]; ok {
			if _, valid := numberValue(value); !valid {
				return fmt.Errorf("parameter %q field %s must be a number", name, key)
			}
		}
	}
	if minimum, hasMinimum := numberValue(property["minimum"]); hasMinimum {
		if maximum, hasMaximum := numberValue(property["maximum"]); hasMaximum && minimum > maximum {
			return fmt.Errorf("parameter %q minimum cannot exceed maximum", name)
		}
	}
	if multiple, ok := numberValue(property["multipleOf"]); ok && multiple <= 0 {
		return fmt.Errorf("parameter %q multipleOf must be greater than zero", name)
	}
	if typeName != "number" && typeName != "integer" {
		for _, key := range []string{"minimum", "maximum", "multipleOf"} {
			if _, ok := property[key]; ok {
				return fmt.Errorf("parameter %q field %s requires a numeric type", name, key)
			}
		}
	}
	if enum, ok := property["enum"]; ok {
		values, valid := enum.([]any)
		if !valid || len(values) == 0 {
			return fmt.Errorf("parameter %q enum must be a non-empty array", name)
		}
		for _, value := range values {
			if err := validateValueType(name, value, typeName); err != nil {
				return fmt.Errorf("enum %w", err)
			}
		}
	}
	if typeName == "array" {
		if items, ok := property["items"]; ok {
			itemSchema, valid := items.(map[string]any)
			if !valid || len(itemSchema) != 1 {
				return fmt.Errorf("parameter %q items only supports a type declaration", name)
			}
			itemType, _ := itemSchema["type"].(string)
			switch itemType {
			case "string", "number", "integer", "boolean", "object":
			default:
				return fmt.Errorf("parameter %q has unsupported array item type %q", name, itemType)
			}
		}
	} else if _, ok := property["items"]; ok {
		return fmt.Errorf("parameter %q items requires type array", name)
	}
	if value, ok := property["default"]; ok {
		if err := validateParameterValue(name, value, property); err != nil {
			return err
		}
	}
	return nil
}

func validateParameterValue(name string, value any, property map[string]any) error {
	typeName, _ := property["type"].(string)
	if err := validateValueType(name, value, typeName); err != nil {
		return err
	}
	if number, ok := numberValue(value); ok {
		if minimum, exists := numberValue(property["minimum"]); exists && number < minimum {
			return fmt.Errorf("parameter %q must be at least %v", name, minimum)
		}
		if maximum, exists := numberValue(property["maximum"]); exists && number > maximum {
			return fmt.Errorf("parameter %q must be at most %v", name, maximum)
		}
	}
	if rawEnum, ok := property["enum"]; ok {
		matched := false
		for _, candidate := range rawEnum.([]any) {
			if fmt.Sprint(candidate) == fmt.Sprint(value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("parameter %q is not one of the declared enum values", name)
		}
	}
	return nil
}

func validateValueType(name string, value any, typeName string) error {
	valid := false
	switch typeName {
	case "string":
		_, valid = value.(string)
	case "number":
		_, valid = numberValue(value)
	case "integer":
		if number, ok := numberValue(value); ok {
			valid = math.Trunc(number) == number
		}
	case "boolean":
		_, valid = value.(bool)
	case "object":
		_, valid = value.(map[string]any)
	case "array":
		_, valid = value.([]any)
	}
	if !valid {
		return fmt.Errorf("parameter %q must be %s", name, typeName)
	}
	return nil
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}
