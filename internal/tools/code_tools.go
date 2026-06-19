package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type FindSymbolTool struct {
	Workdir string
}

func (t *FindSymbolTool) Name() string {
	return "find_symbol"
}

func (t *FindSymbolTool) Description() string {
	return "Search workspace code symbols by name using Go-aware indexing."
}

func (t *FindSymbolTool) Parameters() json.RawMessage {
	return schemaObject([]string{"query"}, map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": "Symbol name or partial symbol name to search for.",
		},
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum number of results. Defaults to 10.",
		},
	})
}

func (t *FindSymbolTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	query := strings.TrimSpace(params.Query)
	if query == "" {
		return Error("query is required"), nil
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	output, err := runCommandOutput(ctx, t.Workdir, nil, "codegraph", "query", query, "-p", t.Workdir, "-l", strconv.Itoa(limit), "-j")
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: output}, nil
	}
	formatted := formatCodegraphQueryOutput(output, limit)
	return Success(fmt.Sprintf("searched symbols for %q", query), formatted), nil
}

type FindReferencesTool struct {
	Workdir string
}

func (t *FindReferencesTool) Name() string {
	return "find_references"
}

func (t *FindReferencesTool) Description() string {
	return "Find references to a Go symbol at a specific file position."
}

func (t *FindReferencesTool) Parameters() json.RawMessage {
	return symbolPositionParameters("Find references to the identifier at the given file position.")
}

func (t *FindReferencesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	return runGoplsPositionTool(ctx, t.Workdir, args, "references", false)
}

type FindCallersTool struct {
	Workdir string
}

func (t *FindCallersTool) Name() string {
	return "find_callers"
}

func (t *FindCallersTool) Description() string {
	return "Find functions that call the Go symbol at a specific file position."
}

func (t *FindCallersTool) Parameters() json.RawMessage {
	return symbolPositionParameters("Find callers of the identifier at the given file position.")
}

func (t *FindCallersTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	return runGoplsPositionTool(ctx, t.Workdir, args, "call_hierarchy", true)
}

type FindCalleesTool struct {
	Workdir string
}

func (t *FindCalleesTool) Name() string {
	return "find_callees"
}

func (t *FindCalleesTool) Description() string {
	return "Find functions called by the Go symbol at a specific file position."
}

func (t *FindCalleesTool) Parameters() json.RawMessage {
	return symbolPositionParameters("Find callees of the identifier at the given file position.")
}

func (t *FindCalleesTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	return runGoplsPositionTool(ctx, t.Workdir, args, "call_hierarchy", false)
}

func symbolPositionParameters(positionDescription string) json.RawMessage {
	return schemaObject([]string{"path", "line", "column"}, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Workdir-relative Go source file path.",
		},
		"line": map[string]any{
			"type":        "integer",
			"description": "1-based line number of the target identifier.",
		},
		"column": map[string]any{
			"type":        "integer",
			"description": "1-based column number of the target identifier.",
		},
		"include_declaration": map[string]any{
			"type":        "boolean",
			"description": positionDescription + " For references, optionally include the declaration itself.",
		},
	})
}

func runGoplsPositionTool(ctx context.Context, workdir string, args json.RawMessage, subcommand string, callersOnly bool) (Result, error) {
	var params struct {
		Path               string `json:"path"`
		Line               int    `json:"line"`
		Column             int    `json:"column"`
		IncludeDeclaration bool   `json:"include_declaration"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if params.Line <= 0 || params.Column <= 0 {
		return Error("line and column must be positive"), nil
	}
	path, err := resolvePath(workdir, params.Path)
	if err != nil {
		return Error(err.Error()), nil
	}
	position := filepath.ToSlash(displayPath(workdir, path)) + ":" + strconv.Itoa(params.Line) + ":" + strconv.Itoa(params.Column)
	commandArgs := []string{subcommand}
	if subcommand == "references" && params.IncludeDeclaration {
		commandArgs = append(commandArgs, "-d")
	}
	commandArgs = append(commandArgs, position)
	output, err := runCommandOutput(ctx, workdir, goplsEnv(), "gopls", commandArgs...)
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: sanitizeGoplsOutput(output)}, nil
	}
	output = sanitizeGoplsOutput(output)
	if subcommand == "call_hierarchy" {
		output = filterCallHierarchyOutput(output, callersOnly)
	}
	if strings.TrimSpace(output) == "" {
		output = "(no results)"
	}
	return Success(fmt.Sprintf("%s completed for %s", strings.ReplaceAll(subcommand, "_", " "), position), output), nil
}

func goplsEnv() []string {
	home := "/tmp/local-agent-home"
	cacheHome := "/tmp/local-agent-cache"
	tmpDir := "/tmp/local-agent-tmp"
	goCache := "/tmp/local-agent-gocache"
	goPath := "/tmp/local-agent-gopath"
	for _, dir := range []string{home, cacheHome, tmpDir, goCache, goPath} {
		_ = os.MkdirAll(dir, 0755)
	}
	env := []string{
		"HOME=" + home,
		"XDG_CACHE_HOME=" + cacheHome,
		"TMPDIR=" + tmpDir,
		"GOCACHE=" + goCache,
		"GOPATH=" + goPath,
	}
	if goModCache := currentGoModCache(); goModCache != "" {
		// gopls needs module metadata for the workspace. Keep writable build
		// caches isolated in /tmp, but reuse the existing module cache so
		// read-only navigation does not depend on network downloads.
		env = append(env, "GOMODCACHE="+goModCache)
	}
	return env
}

func currentGoModCache() string {
	if raw := strings.TrimSpace(os.Getenv("GOMODCACHE")); raw != "" {
		return raw
	}
	if raw := strings.TrimSpace(os.Getenv("GOPATH")); raw != "" {
		parts := filepath.SplitList(raw)
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return filepath.Join(parts[0], "pkg", "mod")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "go", "pkg", "mod")
}

func sanitizeGoplsOutput(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "202") && strings.Contains(line, "creating temp dir:") {
			continue
		}
		if strings.HasPrefix(line, "202") && strings.Contains(line, "Error:") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func filterCallHierarchyOutput(text string, callersOnly bool) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "identifier:") {
			filtered = append(filtered, line)
			continue
		}
		if callersOnly {
			if strings.HasPrefix(line, "caller[") {
				filtered = append(filtered, line)
			}
			continue
		}
		if strings.HasPrefix(line, "callee[") {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "\n")
}

func formatCodegraphQueryOutput(raw string, limit int) string {
	type codegraphQueryResult struct {
		Node struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			FilePath  string `json:"filePath"`
			StartLine int    `json:"startLine"`
			Signature string `json:"signature"`
		} `json:"node"`
	}
	var results []codegraphQueryResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return raw
	}
	if len(results) == 0 {
		return "(no symbols found)"
	}
	if len(results) > limit {
		results = results[:limit]
	}
	lines := make([]string, 0, len(results))
	for _, result := range results {
		line := fmt.Sprintf("%s %s %s:%d", result.Node.Kind, result.Node.Name, result.Node.FilePath, result.Node.StartLine)
		if signature := strings.TrimSpace(result.Node.Signature); signature != "" {
			line += " " + signature
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
