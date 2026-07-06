package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
	"local-agent/internal/ui"
)

func newSizedTestModel() *Model {
	model := NewModel(ui.DefaultOptions(), ui.StartupInfo{
		Workdir: "/tmp/project",
		Model:   "test-model",
		LogFile: "/tmp/project/agent.log",
	}, NewBridge())
	model.SetSlashCommands([]ui.CommandSuggestion{
		{Name: "info", Desc: "show startup details"},
		{Name: "quit", Desc: "exit the agent"},
	})
	model.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
	return model
}

func TestRuntimeEventsRenderTranscript(t *testing.T) {
	model := newSizedTestModel()

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: "hello"}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeAssistantMessage, Message: "working on it"}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeFinal, Message: "**done**"}})

	view := model.View()
	if !containsAll(view, "███████", "hello", "working on it", "done") {
		t.Fatalf("view missing transcript content:\n%s", view)
	}
	if strings.Contains(view, "\nYou\n") || strings.Contains(view, "\nAgent\n") {
		t.Fatalf("view should hide transcript role labels:\n%s", view)
	}
	if strings.Contains(view, "Session") {
		t.Fatalf("view should not render legacy session box label:\n%s", view)
	}
}

func TestUserBlocksRenderWithPromptMarker(t *testing.T) {
	model := newSizedTestModel()

	rendered := model.renderBlock(transcriptBlock{
		Kind:  blockUser,
		Title: "You",
		Body:  "请问你谁？",
	}, 40)

	if !strings.Contains(rendered, "请问你谁？") {
		t.Fatalf("expected user question text in rendered block:\n%s", rendered)
	}
	if strings.Contains(rendered, "You") {
		t.Fatalf("user role label should be hidden inside question box:\n%s", rendered)
	}
	if !strings.Contains(rendered, "*") {
		t.Fatalf("expected user block to render with a star marker:\n%s", rendered)
	}
	if containsAll(rendered, "┌", "┐", "└", "┘") {
		t.Fatalf("user block should no longer render as a bordered box:\n%s", rendered)
	}
}

func TestAssistantBlocksRenderWithoutRoleTitle(t *testing.T) {
	model := newSizedTestModel()

	rendered := model.renderBlock(transcriptBlock{
		Kind:  blockAssistant,
		Title: "Agent",
		Body:  "我是 Echo Dust Code 的本地 coding agent。",
	}, 48)

	if !strings.Contains(rendered, "我是 Echo Dust Code 的本地 coding agent。") {
		t.Fatalf("expected assistant body in rendered block:\n%s", rendered)
	}
	if strings.Contains(rendered, "Agent") {
		t.Fatalf("assistant role label should be hidden:\n%s", rendered)
	}
}

func TestToolCallsRenderButToolResultsStayHidden(t *testing.T) {
	model := newSizedTestModel()

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: "read_file",
		Args: json.RawMessage(`{"path":"README.md"}`),
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "read_file",
		Args: json.RawMessage(`{"path":"README.md"}`),
		Result: &tools.Result{
			Status:  "success",
			Summary: "read",
			Output:  "README details",
		},
	}})

	view := model.View()
	if !strings.Contains(view, "Tool read_file") {
		t.Fatalf("expected tool call to render:\n%s", view)
	}
	if !strings.Contains(view, "●") {
		t.Fatalf("expected tool call to render with green dot marker:\n%s", view)
	}
	if !strings.Contains(view, `{"path":"README.md"}`) {
		t.Fatalf("expected tool call args to render:\n%s", view)
	}
	if strings.Contains(view, "README details") || strings.Contains(view, "Explored") {
		t.Fatalf("tool result output should stay hidden:\n%s", view)
	}
}

func TestEditToolResultsRenderDiffBlocks(t *testing.T) {
	model := newSizedTestModel()
	diff := "--- a/hello.txt\n+++ b/hello.txt\n@@ -1 +1 @@\n-old\n+new"

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "write_file",
		Result: &tools.Result{
			Status: "success",
			Changes: []tools.FileChange{
				{
					Path:         "hello.txt",
					Action:       "edited",
					AddedLines:   1,
					RemovedLines: 1,
					Diff:         diff,
					Preview:      diff,
				},
			},
		},
	}})

	view := model.View()
	if !containsAll(view, "Diff hello.txt (+1 -1)", "    1 - old", "    1 + new") {
		t.Fatalf("edit result should append diff block:\n%s", view)
	}
	if strings.Contains(view, "--- a/hello.txt") || strings.Contains(view, "+++ b/hello.txt") || strings.Contains(view, "@@ -1 +1 @@") {
		t.Fatalf("diff block should hide raw patch headers:\n%s", view)
	}
}

func TestGitDiffResultsRenderDiffBlocks(t *testing.T) {
	model := newSizedTestModel()
	diff := "diff --git a/hello.txt b/hello.txt\nindex 1111111..2222222 100644\n--- a/hello.txt\n+++ b/hello.txt\n@@ -1 +1 @@\n-old\n+new"

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "git_diff",
		Result: &tools.Result{
			Status: "success",
			Changes: []tools.FileChange{
				{
					Path:         "hello.txt",
					Action:       "edited",
					AddedLines:   1,
					RemovedLines: 1,
					Diff:         diff,
					Preview:      diff,
				},
			},
		},
	}})

	view := model.View()
	if !containsAll(view, "Diff hello.txt (+1 -1)", "    1 - old", "    1 + new") {
		t.Fatalf("git diff result should append diff block:\n%s", view)
	}
	if strings.Contains(view, "diff --git a/hello.txt b/hello.txt") || strings.Contains(view, "index 1111111..2222222 100644") {
		t.Fatalf("git diff block should render as inline code rows:\n%s", view)
	}
}

func TestDiffBlocksRenderLineNumbers(t *testing.T) {
	model := newSizedTestModel()

	rendered := model.renderBlock(transcriptBlock{
		Kind:  blockDiff,
		Title: "Diff hello.txt (+2 -1)",
		Body:  "@@ -12,2 +12,3 @@\n keep\n-old\n+new\n+next",
	}, 80)

	if !containsAll(rendered, "   12   keep", "   13 - old", "   13 + new", "   14 + next") {
		t.Fatalf("diff block should render line numbers:\n%s", rendered)
	}
	if strings.Contains(rendered, "@@ -12,2 +12,3 @@") {
		t.Fatalf("diff block should hide hunk headers:\n%s", rendered)
	}
}

func TestDiffBlocksRenderWithColor(t *testing.T) {
	model := newSizedTestModel()

	metaStyle, _, _ := model.diffLineParts("@@ -1 +1 @@")
	removeStyle, _, _ := model.diffLineParts("-old")
	addStyle, _, _ := model.diffLineParts("+new")

	if got := metaStyle.GetForeground(); got != lipgloss.Color("117") {
		t.Fatalf("meta color = %#v, want 117", got)
	}
	if got := removeStyle.GetForeground(); got != lipgloss.Color("#F2B8BD") {
		t.Fatalf("remove color = %#v, want #F2B8BD", got)
	}
	if got := removeStyle.GetBackground(); got != lipgloss.Color("#4A221D") {
		t.Fatalf("remove background = %#v, want #4A221D", got)
	}
	if got := addStyle.GetForeground(); got != lipgloss.Color("#8BD5A0") {
		t.Fatalf("add color = %#v, want #8BD5A0", got)
	}
	if got := addStyle.GetBackground(); got != lipgloss.Color("#183126") {
		t.Fatalf("add background = %#v, want #183126", got)
	}
}

func TestDiffAddedAndRemovedRowsFillAvailableWidth(t *testing.T) {
	model := newSizedTestModel()
	state := diffRenderState{oldLine: 4, newLine: 4, hasHunk: true}

	removed := model.renderDiffLine("-old", 30, &state)
	added := model.renderDiffLine("+new", 30, &state)

	if got := lipgloss.Width(removed); got != 30 {
		t.Fatalf("removed row width = %d, want 30", got)
	}
	if got := lipgloss.Width(added); got != 30 {
		t.Fatalf("added row width = %d, want 30", got)
	}
}

func TestDiffBlockRowsDoNotKeepLeftIndentGap(t *testing.T) {
	model := newSizedTestModel()

	rendered := model.renderBlock(transcriptBlock{
		Kind:  blockDiff,
		Title: "Diff hello.txt (+1 -1)",
		Body:  "@@ -1 +1 @@\n-old\n+new",
	}, 40)

	lines := strings.Split(rendered, "\n")
	if len(lines) < 3 {
		t.Fatalf("rendered diff too short:\n%s", rendered)
	}
	if got := lipgloss.Width(lines[1]); got != 40 {
		t.Fatalf("removed row width = %d, want 40 without extra left indent:\n%s", got, rendered)
	}
	if got := lipgloss.Width(lines[2]); got != 40 {
		t.Fatalf("added row width = %d, want 40 without extra left indent:\n%s", got, rendered)
	}
}

func TestDiffSyntaxHighlighterUsesGoLexer(t *testing.T) {
	highlighter := newDiffSyntaxHighlighter("--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n+func demo() string { return \"hi\" }")
	spans := highlighter.highlight("func demo() string { return \"hi\" }", lipgloss.NewStyle())

	assertSpanStyle := func(text string, want lipgloss.TerminalColor) {
		t.Helper()
		for _, span := range spans {
			if span.Text == text {
				if got := span.Style.GetForeground(); got != want {
					t.Fatalf("span %q foreground = %#v, want %#v", text, got, want)
				}
				return
			}
		}
		t.Fatalf("span %q not found in %#v", text, spans)
	}

	assertSpanStyle("func", lipgloss.Color("#8AADF4"))
	assertSpanStyle("demo", lipgloss.Color("#C6A0F6"))
	assertSpanStyle("\"hi\"", lipgloss.Color("#A6DA95"))
}

func TestDiffSyntaxHighlighterFallsBackToFilenameAgnosticAnalysis(t *testing.T) {
	highlighter := newDiffSyntaxHighlighter("@@ -1 +1 @@\n+{\"name\":\"echo\",\"count\":1}")
	spans := highlighter.highlight("{\"name\":\"echo\",\"count\":1}", lipgloss.NewStyle())

	foundString := false
	foundNumber := false
	for _, span := range spans {
		switch {
		case strings.Contains(span.Text, "\"echo\""):
			foundString = span.Style.GetForeground() == lipgloss.Color("#A6DA95")
		case strings.Contains(span.Text, "1"):
			foundNumber = span.Style.GetForeground() == lipgloss.Color("#F5A97F")
		}
	}
	if !foundString {
		t.Fatalf("expected JSON string highlighting in %#v", spans)
	}
	if !foundNumber {
		t.Fatalf("expected JSON number highlighting in %#v", spans)
	}
}

func TestIdleViewShowsOnlyBannerAndInputChrome(t *testing.T) {
	model := newSizedTestModel()

	view := model.View()
	for _, unwanted := range []string{
		"cwd /tmp/project",
		"model test-model",
		"status idle",
		"Ready",
		"Use /info, /model, /quit or type a task to start.",
		"No conversation yet.",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("idle view should not render %q:\n%s", unwanted, view)
		}
	}
}

func TestTokenFooterRendersAboveInputBox(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:             runtimeevent.TypeTokenUsage,
		PromptTokens:     32665,
		CompletionTokens: 1443,
		CumulativeTotal:  34108,
		CachedTokens:     800,
	}})

	view := model.View()
	if !containsAll(view, "Tokens 34.1k (p32.7k c1.4k, cache 800, hit 2.4%)", "Ask the agent") {
		t.Fatalf("token footer missing expected summary:\n%s", view)
	}
	if strings.LastIndex(view, "Tokens 34.1k (p32.7k c1.4k, cache 800, hit 2.4%)") > strings.LastIndex(view, "Ask the agent") {
		t.Fatalf("token footer should render above the input box:\n%s", view)
	}
}

func TestLiveRunTimerRendersInLeftStatusBar(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.runStartedAt = time.Now().Add(-20600 * time.Millisecond)
	model.runElapsedMS = 20600

	view := model.View()
	if !containsAll(view, "Total · 20.6s", "Ask the agent") {
		t.Fatalf("live run timer missing from input area:\n%s", view)
	}
	if strings.LastIndex(view, "Total · 20.6s") > strings.LastIndex(view, "Ask the agent") {
		t.Fatalf("live run timer should render above the input box:\n%s", view)
	}
}

func TestLiveRunTimerSharesStatusBarWithTokenSummary(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.runStartedAt = time.Now().Add(-20600 * time.Millisecond)
	model.runElapsedMS = 20600
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:             runtimeevent.TypeTokenUsage,
		PromptTokens:     32665,
		CompletionTokens: 1443,
		CumulativeTotal:  34108,
		CachedTokens:     800,
	}})

	view := model.View()
	var statusLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Total · 20.6s") {
			statusLine = line
			break
		}
	}
	if statusLine == "" || !strings.Contains(statusLine, "Tokens 34.1k (p32.7k c1.4k, cache 800, hit 2.4%)") {
		t.Fatalf("expected timer and token summary to share one status line:\n%s", view)
	}
	if strings.Index(statusLine, "Total · 20.6s") > strings.Index(statusLine, "Tokens 34.1k (p32.7k c1.4k, cache 800, hit 2.4%)") {
		t.Fatalf("expected timer on the left and tokens on the right:\n%s", view)
	}
}

func TestRunEndHidesLiveTimerButKeepsFinalTotalBlock(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.runStartedAt = time.Now().Add(-20600 * time.Millisecond)
	model.runElapsedMS = 20600
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunEnd}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunTiming, DurationMS: 20600}})

	view := model.View()
	if strings.Count(view, "Total · 20.6s") != 1 {
		t.Fatalf("expected only the final transcript total after run end:\n%s", view)
	}
	if strings.LastIndex(view, "Total · 20.6s") > strings.LastIndex(view, "Ask the agent") {
		t.Fatalf("final total should stay in the transcript, not the input box, after run end:\n%s", view)
	}
}

func TestTokenFooterIncludesSubagentTotals(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:             runtimeevent.TypeTokenUsage,
		PromptTokens:     1200,
		CompletionTokens: 300,
		CumulativeTotal:  1500,
		CachedTokens:     200,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Inspect README",
		SubagentIndex:   1,
		PromptTokens:    500,
		CumulativeTotal: 700,
		CachedTokens:    50,
	}})

	view := model.View()
	if !strings.Contains(view, "Tokens 2.2k total | main 1.5k | sub 700 | cache 250 | hit 14.7%") {
		t.Fatalf("expected combined main/subagent token footer:\n%s", view)
	}
}

func TestTokenFooterUsesMillionSuffixForLargeCounts(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		PromptTokens:    11647,
		CumulativeTotal: 11647,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Analyze repo",
		SubagentIndex:   1,
		PromptTokens:    1200000,
		CumulativeTotal: 1398223,
		CachedTokens:    1127680,
	}})

	view := model.View()
	if !strings.Contains(view, "Tokens 1.4m total | main 11.6k | sub 1.4m | cache 1.1m | hit 93.1%") {
		t.Fatalf("expected footer to use compact million suffixes:\n%s", view)
	}
}

func TestRunStartResetsTokenFooter(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:             runtimeevent.TypeTokenUsage,
		PromptTokens:     100,
		CompletionTokens: 20,
		CumulativeTotal:  120,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})

	if model.tokens.Total != 0 || model.tokens.Prompt != 0 || model.tokens.Completion != 0 || model.tokens.Cached != 0 {
		t.Fatalf("tokens should reset on new run start, got %+v", model.tokens)
	}
	if strings.Contains(model.View(), "Tokens ") {
		t.Fatalf("token footer should disappear after run start reset:\n%s", model.View())
	}
}

func TestTodoRendersInMainContentDuringRun(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Read files", Status: runtimeevent.TodoInProgress},
			{Text: "Summarize findings", Status: runtimeevent.TodoCompleted},
		},
	}})

	view := model.View()
	if strings.Contains(view, "Todo") {
		t.Fatalf("todo title should not render in checklist mode:\n%s", view)
	}
	if !containsAll(view, "□ Read files", "■ Summarize findings") {
		t.Fatalf("todo checklist should render in main content while running:\n%s", view)
	}
}

func TestTodoStaysAtEndOfCurrentRunContent(t *testing.T) {
	model := newSizedTestModel()
	model.appendBlock(transcriptBlock{
		Kind:  blockAssistant,
		Title: "Agent",
		Body:  "previous run",
	})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeUserMessage,
		Message: "当前项目是做什么？",
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Handle request: 当前项目是做什么？", Status: runtimeevent.TodoInProgress},
		},
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: "list_files",
		Args: json.RawMessage(`{"path":"."}`),
	}})

	view := model.View()
	userIndex := strings.Index(view, "当前项目是做什么？")
	toolIndex := strings.Index(view, "Tool list_files")
	todoIndex := strings.Index(view, "□ Handle request: 当前项目是做什么？")
	if userIndex < 0 || todoIndex < 0 || toolIndex < 0 {
		t.Fatalf("expected user message, todo block and tool call in view:\n%s", view)
	}
	if !(userIndex < toolIndex && toolIndex < todoIndex) {
		t.Fatalf("todo block should stay at the end of the current run content:\n%s", view)
	}
}

func TestCurrentRunAssistantBodyHidesDuplicatedTodoChecklist(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeAssistantMessage,
		Message: "Working on it.\n- [x] Add selectionMode field to Model\n- [x] Create internal/tui/selection_mode.go",
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Add selectionMode field to Model", Status: runtimeevent.TodoCompleted},
			{Text: "Create internal/tui/selection_mode.go", Status: runtimeevent.TodoCompleted},
		},
	}})

	view := model.View()
	if !strings.Contains(view, "Working on it.") {
		t.Fatalf("assistant prose should remain visible:\n%s", view)
	}
	if strings.Contains(view, "- [x] Add selectionMode field to Model") || strings.Contains(view, "- [x] Create internal/tui/selection_mode.go") {
		t.Fatalf("duplicated todo checklist should not remain in assistant body:\n%s", view)
	}
	if !containsAll(view, "■ Add selectionMode field to Model", "■ Create internal/tui/selection_mode.go") {
		t.Fatalf("live todo block should still render:\n%s", view)
	}
}

func TestStreamingAssistantBodyStripsOrphanThinkTag(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:  runtimeevent.TypeAssistantDelta,
		Delta: "</think>\nVisible answer",
	}})

	view := model.View()
	if strings.Contains(view, "</think>") || strings.Contains(view, "<think>") {
		t.Fatalf("think tags should not render in streaming body:\n%s", view)
	}
	if !strings.Contains(view, "Visible answer") {
		t.Fatalf("visible streaming text should remain:\n%s", view)
	}
}

func TestRunWithoutTodoDoesNotRenderTodoBlock(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeUserMessage,
		Message: "hello",
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeAssistantMessage,
		Message: "Hello!",
	}})

	view := model.View()
	if strings.Contains(view, "Todo") || strings.Contains(view, "Waiting for todo list") {
		t.Fatalf("plain text runs should not render todo block before a real todo update:\n%s", view)
	}
}

func TestTodoBlockHidesAfterRunEnd(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Read files", Status: runtimeevent.TodoInProgress},
		},
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunEnd}})

	view := model.View()
	if strings.Contains(view, "□ Read files") || strings.Contains(view, "■ Read files") {
		t.Fatalf("todo block should hide after run end:\n%s", view)
	}
}

func TestChatRetryRendersBelowTranscriptAndAboveTodo(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeAssistantMessage,
		Message: "working on it",
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Read files", Status: runtimeevent.TodoInProgress},
		},
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:       runtimeevent.TypeChatRetry,
		Count:      1,
		After:      5,
		DurationMS: (11 * time.Second).Milliseconds(),
		Message:    "LLM request timed out. Waiting before retry.",
	}})

	view := model.View()
	assistantIndex := strings.Index(view, "working on it")
	retryIndex := strings.Index(view, "Retrying chat... 1/5 (11s · esc to interrupt)")
	todoIndex := strings.Index(view, "□ Read files")
	if assistantIndex < 0 || retryIndex < 0 || todoIndex < 0 {
		t.Fatalf("expected assistant body, retry block and todo block in view:\n%s", view)
	}
	if !(assistantIndex < retryIndex && retryIndex < todoIndex) {
		t.Fatalf("retry block should render below transcript output and above todo:\n%s", view)
	}
	if !strings.Contains(view, "LLM request timed out. Waiting before retry.") {
		t.Fatalf("retry detail missing from view:\n%s", view)
	}

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeAssistantMessage,
		Message: "retried answer",
	}})
	view = model.View()
	if strings.Contains(view, "Retrying chat...") {
		t.Fatalf("retry block should clear after new assistant progress:\n%s", view)
	}
}

func TestChatRetryCountdownRefreshesOnRunTick(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{Type: runtimeevent.TypeRunStart}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:       runtimeevent.TypeChatRetry,
		Count:      1,
		After:      5,
		DurationMS: (11 * time.Second).Milliseconds(),
		Message:    "LLM request timed out. Waiting before retry.",
	}})

	view := model.View()
	if !strings.Contains(view, "Retrying chat... 1/5 (11s · esc to interrupt)") {
		t.Fatalf("initial retry countdown missing from view:\n%s", view)
	}

	model.chatRetry.Until = time.Now().Add(2 * time.Second)
	model.Update(runTimerTickMsg{At: time.Now()})

	view = model.View()
	if !strings.Contains(view, "Retrying chat... 1/5 (2s · esc to interrupt)") {
		t.Fatalf("retry countdown should refresh on run tick:\n%s", view)
	}
}

func TestMouseWheelScrollsViewport(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for i := 0; i < 24; i++ {
		model.appendBlock(transcriptBlock{
			Kind:  blockInfo,
			Title: "Event",
			Body:  "line",
		})
	}
	model.syncLayout()
	model.viewport.GotoTop()

	before := model.viewport.YOffset
	model.Update(tea.MouseMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
		Type:   tea.MouseWheelDown,
	})

	if model.viewport.YOffset <= before {
		t.Fatalf("expected viewport to scroll, before=%d after=%d", before, model.viewport.YOffset)
	}
}

func TestMouseWheelDoesNotDirtyViewportContent(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for i := 0; i < 24; i++ {
		model.appendBlock(transcriptBlock{
			Kind:  blockInfo,
			Title: "Event",
			Body:  "line",
		})
	}
	model.syncLayout()
	_ = model.View()
	if model.layoutDirty || model.viewportDirty || model.subagentViewportDirty {
		t.Fatalf("expected clean layout before scroll, got layout=%v viewport=%v subagent=%v", model.layoutDirty, model.viewportDirty, model.subagentViewportDirty)
	}

	model.viewport.GotoTop()
	model.Update(tea.MouseMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
		Type:   tea.MouseWheelDown,
	})

	if model.layoutDirty || model.viewportDirty || model.subagentViewportDirty {
		t.Fatalf("scroll should not rebuild transcript, got layout=%v viewport=%v subagent=%v", model.layoutDirty, model.viewportDirty, model.subagentViewportDirty)
	}
	_ = model.View()
	if model.layoutDirty || model.viewportDirty || model.subagentViewportDirty {
		t.Fatalf("view after scroll should stay clean, got layout=%v viewport=%v subagent=%v", model.layoutDirty, model.viewportDirty, model.subagentViewportDirty)
	}
}

func TestTypingSlashCommandMarksLayoutDirtyAndRendersSuggestions(t *testing.T) {
	model := newSizedTestModel()
	_ = model.View()
	if model.layoutDirty || model.viewportDirty || model.subagentViewportDirty {
		t.Fatalf("expected clean layout before typing, got layout=%v viewport=%v subagent=%v", model.layoutDirty, model.viewportDirty, model.subagentViewportDirty)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !model.layoutDirty {
		t.Fatal("typing slash should mark layout dirty so suggestions can resize the transcript viewport")
	}

	view := model.View()
	if !containsAll(view, "/info", "/quit") {
		t.Fatalf("expected slash suggestions after typing '/':\n%s", view)
	}
	if model.layoutDirty || model.viewportDirty || model.subagentViewportDirty {
		t.Fatalf("expected clean layout after rendering suggestions, got layout=%v viewport=%v subagent=%v", model.layoutDirty, model.viewportDirty, model.subagentViewportDirty)
	}
}

func TestTabAcceptsSlashSuggestion(t *testing.T) {
	model := newSizedTestModel()

	model.input.SetValue("/qu")
	model.Update(tea.KeyMsg{Type: tea.KeyTab})

	if got := model.input.Value(); got != "/quit" {
		t.Fatalf("input value = %q, want /quit", got)
	}
}

func TestLongInputExpandsInputBox(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	model.input.SetValue("path/filepath import .gitignore 忽略 ECHODUST.md 测试通过 cmd/agent 和 internal/memory 包均正常。")
	model.markLayoutDirty()

	model.syncLayout()

	if model.input.Height() <= 1 {
		t.Fatalf("expected input height to expand, got %d", model.input.Height())
	}
	view := model.View()
	if !strings.Contains(view, "path/filepath") || !strings.Contains(view, "internal/memory") {
		t.Fatalf("expanded input content missing from view:\n%s", view)
	}
}

func TestPastedMultilineInputExpandsInputBox(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	model.input.SetValue("第一行内容\n第二行内容\n第三行内容")
	model.markLayoutDirty()

	model.syncLayout()

	if model.input.Height() < 3 {
		t.Fatalf("expected pasted multiline input to use at least 3 lines, got %d", model.input.Height())
	}
	view := model.View()
	if !containsAll(view, "第一行内容", "第二行内容", "第三行内容") {
		t.Fatalf("multiline input content missing from view:\n%s", view)
	}
}

func TestDefaultPlaceholderHintsAtMouseMode(t *testing.T) {
	model := newSizedTestModel()

	if !model.mouseEnabled {
		t.Fatal("mouse capture should start enabled")
	}
	if !strings.Contains(model.input.Placeholder, "F2 text copy") {
		t.Fatalf("unexpected placeholder: %q", model.input.Placeholder)
	}
}

func TestF2TogglesMouseModeAndPlaceholder(t *testing.T) {
	model := newSizedTestModel()

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyF2})
	if model.mouseEnabled {
		t.Fatal("expected F2 to disable mouse mode")
	}
	if !strings.Contains(model.input.Placeholder, "F2 mouse scroll") {
		t.Fatalf("unexpected disabled placeholder: %q", model.input.Placeholder)
	}
	if cmd == nil {
		t.Fatal("expected F2 toggle to emit a Bubble Tea command")
	}

	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !model.mouseEnabled {
		t.Fatal("expected second F2 to enable mouse mode")
	}
	if !strings.Contains(model.input.Placeholder, "F2 text copy") {
		t.Fatalf("unexpected enabled placeholder: %q", model.input.Placeholder)
	}
	if cmd == nil {
		t.Fatal("expected second F2 toggle to emit a Bubble Tea command")
	}
}

func TestMouseWheelIgnoredWhileCopyModeEnabled(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	for i := 0; i < 24; i++ {
		model.appendBlock(transcriptBlock{
			Kind:  blockInfo,
			Title: "Event",
			Body:  "line",
		})
	}
	model.syncLayout()
	model.viewport.GotoTop()
	model.Update(tea.KeyMsg{Type: tea.KeyF2})
	_ = model.View()

	before := model.viewport.YOffset
	model.Update(tea.MouseMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
		Type:   tea.MouseWheelDown,
	})
	if model.viewport.YOffset != before {
		t.Fatalf("expected mouse wheel to be ignored while copy mode is active, before=%d after=%d", before, model.viewport.YOffset)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyF2})
	_ = model.View()
	model.Update(tea.MouseMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
		Type:   tea.MouseWheelDown,
	})
	if model.viewport.YOffset <= before {
		t.Fatalf("expected viewport to scroll after re-enabling mouse mode, before=%d after=%d", before, model.viewport.YOffset)
	}
}

func TestEscInterruptsRunningAgent(t *testing.T) {
	model := newSizedTestModel()
	interrupted := false
	model.running = true
	model.cancelCurrent = func() { interrupted = true }

	model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if !interrupted {
		t.Fatal("expected esc to interrupt the running agent")
	}
	if !model.interrupting {
		t.Fatal("expected interrupting state to be set")
	}
}

func TestEscInterruptsRunningApprovalPrompt(t *testing.T) {
	model := newSizedTestModel()
	interrupted := false
	model.running = true
	model.cancelCurrent = func() { interrupted = true }
	response := make(chan approval.Decision, 1)

	model.Update(approvalPromptMsg{
		Request:  approval.Request{Tool: "write_file", Category: approval.CategoryWorkspaceWrite},
		Response: response,
	})
	model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if !interrupted {
		t.Fatal("expected esc to interrupt a run even when approval is open")
	}
	if model.approval != nil {
		t.Fatal("expected approval prompt to close on esc interrupt")
	}
	select {
	case got := <-response:
		if got != approval.DecisionDeny {
			t.Fatalf("decision = %q, want %q", got, approval.DecisionDeny)
		}
	default:
		t.Fatal("expected approval decision to be sent")
	}
}

func TestApprovalPromptSelectsFirstOptionOnEnter(t *testing.T) {
	model := newSizedTestModel()
	response := make(chan approval.Decision, 1)

	model.Update(approvalPromptMsg{
		Request:  approval.Request{Tool: "write_file", Category: approval.CategoryWorkspaceWrite},
		Response: response,
	})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	select {
	case got := <-response:
		if got != approval.DecisionAllow {
			t.Fatalf("decision = %q, want %q", got, approval.DecisionAllow)
		}
	default:
		t.Fatal("expected approval decision to be sent")
	}
}

func TestApprovalPromptRendersInlineUnderApprovalRequest(t *testing.T) {
	model := newSizedTestModel()
	response := make(chan approval.Decision, 1)
	request := approval.Request{
		Tool:     "write_file",
		Category: approval.CategoryWorkspaceWrite,
		Args:     json.RawMessage(`{"path":"notes.txt","content":"hello"}`),
		Reason:   "workspace write requested",
		Scope:    approval.ScopeSession,
		Key:      "workspace_write",
		Options:  []approval.Decision{approval.DecisionAlways, approval.DecisionDeny},
	}

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:      runtimeevent.TypeApprovalRequest,
		Tool:      request.Tool,
		Category:  request.Category,
		Args:      request.Args,
		Reason:    request.Reason,
		Decisions: request.Options,
	}})
	model.Update(approvalPromptMsg{Request: request, Response: response})

	view := model.View()
	if strings.Contains(view, "Approval Required") {
		t.Fatalf("approval should render inline instead of full-screen modal:\n%s", view)
	}
	if !containsAll(
		view,
		"Approval requested",
		"write_file [workspace_write]: workspace write requested",
		"Always allow workspace writes this session",
		"Deny",
		"Ask the agent",
	) {
		t.Fatalf("inline approval view missing expected content:\n%s", view)
	}
}

func TestApprovalPromptFallsBackInlineBeforeRuntimeEventArrives(t *testing.T) {
	model := newSizedTestModel()
	response := make(chan approval.Decision, 1)
	request := approval.Request{
		Tool:     "run_command",
		Category: approval.CategoryExternalOrDestructive,
		Args:     json.RawMessage(`{"command":"rm -rf /tmp/demo"}`),
		Reason:   "external write requested",
		Scope:    approval.ScopeLoop,
		Options:  []approval.Decision{approval.DecisionAllow, approval.DecisionAlways, approval.DecisionDeny},
	}

	model.Update(approvalPromptMsg{Request: request, Response: response})

	view := model.View()
	if !containsAll(view, "Approval requested", "run_command [external_or_destructive]: external write requested", "Allow once", "Deny") {
		t.Fatalf("inline approval fallback should render from prompt state alone:\n%s", view)
	}
}

func TestSubagentPanelCollapsesOutputByDefault(t *testing.T) {
	model := newSizedTestModel()
	seedSubagent(model, 1, "Inspect README", "README details")

	view := model.View()
	if !containsAll(view, "Subagents", "Inspect README", "Subagent-1") {
		t.Fatalf("view missing subagent list content:\n%s", view)
	}
	if strings.Contains(view, "README details") {
		t.Fatalf("collapsed subagent panel should hide detailed output:\n%s", view)
	}
	if len(model.blocks) != 0 {
		t.Fatalf("main transcript should keep subagent events out, got %d blocks", len(model.blocks))
	}
}

func TestSubagentSelectionAndDetailToggle(t *testing.T) {
	model := newSizedTestModel()
	seedSubagent(model, 1, "Inspect README", "README details")
	seedSubagent(model, 2, "Trace scheduler", "scheduler details")

	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if model.selectedSubagent != 2 {
		t.Fatalf("selected subagent = %d, want 2", model.selectedSubagent)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !model.viewingSubagent {
		t.Fatal("expected enter to open subagent detail view")
	}
	if !strings.Contains(model.View(), "Tool read_file") {
		t.Fatalf("detail view should show tool call name:\n%s", model.View())
	}
	if !strings.Contains(model.View(), `{"path":"README.md"}`) {
		t.Fatalf("detail view should show tool call args:\n%s", model.View())
	}
	if strings.Contains(model.View(), "scheduler details") {
		t.Fatalf("detail view should hide tool result output:\n%s", model.View())
	}

	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.viewingSubagent {
		t.Fatal("expected esc to leave subagent detail view")
	}
	if strings.Contains(model.View(), "scheduler details") {
		t.Fatalf("collapsed list should hide subagent detail after esc:\n%s", model.View())
	}
}

func TestSubagentPanelHidesAfterFinal(t *testing.T) {
	model := newSizedTestModel()
	seedSubagent(model, 1, "Inspect README", "README details")
	if !strings.Contains(model.View(), "Subagents") {
		t.Fatalf("expected subagent panel before final:\n%s", model.View())
	}

	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "done",
	}})

	if strings.Contains(model.View(), "Subagents") {
		t.Fatalf("subagent panel should hide after final:\n%s", model.View())
	}
}

func TestSubagentRowUsesCompactTaskSummary(t *testing.T) {
	model := newSizedTestModel()
	task := "深入分析 TUI 的上下文 (context) 相关代码，重点关注以下方面：1. transcript block 的创建与渲染"
	seedSubagent(model, 1, task, "README details")

	view := model.View()
	if !strings.Contains(view, "深入分析 TUI 的上下文") {
		t.Fatalf("expected compact task summary in subagent row:\n%s", view)
	}
	if strings.Contains(view, "重点关注以下方面") {
		t.Fatalf("subagent row should hide long trailing task detail:\n%s", view)
	}
}

func TestSubagentRowShowsOwnTokenTotal(t *testing.T) {
	model := newSizedTestModel()
	seedSubagent(model, 1, "Inspect README", "README details")
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Inspect README",
		SubagentIndex:   1,
		PromptTokens:    1000,
		CumulativeTotal: 1500,
		CachedTokens:    800,
	}})

	view := model.View()
	if !strings.Contains(view, "· 1.5k") {
		t.Fatalf("expected subagent row to show its own token total:\n%s", view)
	}
	if !strings.Contains(view, "cache 800") {
		t.Fatalf("expected selected subagent row to show cache usage:\n%s", view)
	}
	if !strings.Contains(view, "hit 80.0%") {
		t.Fatalf("expected selected subagent row to show cache hit rate:\n%s", view)
	}
}

func TestOnlySelectedSubagentRowShowsCacheBreakdown(t *testing.T) {
	model := newSizedTestModel()
	seedSubagent(model, 1, "Inspect README", "README details")
	seedSubagent(model, 2, "Trace scheduler", "scheduler details")
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Inspect README",
		SubagentIndex:   1,
		PromptTokens:    1000,
		CumulativeTotal: 1500,
		CachedTokens:    800,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Trace scheduler",
		SubagentIndex:   2,
		PromptTokens:    1000,
		CumulativeTotal: 2200,
		CachedTokens:    900,
	}})

	view := model.View()
	if strings.Contains(view, "cache 900") {
		t.Fatalf("unselected subagent row should not show cache breakdown:\n%s", view)
	}
	if strings.Contains(view, "hit 90.0%") {
		t.Fatalf("unselected subagent row should not show cache hit rate:\n%s", view)
	}
	if !strings.Contains(view, "· 2.2k") {
		t.Fatalf("every subagent row should still show total tokens:\n%s", view)
	}
}

func TestHeaderTodoSummaryStaysSingleLine(t *testing.T) {
	model := newSizedTestModel()
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type: runtimeevent.TypeTodoUpdate,
		Todos: []runtimeevent.TodoItem{
			{Text: "Handle request: 你觉得当前项目还缺什么功能以及下一步优先做什么，这是一段明显超长的进行中任务描述", Status: runtimeevent.TodoInProgress},
		},
	}})

	header := model.renderHeader()
	if strings.Contains(header, "... truncated") {
		t.Fatalf("header should not include block truncation marker:\n%s", header)
	}
	if strings.Contains(header, "\n...") {
		t.Fatalf("header todo summary should stay single-line within the meta row:\n%s", header)
	}
}

func TestEndAndMouseWheelWorkForSubagentDetailViewport(t *testing.T) {
	model := newSizedTestModel()
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	seedSubagent(model, 1, "Inspect README", "final summary")
	for i := 0; i < 24; i++ {
		model.Update(runtimeEventMsg{Event: runtimeevent.Event{
			Type:          runtimeevent.TypeAssistantMessage,
			Message:       "detail line",
			Source:        "subagent",
			ParentTool:    "Inspect README",
			SubagentIndex: 1,
		}})
	}

	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model.subagentViewport.GotoTop()
	before := model.subagentViewport.YOffset
	model.Update(tea.MouseMsg{
		X:      1,
		Y:      1,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
		Type:   tea.MouseWheelDown,
	})
	if model.subagentViewport.YOffset <= before {
		t.Fatalf("expected subagent viewport to scroll, before=%d after=%d", before, model.subagentViewport.YOffset)
	}

	model.subagentViewport.GotoTop()
	model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !model.subagentViewport.AtBottom() {
		t.Fatalf("expected End to jump subagent viewport to bottom, offset=%d", model.subagentViewport.YOffset)
	}
}

func seedSubagent(model *Model, index int, task, output string) {
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:          runtimeevent.TypeToolCall,
		Tool:          tools.DelegateTaskToolName,
		Args:          json.RawMessage(`{"task":"` + task + `"}`),
		SubagentIndex: index,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:          runtimeevent.TypeToolCall,
		Tool:          "read_file",
		Args:          json.RawMessage(`{"path":"README.md"}`),
		Source:        "subagent",
		ParentTool:    task,
		SubagentIndex: index,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:          runtimeevent.TypeToolResult,
		Tool:          "read_file",
		Args:          json.RawMessage(`{"path":"README.md"}`),
		Source:        "subagent",
		ParentTool:    task,
		SubagentIndex: index,
		Result: &tools.Result{
			Status:  "success",
			Summary: "read",
			Output:  output,
		},
	}})
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
