package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func TestBlockRendererRendersExploreRunEditAndFinal(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "list_files",
		Args: json.RawMessage(`{"path":"."}`),
		Result: &tools.Result{
			Status:  "success",
			Summary: "listed",
			Output:  "go.mod\ncmd/",
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "find_files",
		Args: json.RawMessage(`{"query":"test"}`),
		Result: &tools.Result{
			Status:  "success",
			Summary: "found",
			Output:  "[DIR]  internal/test",
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: "run_command",
		Args: json.RawMessage(`{"command":"pwd"}`),
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "run_command",
		Args: json.RawMessage(`{"command":"pwd"}`),
		Result: &tools.Result{
			Status: "success",
			Output: "/tmp/work",
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "write_file",
		Result: &tools.Result{
			Status: "success",
			Changes: []tools.FileChange{
				{Path: "hello.txt", Action: "added", AddedLines: 2, Preview: "    1 +hello"},
			},
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "done",
	})

	text := out.String()
	for _, want := range []string{
		"• Explored",
		"List .",
		"Find test in .",
		"• Running pwd",
		"• Ran pwd",
		"• Added hello.txt (+2 -0)",
		"done",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererRendersMarkdownFinalText(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeFinal,
		Message: `## 最近一次代码改动总结

**提交信息**: ` + "`Add Codex-style CLI UI`" + `

| 文件 | 作用 |
|------|------|
| renderer.go | **块渲染器** |

### 1. 🆕 新增模块

这是一个 **Go CLI Agent**。`,
	})

	text := out.String()
	if strings.Contains(text, "###") || strings.Contains(text, "## ") {
		t.Fatalf("rendered headings should not show markdown heading prefixes:\n%s", text)
	}
	for _, want := range []string{
		"最近一次代码改动总结",
		"提交信息",
		"Add Codex-style CLI UI",
		"文件",
		"作用",
		"renderer.go",
		"块渲染器",
		"新增模块",
		"Go CLI Agent",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererRendersCodeBlocksWithoutHeavyChromaHighlighting(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "项目结构\n\n```text\ncmd/agent/\ninternal/ui/\n```\n",
	})

	text := out.String()
	for _, want := range []string{"项目结构", "cmd/agent/", "internal/ui/"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[38;5;203m") {
		t.Fatalf("code block should not use heavy red chroma highlighting:\n%q", text)
	}
}

func TestBlockRendererRendersTodoUpdates(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Read files", Status: runtimeevent.TodoCompleted},
			{Text: "Edit code", Status: runtimeevent.TodoInProgress},
			{Text: "Run tests", Status: runtimeevent.TodoPending},
		},
	})

	text := out.String()
	for _, want := range []string{
		"• Todo",
		"[✓] Read files",
		"[>] Edit code",
		"[ ] Run tests",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Tool update_todos") {
		t.Fatalf("todo update should not render as a generic tool:\n%s", text)
	}
}

func TestBlockRendererCollapsesAndExpandsRunTools(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Run pwd", Status: runtimeevent.TodoInProgress},
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "run_command",
		Args: json.RawMessage(`{"command":"pwd"}`),
		Result: &tools.Result{
			Status: "success",
			Output: "/tmp/work",
		},
	})

	collapsed := out.String()
	for _, want := range []string{"• Todo", "• Tools (collapsed, Ctrl+E to expand)", "latest: Ran pwd"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("collapsed output missing %q:\n%s", want, collapsed)
		}
	}
	if strings.Contains(collapsed, "/tmp/work") {
		t.Fatalf("collapsed tools should hide command output:\n%s", collapsed)
	}

	renderer.ToggleTools()
	expanded := out.String()
	for _, want := range []string{"• Tools (expanded, Ctrl+E to collapse)", "Ran pwd", "/tmp/work"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded output missing %q:\n%s", want, expanded)
		}
	}
}

func TestBlockRendererKeepsLatestTodoState(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Read files", Status: runtimeevent.TodoInProgress}},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Read files", Status: runtimeevent.TodoCompleted}},
	})

	if len(renderer.todos) != 1 || renderer.todos[0].Status != runtimeevent.TodoCompleted {
		t.Fatalf("renderer todos = %#v, want latest completed state", renderer.todos)
	}
}

func TestBlockRendererRedrawReturnsToLineStart(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "First", Status: runtimeevent.TodoInProgress}},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Second", Status: runtimeevent.TodoInProgress}},
	})

	if !strings.Contains(out.String(), "\r\x1b[") {
		t.Fatalf("redraw should return to line start before cursor-up; output=%q", out.String())
	}
}

func TestBlockRendererLiveFrameUsesCRLF(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Read files", Status: runtimeevent.TodoInProgress}},
	})

	text := out.String()
	if !strings.Contains(text, "\r\n") {
		t.Fatalf("live frame output should use CRLF in raw terminal mode; output=%q", text)
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Fatalf("live frame output should not leave bare LF bytes; output=%q", text)
	}
}

func TestBlockRendererLimitsExpandedLiveFrame(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.liveFrameMaxLines = 10
	renderer.liveFrameMaxWidth = 60

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Read files", Status: runtimeevent.TodoInProgress}},
	})
	renderer.ToggleTools()
	for i := 0; i < 10; i++ {
		renderer.HandleEvent(runtimeevent.Event{
			Type: runtimeevent.TypeToolResult,
			Tool: "read_file",
			Args: json.RawMessage(`{"path":"README.md"}`),
			Result: &tools.Result{
				Status: "success",
				Output: strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 10),
			},
		})
	}

	frame := latestFrame(out.String())
	normalized := strings.ReplaceAll(frame, "\r\n", "\n")
	if got := countLines(normalized); got > renderer.liveFrameMaxLines {
		t.Fatalf("live frame line count = %d, want <= %d:\n%q", got, renderer.liveFrameMaxLines, frame)
	}
	if strings.Count(normalized, "abcdefghijklmnopqrstuvwxyz0123456789") > 2 {
		t.Fatalf("expanded live frame should not repeat full long tool outputs:\n%s", normalized)
	}
	if !strings.Contains(normalized, "truncated") && !strings.Contains(normalized, "hidden") {
		t.Fatalf("bounded live frame should explain hidden or truncated content:\n%s", normalized)
	}
}

func latestFrame(output string) string {
	index := strings.LastIndex(output, "\x1b[J")
	if index < 0 {
		return output
	}
	return output[index+len("\x1b[J"):]
}

func TestBlockRendererRendersApprovalRequestDetails(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type:     runtimeevent.TypeApprovalRequest,
		Tool:     "run_command",
		Category: approval.CategoryNetworkDependency,
		Args:     json.RawMessage(`{"command":"curl https://example.com"}`),
		Reason:   "tool execution requested",
	})

	text := out.String()
	for _, want := range []string{
		"Approval requested",
		"run_command [network_dependency]",
		"Command: curl https://example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererSummarizesWriteApprovalArgs(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type:     runtimeevent.TypeApprovalRequest,
		Tool:     "write_file",
		Category: approval.CategoryWorkspaceWrite,
		Args:     json.RawMessage(`{"path":"test/古诗.txt","content":"第一行\n第二行"}`),
		Reason:   "tool execution requested",
	})

	text := out.String()
	for _, want := range []string{
		"write_file [workspace_write]",
		"Path: test/古诗.txt",
		"Content: 2 lines",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "第一行") || strings.Contains(text, "第二行") {
		t.Fatalf("approval output should summarize write content instead of printing it:\n%s", text)
	}
}
