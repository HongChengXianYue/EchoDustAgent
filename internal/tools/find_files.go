package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

type FindFilesTool struct {
	Workdir string
}

func (t *FindFilesTool) Name() string {
	return "find_files"
}

func (t *FindFilesTool) Description() string {
	return "Find files or directories by name or workdir-relative path under a directory. Use this to check whether a file or directory exists anywhere under the workspace; list_files only lists one directory level."
}

func (t *FindFilesTool) Parameters() json.RawMessage {
	return schemaObject([]string{"query"}, map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": "Literal text or regular expression to match against file names, directory names, or relative paths.",
		},
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative directory path to search under. Defaults to '.'.",
		},
		"regex": map[string]any{
			"type":        "boolean",
			"description": "Treat query as a regular expression. Defaults to false.",
		},
		"max_matches": map[string]any{
			"type":        "integer",
			"description": "Maximum number of matching paths to return. Defaults to 50.",
		},
	})
}

func (t *FindFilesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Query      string `json:"query"`
		Path       string `json:"path"`
		Regex      bool   `json:"regex"`
		MaxMatches int    `json:"max_matches"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return Error("query is required"), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		params.Path = "."
	}
	if params.MaxMatches <= 0 {
		params.MaxMatches = 50
	}

	root, err := resolvePath(t.Workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	matchesPath, err := pathMatcher(params.Query, params.Regex)
	if err != nil {
		return Error(err.Error()), nil
	}

	matches := make([]string, 0, params.MaxMatches)
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if len(matches) >= params.MaxMatches {
			return filepath.SkipAll
		}
		if entry.IsDir() && shouldSkipFindDir(entry.Name()) && path != root {
			return filepath.SkipDir
		}

		rel := displayPath(t.Workdir, path)
		if rel == "." {
			return nil
		}
		if !matchesPath(entry.Name()) && !matchesPath(rel) {
			return nil
		}

		kind := "[FILE]"
		if entry.IsDir() {
			kind = "[DIR] "
		}
		matches = append(matches, fmt.Sprintf("%s %s", kind, rel))
		if len(matches) >= params.MaxMatches {
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return Error(walkErr.Error()), nil
	}

	summary := fmt.Sprintf("found %d path(s)", len(matches))
	if len(matches) == params.MaxMatches {
		summary += " (truncated)"
	}
	return Success(summary, strings.Join(matches, "\n")), nil
}

type pathMatcherFunc func(value string) bool

func pathMatcher(query string, regex bool) (pathMatcherFunc, error) {
	if !regex {
		return func(value string) bool {
			return strings.Contains(value, query)
		}, nil
	}
	pattern, err := regexp.Compile(query)
	if err != nil {
		return nil, err
	}
	return pattern.MatchString, nil
}

func shouldSkipFindDir(name string) bool {
	switch name {
	case ".git", ".agents", ".codex", ".codegraph", ".cursor", "node_modules", "target", "dist", "bin":
		return true
	default:
		return false
	}
}
