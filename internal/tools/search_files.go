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
	"regexp"
	"strings"
)

type SearchFilesTool struct {
	Workdir      string
	MaxMatches   int
	MaxFileBytes int64
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
		"regex": map[string]any{
			"type":        "boolean",
			"description": "Treat query as a regular expression when true. Defaults to false.",
		},
	})
}

func (t *SearchFilesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Query string `json:"query"`
		Path  string `json:"path"`
		Regex bool   `json:"regex"`
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

	maxMatches := t.MaxMatches
	if maxMatches <= 0 {
		maxMatches = DefaultOptions().SearchMaxMatches
	}
	maxFileBytes := t.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = int64(DefaultOptions().SearchMaxFileBytes)
	}
	matchesLine, err := contentMatcher(params.Query, params.Regex)
	if err != nil {
		return Error(err.Error()), nil
	}
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
			if matchesLine(line) {
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

type contentMatcherFunc func(value string) bool

func contentMatcher(query string, regex bool) (contentMatcherFunc, error) {
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
