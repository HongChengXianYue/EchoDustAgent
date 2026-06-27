package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

func TestBlockRendererRendersDelegateTaskAsSubagent(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: tools.DelegateTaskToolName,
		Args: json.RawMessage(`{"task":"Inspect README"}`),
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: tools.DelegateTaskToolName,
		Args: json.RawMessage(`{"task":"Inspect README"}`),
		Result: &tools.Result{
			Status:  "success",
			Summary: "subagent completed",
			Output:  "README describes local-agent.",
		},
	})

	text := out.String()
	for _, want := range []string{
		"• Subagent",
		"Task: Inspect README",
		"subagent completed",
		"README describes local-agent.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Tool delegate_task") {
		t.Fatalf("delegate_task should render as subagent, not generic tool:\n%s", text)
	}
}

func TestBlockRendererShowsAssistantStreamingInLiveFrame(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeUserMessage,
		Message: "stream this",
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Inspect project", Status: runtimeevent.TodoInProgress},
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeAssistantDelta,
		Delta: "partial answer",
	})

	text := out.String()
	for _, want := range []string{
		"› stream this",
		"• Todo",
		"[>] Inspect project",
		"• Assistant (streaming)",
		"partial answer",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	normalized := strings.ReplaceAll(latestFrame(text), "\r\n", "\n")
	todoIndex := strings.Index(normalized, "• Todo")
	toolsIndex := strings.Index(normalized, "• Tools")
	streamIndex := strings.Index(normalized, "• Assistant (streaming)")
	if todoIndex == -1 || toolsIndex == -1 || streamIndex == -1 || todoIndex > toolsIndex || toolsIndex > streamIndex {
		t.Fatalf("todo and tools blocks should appear before streaming block:\n%s", normalized)
	}
}

func TestBlockRendererLabelsForwardedSubagentEvents(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type:       runtimeevent.TypeToolResult,
		Tool:       "read_file",
		Args:       json.RawMessage(`{"path":"README.md"}`),
		Source:     "subagent",
		ParentTool: "Inspect project docs",
		Result: &tools.Result{
			Status: "success",
			Output: "README contents",
		},
	})

	text := out.String()
	for _, want := range []string{
		"• Subagent Explored",
		"Task: Inspect project docs",
		"Read README.md",
		"README contents",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
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

func TestBlockRendererClearsApprovalPromptLinesOnDecision(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Write file", Status: runtimeevent.TodoInProgress}},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:      runtimeevent.TypeApprovalRequest,
		Tool:      "write_file",
		Category:  approval.CategoryWorkspaceWrite,
		Args:      json.RawMessage(`{"path":"hello.txt","content":"hello"}`),
		Decisions: []approval.Decision{approval.DecisionAlways, approval.DecisionDeny},
		Reason:    "workspace write requested",
	})
	clearLines := renderer.frameLines + renderer.pendingPromptLines

	renderer.HandleEvent(runtimeevent.Event{
		Type:     runtimeevent.TypeApprovalDecision,
		Tool:     "write_file",
		Decision: string(approval.DecisionAlways),
		Reason:   "workspace write requested",
	})

	if !strings.Contains(out.String(), fmt.Sprintf("\x1b[%dA", clearLines)) {
		t.Fatalf("approval decision redraw should clear frame plus prompt lines (%d); output=%q", clearLines, out.String())
	}
	if renderer.pendingPromptLines != 0 {
		t.Fatalf("pending prompt lines = %d, want cleared", renderer.pendingPromptLines)
	}
}

func TestBlockRendererRunEndStopsWatcherOutsideRendererLock(t *testing.T) {
	var out bytes.Buffer
	stop := make(chan struct{})
	done := make(chan struct{})
	renderer := NewBlockRenderer(&out)
	renderer.keyWatcher = &toggleKeyWatcher{running: true, stop: stop, done: done}
	renderer.inRun = true
	renderer.renderedFrame = true
	renderer.todos = []runtimeevent.TodoItem{{Text: "Wait for final", Status: runtimeevent.TodoInProgress}}

	finished := make(chan struct{})
	go func() {
		renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
		close(finished)
	}()

	waitForClosed(t, stop, "watcher stop")
	assertRendererLockAvailable(t, renderer)

	select {
	case <-finished:
		t.Fatalf("run end returned before watcher finished")
	default:
	}
	close(done)
	waitForClosed(t, finished, "run end")
}

func TestBlockRendererRunEndCollapsesExpandedToolsBeforeFinal(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.liveFrameMaxLines = 20
	renderer.liveFrameMaxWidth = 100

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{{Text: "Summarize findings", Status: runtimeevent.TodoCompleted}},
	})
	renderer.ToggleTools()
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "read_file",
		Args: json.RawMessage(`{"path":"README.md"}`),
		Result: &tools.Result{
			Status: "success",
			Output: strings.Repeat("long output\n", 20),
		},
	})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})

	frame := latestFrame(out.String())
	if !strings.Contains(frame, "Tools (collapsed, Ctrl+E to expand)") {
		t.Fatalf("final live frame should collapse tools before final answer:\n%q", frame)
	}
	if strings.Contains(frame, "Tools (expanded, Ctrl+E to collapse)") {
		t.Fatalf("final live frame should not leave tools expanded:\n%q", frame)
	}
}

func TestBlockRendererKeepsUserPromptInLiveFrame(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.liveFrameMaxLines = 12
	renderer.liveFrameMaxWidth = 100

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeUserMessage,
		Message: "你觉得当前项目还缺少什么？",
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "分析项目缺失能力", Status: runtimeevent.TodoInProgress},
		},
	})

	frame := strings.ReplaceAll(latestFrame(out.String()), "\r\n", "\n")
	if !strings.Contains(frame, "› 你觉得当前项目还缺少什么？") {
		t.Fatalf("live frame should include the submitted prompt:\n%s", frame)
	}
	if !strings.Contains(frame, "• Todo") {
		t.Fatalf("live frame should still include todo block:\n%s", frame)
	}
}

func TestBlockRendererClearsLiveFrameBeforeFinal(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.liveFrameMaxLines = 12
	renderer.liveFrameMaxWidth = 100

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeUserMessage,
		Message: "你觉得当前项目还缺少什么？",
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Analyze project", Status: runtimeevent.TodoCompleted},
			{Text: "Summarize findings", Status: runtimeevent.TodoCompleted},
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "read_file",
		Args: json.RawMessage(`{"path":"README.md"}`),
		Result: &tools.Result{
			Status: "success",
			Output: strings.Repeat("project detail\n", 12),
		},
	})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "第一行最终回答\n\n第二行最终回答\n\n第三行最终回答",
	})

	frame := latestFrame(out.String())
	normalized := strings.ReplaceAll(frame, "\r\n", "\n")
	if strings.Contains(normalized, "• Todo") || strings.Contains(normalized, "• Tools") {
		t.Fatalf("final answer should replace the live frame instead of appending below it:\n%q", normalized)
	}
	for _, want := range []string{"› 你觉得当前项目还缺少什么？", "第一行最终回答", "第二行最终回答", "第三行最终回答"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("final output missing %q:\n%s", want, normalized)
		}
	}
	if renderer.renderedFrame || renderer.frameLines != 0 || renderer.pendingPromptLines != 0 || renderer.userMessage != "" {
		t.Fatalf("live frame state after final = rendered:%v frame:%d prompt:%d user:%q, want cleared", renderer.renderedFrame, renderer.frameLines, renderer.pendingPromptLines, renderer.userMessage)
	}
}

func TestBlockRendererClearsAssistantStreamingStateOnRunEnd(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:  runtimeevent.TypeAssistantDelta,
		Delta: "partial answer",
	})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	if renderer.assistantMessage != "" {
		t.Fatalf("assistant streaming state = %q, want cleared", renderer.assistantMessage)
	}

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Next task", Status: runtimeevent.TodoInProgress},
		},
	})
	parts := strings.Split(strings.ReplaceAll(out.String(), "\r\n", "\n"), "\x1b[J")
	frame := parts[len(parts)-1]
	if strings.Contains(frame, "partial answer") {
		t.Fatalf("next run frame should not include previous streaming text:\n%s", frame)
	}
}

func TestBlockRendererApprovalRequestStopsWatcherOutsideRendererLock(t *testing.T) {
	var out bytes.Buffer
	stop := make(chan struct{})
	done := make(chan struct{})
	renderer := NewBlockRenderer(&out)
	renderer.keyWatcher = &toggleKeyWatcher{running: true, stop: stop, done: done}
	renderer.inRun = true

	finished := make(chan struct{})
	go func() {
		renderer.HandleEvent(runtimeevent.Event{
			Type:      runtimeevent.TypeApprovalRequest,
			Tool:      "write_file",
			Category:  approval.CategoryWorkspaceWrite,
			Args:      json.RawMessage(`{"path":"hello.txt","content":"hello"}`),
			Decisions: []approval.Decision{approval.DecisionAlways, approval.DecisionDeny},
		})
		close(finished)
	}()

	waitForClosed(t, stop, "watcher stop")
	assertRendererLockAvailable(t, renderer)

	close(done)
	waitForClosed(t, finished, "approval request")
	if renderer.pendingPromptLines == 0 {
		t.Fatalf("approval request should record pending prompt lines")
	}
}

func TestBlockRendererReleaseTerminalStopsWatcher(t *testing.T) {
	var out bytes.Buffer
	stop := make(chan struct{})
	done := make(chan struct{})
	renderer := NewBlockRenderer(&out)
	renderer.keyWatcher = &toggleKeyWatcher{running: true, stop: stop, done: done}

	finished := make(chan struct{})
	go func() {
		renderer.ReleaseTerminal()
		close(finished)
	}()

	waitForClosed(t, stop, "watcher stop")
	close(done)
	waitForClosed(t, finished, "release terminal")
}

func TestRefreshLiveFrameBoundsNoopsWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.liveFrameMaxLines = 7
	renderer.liveFrameMaxWidth = 33

	renderer.refreshLiveFrameBounds()

	if renderer.liveFrameMaxLines != 7 || renderer.liveFrameMaxWidth != 33 {
		t.Fatalf("non-terminal refresh should keep cached bounds, got lines=%d width=%d", renderer.liveFrameMaxLines, renderer.liveFrameMaxWidth)
	}
}

func TestCurrentFrameLinesForClearRewrapsStoredFrameText(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	renderer.rewriteFrame = true
	renderer.renderedFrame = true
	renderer.frameLines = 2
	renderer.frameText = "1234567890\nabc\n"
	renderer.liveFrameMaxWidth = 4

	got := renderer.currentFrameLinesForClear()
	if got != 4 {
		t.Fatalf("currentFrameLinesForClear() = %d, want 4", got)
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

func TestBlockRendererFullToolLogKeepsCompleteOutput(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)
	longOutput := strings.Repeat("0123456789", 450) + "END_OF_FULL_OUTPUT"

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "read_file",
		Args: json.RawMessage(`{"path":"README.md"}`),
		Result: &tools.Result{
			Status: "success",
			Output: longOutput,
		},
	})

	fullLog := renderer.fullToolLogText()
	if !strings.Contains(fullLog, "END_OF_FULL_OUTPUT") {
		t.Fatalf("full log should include the complete tool output")
	}
	if strings.Contains(fullLog, "… truncated") {
		t.Fatalf("full log should not use preview truncation:\n%s", fullLog)
	}
}

func TestBlockRendererFullToolLogGroupsSubagentsCollapsedByDefault(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: tools.DelegateTaskToolName,
		Args: json.RawMessage(`{"task":"Inspect README"}`),
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:          runtimeevent.TypeToolCall,
		Tool:          "read_file",
		Args:          json.RawMessage(`{"path":"README.md"}`),
		Source:        "subagent",
		ParentTool:    "Inspect README",
		SubagentIndex: 1,
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:          runtimeevent.TypeToolResult,
		Tool:          "read_file",
		Args:          json.RawMessage(`{"path":"README.md"}`),
		Source:        "subagent",
		ParentTool:    "Inspect README",
		SubagentIndex: 1,
		Result:        &tools.Result{Status: "success", Summary: "read", Output: "README details"},
	})

	fullLog := renderer.fullToolLogText()
	for _, want := range []string{
		"Main (expanded, Ctrl+0 to collapse) | 1 event(s)",
		"Subagent-1 (collapsed, Ctrl+1 to expand) | 2 event(s)",
		"Task: Inspect README",
		"Latest: Explored",
	} {
		if !strings.Contains(fullLog, want) {
			t.Fatalf("full log missing %q:\n%s", want, fullLog)
		}
	}
	if strings.Contains(fullLog, "README details") {
		t.Fatalf("collapsed subagent block should hide details:\n%s", fullLog)
	}
}

func TestBlockRendererFullToolLogExpandsAndColorsSubagentBlocks(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{
		Type:          runtimeevent.TypeToolCall,
		Tool:          "read_file",
		Args:          json.RawMessage(`{"path":"README.md"}`),
		Source:        "subagent",
		ParentTool:    "Inspect README",
		SubagentIndex: 1,
	})

	fullLog := renderer.fullToolLogTextWithState(fullLogState{ExpandedSubagents: map[int]bool{1: true}})
	for _, want := range []string{
		"Subagent-1 (expanded, Ctrl+1 to collapse) | 1 event(s)",
		"\x1b[36m[1]\x1b[0m Calling read_file",
		"    Args: {\"path\":\"README.md\"}",
	} {
		if !strings.Contains(fullLog, want) {
			t.Fatalf("full log missing %q:\n%s", want, fullLog)
		}
	}
	for _, unwanted := range []string{
		"\x1b[36mSubagent-1",
		"\x1b[36mCalling read_file",
		"\x1b[36m    Args:",
	} {
		if strings.Contains(fullLog, unwanted) {
			t.Fatalf("only event number prefix should be colored; found %q in:\n%q", unwanted, fullLog)
		}
	}
}

func TestFullLogViewerWrapsLongLines(t *testing.T) {
	lines := wrapFullLogLines("abcdef", 3)
	want := []string{"abc", "def"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("wrapped lines = %#v, want %#v", lines, want)
	}
}

func TestFullLogViewerWrapsANSIColoredLinesByVisibleWidth(t *testing.T) {
	lines := wrapFullLogLines("\x1b[36mabcdef\x1b[0m", 3)
	if len(lines) != 2 {
		t.Fatalf("wrapped lines = %#v, want 2 visible-width lines", lines)
	}
	if !strings.Contains(lines[0], "abc") || !strings.Contains(lines[1], "def") {
		t.Fatalf("wrapped lines should keep visible text grouped by width: %#v", lines)
	}
	if strings.Contains(strings.Join(lines, ""), "\x1b[3\x1b") {
		t.Fatalf("wrapped lines should not split ANSI escape sequences: %#v", lines)
	}
}

func TestFullLogViewerTogglesSubagentBlocks(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "viewer-output")
	if err != nil {
		t.Fatalf("create temp output: %v", err)
	}
	viewer := newStatefulLiveFullLogViewer(nil, output, func(state fullLogState) string {
		if state.SubagentExpanded(1) {
			return "expanded"
		}
		return "collapsed"
	}, DefaultOptions())
	viewer.width = 80

	if !viewer.refreshLines() {
		t.Fatalf("first refresh should populate lines")
	}
	if strings.Join(viewer.lines, "\n") != "collapsed" {
		t.Fatalf("lines = %#v, want collapsed", viewer.lines)
	}
	if viewer.handleInput([]byte{'1'}) {
		t.Fatalf("number shortcut should toggle, not close viewer")
	}
	if strings.Join(viewer.lines, "\n") != "expanded" {
		t.Fatalf("lines = %#v, want expanded", viewer.lines)
	}
	if viewer.handleInput([]byte{27, '[', '4', '9', ';', '5', 'u'}) {
		t.Fatalf("CSI-u ctrl+1 shortcut should toggle, not close viewer")
	}
	if strings.Join(viewer.lines, "\n") != "collapsed" {
		t.Fatalf("lines = %#v, want collapsed", viewer.lines)
	}
}

func TestFullLogViewerTogglesMainBlock(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "viewer-output")
	if err != nil {
		t.Fatalf("create temp output: %v", err)
	}
	viewer := newStatefulLiveFullLogViewer(nil, output, func(state fullLogState) string {
		if state.MainExpanded {
			return "main expanded"
		}
		return "main collapsed"
	}, DefaultOptions())
	viewer.width = 80

	if !viewer.refreshLines() {
		t.Fatalf("first refresh should populate lines")
	}
	if strings.Join(viewer.lines, "\n") != "main expanded" {
		t.Fatalf("lines = %#v, want main expanded", viewer.lines)
	}
	if viewer.handleInput([]byte{'0'}) {
		t.Fatalf("number shortcut should toggle main, not close viewer")
	}
	if strings.Join(viewer.lines, "\n") != "main collapsed" {
		t.Fatalf("lines = %#v, want main collapsed", viewer.lines)
	}
	if viewer.handleInput([]byte{27, '[', '4', '8', ';', '5', 'u'}) {
		t.Fatalf("CSI-u ctrl+0 shortcut should toggle main, not close viewer")
	}
	if strings.Join(viewer.lines, "\n") != "main expanded" {
		t.Fatalf("lines = %#v, want main expanded", viewer.lines)
	}
}

func TestFullLogViewerKeepsSplitWheelEscapeSequenceOpen(t *testing.T) {
	viewer := &fullLogViewer{
		height:            6,
		lines:             make([]string, 40),
		mainExpanded:      true,
		expandedSubagents: map[int]bool{},
	}
	for i := range viewer.lines {
		viewer.lines[i] = fmt.Sprintf("line %d", i)
	}

	input := bytes.Repeat([]byte{27, '[', 'B'}, 11)
	if len(input) != 33 {
		t.Fatalf("test input length = %d, want 33", len(input))
	}
	if viewer.handleInput(input[:32]) {
		t.Fatalf("split wheel escape sequence should not close viewer")
	}
	if viewer.offset != 10 {
		t.Fatalf("offset after first chunk = %d, want 10", viewer.offset)
	}
	if len(viewer.pendingInput) == 0 {
		t.Fatalf("first chunk should retain the partial escape sequence")
	}

	if viewer.handleInput(input[32:]) {
		t.Fatalf("completed wheel escape sequence should not close viewer")
	}
	if viewer.offset != 11 {
		t.Fatalf("offset after completed sequence = %d, want 11", viewer.offset)
	}
	if len(viewer.pendingInput) != 0 {
		t.Fatalf("completed escape sequence should clear pending input")
	}
}

func TestFullLogViewerHandlesMouseWheelCSI(t *testing.T) {
	viewer := &fullLogViewer{
		height:            6,
		lines:             make([]string, 40),
		mainExpanded:      true,
		expandedSubagents: map[int]bool{},
	}

	if viewer.handleInput([]byte("\x1b[<65;10;10M")) {
		t.Fatalf("mouse wheel down should not close viewer")
	}
	if viewer.offset != 1 {
		t.Fatalf("offset after wheel down = %d, want 1", viewer.offset)
	}
	if viewer.handleInput([]byte("\x1b[<64;10;10M")) {
		t.Fatalf("mouse wheel up should not close viewer")
	}
	if viewer.offset != 0 {
		t.Fatalf("offset after wheel up = %d, want 0", viewer.offset)
	}
}

func TestFullLogViewerClosesSingleEscapeAfterIdle(t *testing.T) {
	viewer := &fullLogViewer{}
	if viewer.handleInput([]byte{27}) {
		t.Fatalf("single escape should wait briefly before closing")
	}
	if !viewer.handlePendingInputIdle() {
		t.Fatalf("idle single escape should close viewer")
	}
}

func TestFullLogViewerRefreshesProviderText(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "viewer-output")
	if err != nil {
		t.Fatalf("create temp output: %v", err)
	}
	calls := 0
	viewer := newLiveFullLogViewer(nil, output, func() string {
		calls++
		if calls == 1 {
			return "old"
		}
		return "new"
	}, DefaultOptions())
	viewer.width = 80

	if !viewer.refreshLines() {
		t.Fatalf("first refresh should populate lines")
	}
	if strings.Join(viewer.lines, "\n") != "old" {
		t.Fatalf("lines = %#v, want old", viewer.lines)
	}
	if !viewer.refreshLines() {
		t.Fatalf("second refresh should detect changed text")
	}
	if strings.Join(viewer.lines, "\n") != "new" {
		t.Fatalf("lines = %#v, want new", viewer.lines)
	}
}

func latestFrame(output string) string {
	index := strings.LastIndex(output, "\x1b[J")
	if index < 0 {
		return output
	}
	return output[index+len("\x1b[J"):]
}

func waitForClosed(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func assertRendererLockAvailable(t *testing.T, renderer *BlockRenderer) {
	t.Helper()
	locked := make(chan struct{})
	go func() {
		renderer.mu.Lock()
		renderer.mu.Unlock()
		close(locked)
	}()
	waitForClosed(t, locked, "renderer lock")
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

func TestBlockRendererRendersTokenUsageInLiveFrame(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	// Start a run so the live frame is active.
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: "hello"})

	// Main agent token usage events.
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		PromptTokens:    100,
		CompletionTokens: 20,
		CumulativeTotal: 120,
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		PromptTokens:    200,
		CompletionTokens: 40,
		CumulativeTotal: 360,
	})

	// Subagent token usage event.
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		SubagentIndex:   1,
		ParentTool:      "Research: architecture",
		PromptTokens:    50,
		CompletionTokens: 10,
		CumulativeTotal: 60,
	})

	text := out.String()
	for _, want := range []string{
		"Tokens:",
		"360",                  // main cumulative
		"Research: architectu", // truncated task name (20 chars)
		"60",                   // subagent cumulative
		"total",                // total line
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererTracksMainAndSubagentTokensSeparately(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})

	// Main agent tokens.
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		CumulativeTotal: 500,
	})

	// Subagent tokens.
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		SubagentIndex:   1,
		ParentTool:      "subtask-1",
		CumulativeTotal: 200,
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		SubagentIndex:   2,
		ParentTool:      "subtask-2",
		CumulativeTotal: 300,
	})

	text := out.String()
	// Verify the final frame shows all three components.
	for _, want := range []string{"500 main", "subtask-1", "200", "subtask-2", "300", "total 1.0k"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}

	// Verify internal state tracks them separately.
	if renderer.mainTokenTotal != 500 {
		t.Errorf("mainTokenTotal = %d, want 500", renderer.mainTokenTotal)
	}
	if len(renderer.subagentTokens) != 2 {
		t.Errorf("subagentTokens count = %d, want 2", len(renderer.subagentTokens))
	}
	if renderer.subagentTokens[1] != 200 {
		t.Errorf("subagentTokens[1] = %d, want 200", renderer.subagentTokens[1])
	}
	if renderer.subagentTokens[2] != 300 {
		t.Errorf("subagentTokens[2] = %d, want 300", renderer.subagentTokens[2])
	}
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{12345, "12.3k"},
		{100000, "100.0k"},
	}
	for _, tt := range tests {
		got := formatTokenCount(tt.input)
		if got != tt.want {
			t.Errorf("formatTokenCount(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBlockRendererPrintsTokenSummaryAfterFinalAnswer(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	// Simulate a run with token usage.
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: "test"})
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		CumulativeTotal: 12500,
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		SubagentIndex:   1,
		ParentTool:      "Research: architecture",
		CumulativeTotal: 3200,
	})

	// Final answer is printed after RunEnd.
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "Here is the answer.",
	})

	text := stripANSI(out.String())
	// The final token summary should appear AFTER the final answer text.
	// Use LastIndex because "Tokens:" also appears in the live frame before the
	// final answer; we want to verify the stable post-answer summary position.
	finalIdx := strings.Index(text, "Here is the answer.")
	tokenIdx := strings.LastIndex(text, "Tokens:")
	if finalIdx < 0 {
		t.Fatalf("final answer not found in output:\n%s", text)
	}
	if tokenIdx < 0 {
		t.Fatalf("token summary not found in output:\n%s", text)
	}
	if tokenIdx < finalIdx {
		t.Fatalf("token summary should appear after final answer, but tokenIdx=%d < finalIdx=%d:\n%s", tokenIdx, finalIdx, text)
	}
	// Verify the summary contains expected values.
	for _, want := range []string{"12.5k main", "15.7k"} {
		if !strings.Contains(text, want) {
			t.Fatalf("token summary missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererShowsNAWhenProviderOmitsUsage(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	// Simulate a run with no token usage events (provider omitted usage).
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: "test"})
	renderer.HandleEvent(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "Answer without usage data.",
	})

	text := stripANSI(out.String())
	if !strings.Contains(text, "Tokens: N/A") {
		t.Fatalf("expected N/A token summary when provider omits usage, got:\n%s", text)
	}
}
