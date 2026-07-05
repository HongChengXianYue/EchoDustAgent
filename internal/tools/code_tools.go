package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	output, err := findSymbolsInWorkspace(ctx, t.Workdir, query, limit)
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: output}, nil
	}
	return Success(fmt.Sprintf("searched symbols for %q", query), output), nil
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
	goplsCommand := resolveGoplsCommand()
	output, err := runCommandOutput(ctx, workdir, goplsEnv(), goplsCommand, commandArgs...)
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
	home := "/tmp/echo-dust-code-home"
	cacheHome := "/tmp/echo-dust-code-cache"
	tmpDir := "/tmp/echo-dust-code-tmp"
	goCache := "/tmp/echo-dust-code-gocache"
	goPath := "/tmp/echo-dust-code-gopath"
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

func resolveGoplsCommand() string {
	if raw := strings.TrimSpace(os.Getenv("ECHODUST_CODE_GOPLS")); raw != "" {
		return raw
	}
	exePath, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(exePath), executableName("gopls"))
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() {
			return sibling
		}
	}
	if path, err := exec.LookPath("gopls"); err == nil {
		return path
	}
	return "gopls"
}

func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
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

type symbolMatch struct {
	Kind      string
	Name      string
	FilePath  string
	StartLine int
	Signature string
	Rank      int
}

func findSymbolsInWorkspace(ctx context.Context, workdir string, query string, limit int) (string, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		return "", fmt.Errorf("query is required")
	}
	fset := token.NewFileSet()
	matches := make([]symbolMatch, 0, limit)
	walkErr := filepath.WalkDir(workdir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if path != workdir && shouldSkipFindDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if parseErr != nil && file == nil {
			return nil
		}
		matches = append(matches, collectFileSymbolMatches(fset, workdir, file, normalizedQuery)...)
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(matches) == 0 {
		return "(no symbols found)", nil
	}
	sort.Slice(matches, func(i, j int) bool {
		left := matches[i]
		right := matches[j]
		switch {
		case left.Rank != right.Rank:
			return left.Rank < right.Rank
		case len(left.Name) != len(right.Name):
			return len(left.Name) < len(right.Name)
		case left.FilePath != right.FilePath:
			return left.FilePath < right.FilePath
		case left.StartLine != right.StartLine:
			return left.StartLine < right.StartLine
		default:
			return left.Signature < right.Signature
		}
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		line := fmt.Sprintf("%s %s %s:%d", match.Kind, match.Name, match.FilePath, match.StartLine)
		if signature := strings.TrimSpace(match.Signature); signature != "" {
			line += " " + signature
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func collectFileSymbolMatches(fset *token.FileSet, workdir string, file *ast.File, query string) []symbolMatch {
	results := []symbolMatch{}
	appendMatch := func(kind string, name *ast.Ident, signature string) {
		if name == nil {
			return
		}
		rank, ok := symbolMatchRank(name.Name, query)
		if !ok {
			return
		}
		results = append(results, symbolMatch{
			Kind:      kind,
			Name:      name.Name,
			FilePath:  filepath.ToSlash(displayPath(workdir, fset.Position(name.Pos()).Filename)),
			StartLine: fset.Position(name.Pos()).Line,
			Signature: signature,
			Rank:      rank,
		})
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			kind := "function"
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				kind = "method"
			}
			appendMatch(kind, decl.Name, formatFuncSignature(decl))
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					appendMatch("type", spec.Name, formatTypeSignature(spec))
				case *ast.ValueSpec:
					kind := strings.ToLower(decl.Tok.String())
					for _, name := range spec.Names {
						appendMatch(kind, name, formatValueSignature(kind, spec))
					}
				}
			}
		}
	}
	return results
}

func symbolMatchRank(name string, query string) (int, bool) {
	nameLower := strings.ToLower(name)
	switch {
	case nameLower == query:
		return 0, true
	case strings.HasPrefix(nameLower, query):
		return 1, true
	case strings.Contains(nameLower, query):
		return 2, true
	default:
		return 0, false
	}
}

func formatFuncSignature(decl *ast.FuncDecl) string {
	if decl == nil || decl.Name == nil {
		return ""
	}
	var b strings.Builder
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(formatFieldList(decl.Recv.List, ", "))
		b.WriteString(") ")
	}
	b.WriteString(decl.Name.Name)
	b.WriteString("(")
	b.WriteString(formatFieldList(decl.Type.Params.List, ", "))
	b.WriteString(")")
	if decl.Type.Results != nil && len(decl.Type.Results.List) > 0 {
		results := formatFieldList(decl.Type.Results.List, ", ")
		if len(decl.Type.Results.List) == 1 && len(decl.Type.Results.List[0].Names) == 0 {
			b.WriteString(" ")
			b.WriteString(results)
		} else {
			b.WriteString(" (")
			b.WriteString(results)
			b.WriteString(")")
		}
	}
	return b.String()
}

func formatTypeSignature(spec *ast.TypeSpec) string {
	if spec == nil || spec.Name == nil {
		return ""
	}
	if spec.Type == nil {
		return spec.Name.Name
	}
	return spec.Name.Name + " " + formatNode(spec.Type)
}

func formatValueSignature(kind string, spec *ast.ValueSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Type != nil {
		return kind + " " + formatNode(spec.Type)
	}
	if len(spec.Values) > 0 {
		return kind + " = " + formatNode(spec.Values[0])
	}
	return kind
}

func formatFieldList(fields []*ast.Field, separator string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == nil {
			continue
		}
		typeText := formatNode(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typeText)
			continue
		}
		names := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		parts = append(parts, strings.Join(names, ", ")+" "+typeText)
	}
	return strings.Join(parts, separator)
}

func formatNode(node any) string {
	if node == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), node); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}
