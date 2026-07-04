package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ReplaceInFileTool struct {
	Workdir      string
	PreviewLines int
}

func (t *ReplaceInFileTool) Name() string {
	return "replace_in_file"
}

func (t *ReplaceInFileTool) Description() string {
	return "Replace all exact occurrences of old_text with new_text in a workdir-relative file."
}

func (t *ReplaceInFileTool) Parameters() json.RawMessage {
	return schemaObject([]string{"path", "old_text", "new_text"}, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative file path.",
		},
		"old_text": map[string]any{
			"type":        "string",
			"description": "Exact text to replace. Must not be empty.",
		},
		"new_text": map[string]any{
			"type":        "string",
			"description": "Replacement text.",
		},
	})
}

func (t *ReplaceInFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if params.OldText == "" {
		return Error("old_text must not be empty"), nil
	}
	path, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Error(err.Error()), nil
	}
	text := string(data)
	count := strings.Count(text, params.OldText)
	if count == 0 {
		return Error("old_text was not found"), nil
	}
	updated := strings.ReplaceAll(text, params.OldText, params.NewText)
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return Error(err.Error()), nil
	}
	previewLines := t.PreviewLines
	if previewLines <= 0 {
		previewLines = DefaultOptions().FileChangePreviewLines
	}
	result := Success(fmt.Sprintf("replaced %d occurrence(s) in %s", count, displayPath(t.Workdir, path)), "")
	if change, ok := fileChangeFromText(displayPath(t.Workdir, path), text, updated, "edited", previewLines); ok {
		result.Changes = []FileChange{change}
	}
	return result, nil
}
