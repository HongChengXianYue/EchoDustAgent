package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type ListFilesTool struct {
	Workdir    string
	MaxEntries int
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "List files and directories under a workdir-relative directory."
}

func (t *ListFilesTool) Parameters() json.RawMessage {
	return schemaObject(nil, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative directory path. Defaults to '.'.",
		},
	})
}

func (t *ListFilesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		params.Path = "."
	}
	dir, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return Error(err.Error()), nil
	}
	maxEntries := t.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultOptions().ListMaxEntries
	}
	var lines []string
	for i, entry := range entries {
		if i >= maxEntries {
			lines = append(lines, fmt.Sprintf("[truncated after %d entries]", maxEntries))
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	return Success(fmt.Sprintf("listed %d entries in %s", len(entries), displayPath(t.Workdir, dir)), strings.Join(lines, "\n")), nil
}
