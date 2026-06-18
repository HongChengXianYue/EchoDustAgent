package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/tools"
)

type recallTool struct {
	store Store
}

// NewRecallTool returns the read-only `memory` tool.
func NewRecallTool(store Store) tools.Tool {
	return recallTool{store: store}
}

func (recallTool) Name() string { return "memory" }

func (recallTool) Description() string {
	return "Search, list, and read saved durable memories. Use before remember to avoid duplicates, and when a memory index entry looks relevant."
}

func (recallTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"operation":{"type":"string","enum":["search","read","list"]},
			"query":{"type":"string","description":"Search query for operation=search."},
			"name":{"type":"string","description":"Memory slug for operation=read."},
			"type":{"type":"string","enum":["user","feedback","project","reference"],"description":"Optional type filter for search or list."},
			"limit":{"type":"integer","description":"Maximum search/list results, default 8, max 20."}
		},
		"required":["operation"],
		"additionalProperties":false
	}`)
}

func (t recallTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Operation string `json:"operation"`
		Query     string `json:"query"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Error("invalid memory arguments: " + err.Error()), nil
	}
	if t.store.Dir == "" && t.store.GlobalDir == "" {
		return tools.Success("memory store unavailable", "No memory store is configured."), nil
	}
	memoryType, err := typeFilter(input.Type)
	if err != nil {
		return tools.Error(err.Error()), nil
	}
	switch strings.TrimSpace(input.Operation) {
	case "list":
		return tools.Success("listed memories", formatMemoryList(filterByType(t.store.List(), memoryType), clampLimit(input.Limit))), nil
	case "read":
		memory, ok := t.store.Read(input.Name)
		if !ok {
			return tools.Error(fmt.Sprintf("memory %q not found", slug(input.Name))), nil
		}
		return tools.Success("read memory", formatMemory(memory)), nil
	case "search":
		if strings.TrimSpace(input.Query) == "" {
			return tools.Error("query is required for memory search"), nil
		}
		select {
		case <-ctx.Done():
			return tools.Error(ctx.Err().Error()), nil
		default:
		}
		return tools.Success("searched memories", formatMemoryList(t.store.Search(input.Query, memoryType, clampLimit(input.Limit)), clampLimit(input.Limit))), nil
	case "":
		return tools.Error("operation is required"), nil
	default:
		return tools.Error("unknown memory operation " + input.Operation), nil
	}
}

type rememberTool struct {
	store Store
}

// NewRememberTool returns the `remember` tool.
func NewRememberTool(store Store) tools.Tool {
	return rememberTool{store: store}
}

func (rememberTool) Name() string { return "remember" }

func (rememberTool) Description() string {
	return "Save or update a durable memory that should survive future sessions. Do not save facts that only matter to the current conversation or are already obvious from repo files."
}

func (rememberTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"Short kebab-case slug. Reuse an existing name to update that memory."},
			"title":{"type":"string","description":"Human-readable label for the memory index."},
			"description":{"type":"string","description":"One-line hook shown in the memory index."},
			"type":{"type":"string","enum":["user","feedback","project","reference"]},
			"body":{"type":"string","description":"The durable fact in Markdown."}
		},
		"required":["description","body"],
		"additionalProperties":false
	}`)
}

func (t rememberTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Error("invalid remember arguments: " + err.Error()), nil
	}
	if err := ctx.Err(); err != nil {
		return tools.Error(err.Error()), nil
	}
	path, err := t.store.Save(Memory{
		Name:        input.Name,
		Title:       input.Title,
		Description: input.Description,
		Type:        NormalizeType(input.Type),
		Body:        input.Body,
	})
	if err != nil {
		return tools.Error(err.Error()), nil
	}
	return tools.Success("saved memory", "Saved memory to "+path+". It applies in future sessions and can be read with the memory tool."), nil
}

type forgetTool struct {
	store Store
}

// NewForgetTool returns the `forget` tool.
func NewForgetTool(store Store) tools.Tool {
	return forgetTool{store: store}
}

func (forgetTool) Name() string { return "forget" }

func (forgetTool) Description() string {
	return "Archive a saved memory by slug when it is wrong, stale, or superseded, so it stops loading in future sessions."
}

func (forgetTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","description":"Memory slug to archive."}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
}

func (t forgetTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Error("invalid forget arguments: " + err.Error()), nil
	}
	if err := ctx.Err(); err != nil {
		return tools.Error(err.Error()), nil
	}
	archive, err := t.store.Archive(input.Name)
	if err != nil {
		return tools.Error(err.Error()), nil
	}
	if archive == "" {
		return tools.Success("memory already absent", "No active memory named "+slug(input.Name)+" was found."), nil
	}
	return tools.Success("forgot memory", "Archived memory to "+archive+". It will not load in future sessions."), nil
}

func typeFilter(value string) (Type, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	memoryType := Type(strings.ToLower(strings.TrimSpace(value)))
	if !validTypes[memoryType] {
		return "", fmt.Errorf("type must be one of user, feedback, project, reference")
	}
	return memoryType, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 8
	}
	if limit > 20 {
		return 20
	}
	return limit
}

func filterByType(memories []Memory, memoryType Type) []Memory {
	if memoryType == "" {
		return memories
	}
	out := make([]Memory, 0, len(memories))
	for _, memory := range memories {
		if memory.Type == memoryType {
			out = append(out, memory)
		}
	}
	return out
}

func formatMemoryList(memories []Memory, limit int) string {
	if len(memories) == 0 {
		return "No saved memories found."
	}
	if limit > 0 && len(memories) > limit {
		memories = memories[:limit]
	}
	lines := make([]string, 0, len(memories))
	for _, memory := range memories {
		lines = append(lines, indexLine(memory))
	}
	return strings.Join(lines, "\n")
}

func formatMemory(memory Memory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", firstNonEmpty(memory.Title, titleFromSlug(memory.Name)))
	fmt.Fprintf(&b, "- Name: `%s`\n", memory.Name)
	fmt.Fprintf(&b, "- Type: `%s`\n", NormalizeType(string(memory.Type)))
	if strings.TrimSpace(memory.Description) != "" {
		fmt.Fprintf(&b, "- Description: %s\n", oneLine(memory.Description))
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(memory.Body))
	return b.String()
}
