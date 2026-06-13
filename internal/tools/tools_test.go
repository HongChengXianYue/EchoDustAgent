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

	read := &ReadFileTool{Workdir: workdir}
	result, err = read.Execute(ctx, mustJSON(t, map[string]any{"path": "notes/todo.txt"}))
	if err != nil || result.Output != "alpha\nbeta\n" {
		t.Fatalf("read result = %#v err = %v", result, err)
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

	search := &SearchFilesTool{Workdir: workdir}
	result, err = search.Execute(ctx, mustJSON(t, map[string]any{"query": "gamma"}))
	if err != nil || !strings.Contains(result.Output, "notes/todo.txt:2:gamma") {
		t.Fatalf("search result = %#v err = %v", result, err)
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
	// The patch command expects exactly two leading plus signs in the new-file
	// header; build the string this way so the test fixture remains readable.
	patch = strings.Replace(patch, "++++ b/file.txt", "+++ b/file.txt", 1)

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

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
