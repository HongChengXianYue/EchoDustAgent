package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type SearchFilesTool struct {
	Workdir string
}

func (t *SearchFilesTool) Name() string {
	return "search_files"
}

func (t *SearchFilesTool) Description() string {
	return "Search for a literal text query in files under a workdir-relative directory."
}

func (t *SearchFilesTool) Parameters() json.RawMessage {
	return schemaObject([]string{"query"}, map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": "Literal text to search for.",
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative directory path. Defaults to '.'.",
		},
	})
}

func (t *SearchFilesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if params.Query == "" {
		return Error("query is required"), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		params.Path = "."
	}
	root, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}

	const maxMatches = 100
	const maxFileBytes = 1024 * 1024
	var matches []string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(matches) >= maxMatches {
			return filepath.SkipAll
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxFileBytes {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if strings.Contains(line, params.Query) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", displayPath(t.Workdir, path), lineNo, strings.TrimSpace(line)))
				if len(matches) >= maxMatches {
					break
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return Error(walkErr.Error()), nil
	}
	summary := fmt.Sprintf("found %d match(es)", len(matches))
	if len(matches) == maxMatches {
		summary += " (truncated)"
	}
	return Success(summary, strings.Join(matches, "\n")), nil
}
