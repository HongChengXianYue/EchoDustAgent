package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFileTool struct {
	Workdir      string
	PreviewLines int
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a workdir-relative file, creating parent directories when needed."
}

func (t *WriteFileTool) Parameters() json.RawMessage {
	return schemaObject([]string{"path", "content"}, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative file path.",
		},
		"content": map[string]any{
			"type":        "string",
			"description": "Full file content to write.",
		},
	})
}

func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	path, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	oldContent, readErr := os.ReadFile(path)
	action := "added"
	removedLines := 0
	if readErr == nil {
		action = "edited"
		removedLines = countLines(string(oldContent))
	} else if !os.IsNotExist(readErr) {
		return Error(readErr.Error()), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Error(err.Error()), nil
	}
	if err := os.WriteFile(path, []byte(params.Content), 0644); err != nil {
		return Error(err.Error()), nil
	}
	previewLines := t.PreviewLines
	if previewLines <= 0 {
		previewLines = DefaultOptions().FileChangePreviewLines
	}
	result := Success(fmt.Sprintf("wrote %s (%d bytes)", displayPath(t.Workdir, path), len(params.Content)), "")
	result.Changes = []FileChange{
		{
			Path:         displayPath(t.Workdir, path),
			Action:       action,
			AddedLines:   countLines(params.Content),
			RemovedLines: removedLines,
			Preview:      addedContentPreview(params.Content, previewLines),
		},
	}
	return result, nil
}
