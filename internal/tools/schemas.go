package tools

import "encoding/json"

func schemaObject(required []string, properties map[string]any) json.RawMessage {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	// Some OpenAI-compatible providers reject "required": null. Match tiny-agent's
	// schema style by omitting required entirely when every property is optional.
	if len(required) > 0 {
		schema["required"] = required
	}
	data, _ := json.Marshal(schema)
	return data
}
