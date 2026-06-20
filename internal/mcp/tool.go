package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"local-agent/internal/tools"
)

var emptyObjectSchema = json.RawMessage(`{"type":"object","additionalProperties":true}`)

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type ToolCaller interface {
	CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error)
}

type CallResult struct {
	Content []ContentItem `json:"content,omitempty"`
	IsError bool          `json:"isError,omitempty"`
	Raw     json.RawMessage
}

type ContentItem struct {
	Type string          `json:"type,omitempty"`
	Text string          `json:"text,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type Tool struct {
	publicName  string
	remoteName  string
	serverName  string
	description string
	parameters  json.RawMessage
	caller      ToolCaller
}

func NewTool(serverName string, spec ToolSpec, caller ToolCaller) *Tool {
	remoteName := strings.TrimSpace(spec.Name)
	serverName = sanitizeSegment(serverName)
	publicName := "mcp__" + serverName + "__" + sanitizeSegment(remoteName)
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		description = fmt.Sprintf("Call MCP tool %s from server %s.", remoteName, serverName)
	} else {
		description = fmt.Sprintf("%s\n\nMCP server: %s. Remote tool: %s.", description, serverName, remoteName)
	}
	parameters := normalizeSchema(spec.InputSchema)
	return &Tool{
		publicName:  publicName,
		remoteName:  remoteName,
		serverName:  serverName,
		description: description,
		parameters:  parameters,
		caller:      caller,
	}
}

func (t *Tool) Name() string {
	return t.publicName
}

func (t *Tool) Description() string {
	return t.description
}

func (t *Tool) Parameters() json.RawMessage {
	return t.parameters
}

func (t *Tool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	result, err := t.caller.CallTool(ctx, t.remoteName, args)
	if err != nil {
		return tools.Result{}, err
	}
	output := strings.TrimSpace(result.Text())
	if output == "" && len(result.Raw) > 0 {
		output = string(result.Raw)
	}
	if result.IsError {
		if output == "" {
			output = "MCP tool returned an error"
		}
		return tools.Error(output), nil
	}
	return tools.Success(fmt.Sprintf("MCP tool %s/%s completed", t.serverName, t.remoteName), output), nil
}

func (r CallResult) Text() string {
	var parts []string
	for _, item := range r.Content {
		switch {
		case item.Type == "text" && strings.TrimSpace(item.Text) != "":
			parts = append(parts, item.Text)
		case len(item.Data) > 0:
			parts = append(parts, string(item.Data))
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeSchema(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 || strings.TrimSpace(string(schema)) == "" {
		return emptyObjectSchema
	}
	var value any
	if err := json.Unmarshal(schema, &value); err != nil {
		return emptyObjectSchema
	}
	object, ok := value.(map[string]any)
	if !ok {
		return emptyObjectSchema
	}
	if _, ok := object["type"]; !ok {
		object["type"] = "object"
	}
	data, err := json.Marshal(object)
	if err != nil {
		return emptyObjectSchema
	}
	return data
}

func sanitizeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	previousUnderscore := false
	for _, r := range value {
		valid := r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
		if !valid {
			if !previousUnderscore {
				b.WriteByte('_')
				previousUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		previousUnderscore = r == '_'
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if first := []rune(out)[0]; unicode.IsDigit(first) {
		out = "x_" + out
	}
	return out
}
