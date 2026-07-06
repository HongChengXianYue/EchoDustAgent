package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileToolsRoundTrip(t *testing.T) {
	workdir := t.TempDir()
	ctx := context.Background()

	write := &WriteFileTool{Workdir: workdir}
	result, err := write.Execute(ctx, mustJSON(t, map[string]any{
		"path":    "notes/todo.txt",
		"content": "alpha\nbeta\n",
	}))
	if err != nil || result.Status != "success" {
		t.Fatalf("write result = %#v err = %v", result, err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Action != "added" || result.Changes[0].AddedLines != 2 {
		t.Fatalf("write changes = %#v, want added file metadata", result.Changes)
	}
	if !strings.Contains(result.Changes[0].Diff, "--- /dev/null") ||
		!strings.Contains(result.Changes[0].Diff, "+++ b/notes/todo.txt") ||
		!strings.Contains(result.Changes[0].Diff, "+alpha") ||
		!strings.Contains(result.Changes[0].Diff, "+beta") {
		t.Fatalf("write diff = %q, want unified diff for added file", result.Changes[0].Diff)
	}

	read := &ReadFileTool{Workdir: workdir}
	result, err = read.Execute(ctx, mustJSON(t, map[string]any{"path": "notes/todo.txt"}))
	if err != nil || result.Output != "alpha\nbeta\n" {
		t.Fatalf("read result = %#v err = %v", result, err)
	}

	readRange := &ReadFileRangeTool{Workdir: workdir}
	result, err = readRange.Execute(ctx, mustJSON(t, map[string]any{
		"path":       "notes/todo.txt",
		"start_line": 2,
		"end_line":   2,
	}))
	if err != nil || result.Output != "beta\n" {
		t.Fatalf("read range result = %#v err = %v", result, err)
	}

	replace := &ReplaceInFileTool{Workdir: workdir}
	result, err = replace.Execute(ctx, mustJSON(t, map[string]any{
		"path":     "notes/todo.txt",
		"old_text": "beta",
		"new_text": "gamma",
	}))
	if err != nil || result.Status != "success" {
		t.Fatalf("replace result = %#v err = %v", result, err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Action != "edited" || result.Changes[0].AddedLines != 1 || result.Changes[0].RemovedLines != 1 {
		t.Fatalf("replace changes = %#v, want edited file metadata", result.Changes)
	}
	if !strings.Contains(result.Changes[0].Diff, "--- a/notes/todo.txt") ||
		!strings.Contains(result.Changes[0].Diff, "+++ b/notes/todo.txt") ||
		!strings.Contains(result.Changes[0].Diff, "-beta") ||
		!strings.Contains(result.Changes[0].Diff, "+gamma") {
		t.Fatalf("replace diff = %q, want unified diff for replacement", result.Changes[0].Diff)
	}

	search := &SearchFilesTool{Workdir: workdir}
	result, err = search.Execute(ctx, mustJSON(t, map[string]any{"query": "gamma"}))
	if err != nil || !strings.Contains(result.Output, "notes/todo.txt:2:gamma") {
		t.Fatalf("search result = %#v err = %v", result, err)
	}

	result, err = search.Execute(ctx, mustJSON(t, map[string]any{"query": "^gamm.", "regex": true}))
	if err != nil || !strings.Contains(result.Output, "notes/todo.txt:2:gamma") {
		t.Fatalf("regex search result = %#v err = %v", result, err)
	}

	if err := os.MkdirAll(filepath.Join(workdir, "internal", "test"), 0755); err != nil {
		t.Fatal(err)
	}
	find := &FindFilesTool{Workdir: workdir}
	result, err = find.Execute(ctx, mustJSON(t, map[string]any{"query": "test"}))
	if err != nil || !strings.Contains(result.Output, "[DIR]  internal/test") {
		t.Fatalf("find result = %#v err = %v", result, err)
	}

	list := &ListFilesTool{Workdir: workdir}
	result, err = list.Execute(ctx, mustJSON(t, map[string]any{"path": "."}))
	if err != nil || !strings.Contains(result.Output, "notes/") {
		t.Fatalf("list result = %#v err = %v", result, err)
	}
}

func TestPathEscapeRejected(t *testing.T) {
	read := &ReadFileTool{Workdir: t.TempDir()}
	result, err := read.Execute(context.Background(), mustJSON(t, map[string]any{"path": "../outside.txt"}))
	if err != nil {
		t.Fatalf("read returned Go error = %v", err)
	}
	if result.Status != "error" || !strings.Contains(result.Summary, "escapes workdir") {
		t.Fatalf("result = %#v, want path escape error", result)
	}
}

func TestRunCommandTool(t *testing.T) {
	tool := &RunCommandTool{Workdir: t.TempDir()}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"command": "printf hello",
	}))
	if err != nil || result.Status != "success" || result.Output != "hello" {
		t.Fatalf("run command result = %#v err = %v", result, err)
	}
}

func TestApplyPatchTool(t *testing.T) {
	if _, err := exec.LookPath("patch"); err != nil {
		t.Skip("patch command not available")
	}
	workdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workdir, "file.txt"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	patch := `--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-old
+new
`

	tool := &ApplyPatchTool{Workdir: workdir}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{"patch": patch}))
	if err != nil || result.Status != "success" {
		t.Fatalf("apply patch result = %#v err = %v", result, err)
	}
	data, err := os.ReadFile(filepath.Join(workdir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("file content = %q, want new", string(data))
	}
	if len(result.Changes) != 1 || result.Changes[0].Path != "file.txt" || result.Changes[0].AddedLines != 1 || result.Changes[0].RemovedLines != 1 {
		t.Fatalf("patch changes = %#v, want file.txt (+1 -1)", result.Changes)
	}
	if !strings.Contains(result.Changes[0].Diff, "--- a/file.txt") ||
		!strings.Contains(result.Changes[0].Diff, "+++ b/file.txt") ||
		!strings.Contains(result.Changes[0].Diff, "-old") ||
		!strings.Contains(result.Changes[0].Diff, "+new") {
		t.Fatalf("patch diff = %q, want unified diff content", result.Changes[0].Diff)
	}
}

func TestSchemaObjectOmitsRequiredWhenNoFieldsAreRequired(t *testing.T) {
	schema := schemaObject(nil, map[string]any{
		"path": map[string]any{"type": "string"},
	})
	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if _, ok := decoded["required"]; ok {
		t.Fatalf("required = %#v, want omitted for optional-only schema", decoded["required"])
	}

	schema = schemaObject([]string{"path"}, map[string]any{
		"path": map[string]any{"type": "string"},
	})
	decoded = map[string]any{}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("decode required schema: %v", err)
	}
	required, ok := decoded["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("required = %#v, want [path]", decoded["required"])
	}
}

func TestRegisterBuiltinsIncludesFindFiles(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry, t.TempDir())

	if _, ok := registry.Get("find_files"); !ok {
		t.Fatal("find_files was not registered")
	}
	if _, ok := registry.Get("read_file_range"); !ok {
		t.Fatal("read_file_range was not registered")
	}
	if _, ok := registry.Get("find_symbol"); !ok {
		t.Fatal("find_symbol was not registered")
	}
	if _, ok := registry.Get("git_status"); !ok {
		t.Fatal("git_status was not registered")
	}
}

func TestRegisterBuiltinsAppliesFileChangePreviewLines(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltinsWithOptions(registry, t.TempDir(), Options{FileChangePreviewLines: 1})

	tool, ok := registry.Get("write_file")
	if !ok {
		t.Fatal("write_file was not registered")
	}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"path":    "many.txt",
		"content": "one\ntwo\nthree\n",
	}))
	if err != nil || result.Status != "success" {
		t.Fatalf("write result = %#v err = %v", result, err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("changes = %#v, want one change", result.Changes)
	}
	if !strings.Contains(result.Changes[0].Preview, "--- /dev/null") || !strings.Contains(result.Changes[0].Preview, "+++ b/many.txt") {
		t.Fatalf("preview = %q, want diff headers", result.Changes[0].Preview)
	}
	if strings.Contains(result.Changes[0].Preview, "two") {
		t.Fatalf("preview = %q, want one configured content line", result.Changes[0].Preview)
	}
	if !strings.Contains(result.Changes[0].Preview, "…") {
		t.Fatalf("preview = %q, want truncation marker", result.Changes[0].Preview)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGitTools(t *testing.T) {
	workdir := t.TempDir()
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workdir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(workdir, "a.txt"), []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(workdir, "a.txt"), []byte("beta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	status := &GitStatusTool{Workdir: workdir}
	result, err := status.Execute(ctx, mustJSON(t, map[string]any{}))
	if err != nil || !strings.Contains(result.Output, "a.txt") {
		t.Fatalf("git status result = %#v err = %v", result, err)
	}

	diff := &GitDiffTool{Workdir: workdir, OutputMaxBytes: 8192, PreviewLines: 20}
	result, err = diff.Execute(ctx, mustJSON(t, map[string]any{"path": "a.txt"}))
	if err != nil || len(result.Changes) != 1 || result.Output != "" {
		t.Fatalf("git diff result = %#v err = %v", result, err)
	}
	if result.Changes[0].Path != "a.txt" || result.Changes[0].AddedLines != 1 || result.Changes[0].RemovedLines != 1 {
		t.Fatalf("git diff changes = %#v, want a.txt (+1 -1)", result.Changes)
	}
	if !strings.Contains(result.Changes[0].Diff, "--- a/a.txt") ||
		!strings.Contains(result.Changes[0].Diff, "+++ b/a.txt") ||
		!strings.Contains(result.Changes[0].Diff, "-alpha") ||
		!strings.Contains(result.Changes[0].Diff, "+beta") {
		t.Fatalf("git diff change = %#v, want unified diff content", result.Changes[0])
	}

	logTool := &GitLogTool{Workdir: workdir}
	result, err = logTool.Execute(ctx, mustJSON(t, map[string]any{"limit": 1}))
	if err != nil || !strings.Contains(result.Output, "init") {
		t.Fatalf("git log result = %#v err = %v", result, err)
	}
}

func TestFindSymbolTool(t *testing.T) {
	workdir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tool := &FindSymbolTool{Workdir: workdir}
	result, execErr := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"query": "chatWithTools",
		"limit": 5,
	}))
	if execErr != nil {
		t.Fatalf("find symbol returned Go error = %v", execErr)
	}
	if result.Status != "success" {
		t.Fatalf("find symbol result = %#v", result)
	}
	if !strings.Contains(result.Output, "internal/agent/agent.go") {
		t.Fatalf("find symbol output = %q, want agent.go hit", result.Output)
	}
}

func TestGoCodeNavigationTools(t *testing.T) {
	requireCommand(t, "gopls")
	workdir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	line, column := sourcePosition(t, filepath.Join(workdir, "internal/agent/agent.go"), "chatWithTools(ctx context.Context")

	references := &FindReferencesTool{Workdir: workdir}
	result, execErr := references.Execute(ctx, mustJSON(t, map[string]any{
		"path":                "internal/agent/agent.go",
		"line":                line,
		"column":              column,
		"include_declaration": true,
	}))
	if execErr != nil || result.Status != "success" || !strings.Contains(result.Output, "internal/agent/agent.go") {
		t.Fatalf("find references result = %#v err = %v", result, execErr)
	}

	callers := &FindCallersTool{Workdir: workdir}
	result, execErr = callers.Execute(ctx, mustJSON(t, map[string]any{
		"path":   "internal/agent/agent.go",
		"line":   line,
		"column": column,
	}))
	if execErr != nil || result.Status != "success" || !strings.Contains(result.Output, "caller[") ||
		(!strings.Contains(result.Output, "function Run") && !strings.Contains(result.Output, "function executeStep")) {
		t.Fatalf("find callers result = %#v err = %v", result, execErr)
	}

	callees := &FindCalleesTool{Workdir: workdir}
	result, execErr = callees.Execute(ctx, mustJSON(t, map[string]any{
		"path":   "internal/agent/agent.go",
		"line":   line,
		"column": column,
	}))
	if execErr != nil || result.Status != "success" || !strings.Contains(result.Output, "callee[") ||
		!strings.Contains(result.Output, "function conversationMessages") ||
		!strings.Contains(result.Output, "function functionTools") ||
		!strings.Contains(result.Output, "function chat") {
		t.Fatalf("find callees result = %#v err = %v", result, execErr)
	}
}

func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s command not available", name)
	}
}

func sourcePosition(t *testing.T, path string, needle string) (int, int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if column := strings.Index(line, needle); column >= 0 {
			return i + 1, column + 1
		}
	}
	t.Fatalf("source %s does not contain %q", path, needle)
	return 0, 0
}
