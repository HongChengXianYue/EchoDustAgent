package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type ReadFileTool struct {
	Workdir  string
	MaxBytes int
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read a UTF-8 text file using a workdir-relative path."
}

func (t *ReadFileTool) Parameters() json.RawMessage {
	return schemaObject([]string{"path"}, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative file path.",
		},
	})
}

func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	path, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Error(err.Error()), nil
	}
	maxBytes := t.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultOptions().ReadFileMaxBytes
	}
	output := capOutput(string(data), maxBytes)
	return Success(fmt.Sprintf("read %s (%d bytes)", displayPath(t.Workdir, path), len(data)), output), nil
}
