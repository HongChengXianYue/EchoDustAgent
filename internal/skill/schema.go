package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

type schemaNode struct {
	Type                 string                `json:"type,omitempty"`
	Required             []string              `json:"required,omitempty"`
	Properties           map[string]schemaNode `json:"properties,omitempty"`
	Items                *schemaNode           `json:"items,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
	Enum                 []any                 `json:"enum,omitempty"`
}

func ValidateInput(schemaRaw json.RawMessage, inputRaw json.RawMessage) error {
	var schema schemaNode
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return fmt.Errorf("decode input schema: %w", err)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type != "object" {
		return fmt.Errorf("skill input_schema root must be an object")
	}

	var value any = map[string]any{}
	if len(bytes.TrimSpace(inputRaw)) > 0 {
		if err := json.Unmarshal(inputRaw, &value); err != nil {
			return fmt.Errorf("decode skill input: %w", err)
		}
	}
	return validateSchemaValue("input", schema, value)
}

func validateSchemaDefinition(schemaRaw json.RawMessage) error {
	var schema schemaNode
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type != "object" {
		return fmt.Errorf("root type must be object")
	}
	return nil
}

func schemaSummary(schemaRaw json.RawMessage) string {
	var schema schemaNode
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return "object"
	}
	if schema.Type == "" {
		schema.Type = "object"
	}
	if schema.Type != "object" || len(schema.Properties) == 0 {
		return schema.Type
	}
	required := map[string]struct{}{}
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		label := name
		if _, ok := required[name]; ok {
			label += "*"
		}
		propType := schema.Properties[name].Type
		if propType == "" {
			propType = "any"
		}
		parts = append(parts, label+": "+propType)
	}
	return "object {" + strings.Join(parts, ", ") + "}"
}

func validateSchemaValue(path string, schema schemaNode, value any) error {
	if len(schema.Enum) > 0 && !valueInEnum(value, schema.Enum) {
		return fmt.Errorf("%s must be one of %v", path, schema.Enum)
	}
	switch schema.Type {
	case "", "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		return validateObject(path, schema, object)
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if schema.Items != nil {
			for i, item := range items {
				if err := validateSchemaValue(fmt.Sprintf("%s[%d]", path, i), *schema.Items, item); err != nil {
					return err
				}
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
		return nil
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		return nil
	case "integer":
		number, ok := value.(float64)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("%s must be an integer", path)
		}
		return nil
	default:
		return nil
	}
}

func validateObject(path string, schema schemaNode, object map[string]any) error {
	required := map[string]struct{}{}
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}
	for name := range required {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("%s.%s is required", path, name)
		}
	}
	allowAdditional := true
	if raw, ok := schema.AdditionalProperties.(bool); ok {
		allowAdditional = raw
	}
	for name, value := range object {
		property, ok := schema.Properties[name]
		if !ok {
			if allowAdditional {
				continue
			}
			return fmt.Errorf("%s.%s is not allowed", path, name)
		}
		if property.Type == "" {
			continue
		}
		if err := validateSchemaValue(path+"."+name, property, value); err != nil {
			return err
		}
	}
	return nil
}

func valueInEnum(value any, enum []any) bool {
	for _, item := range enum {
		if reflectValueEqual(value, item) {
			return true
		}
	}
	return false
}

func reflectValueEqual(left any, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
