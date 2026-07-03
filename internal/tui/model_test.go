package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
	if !containsAll(view, "███████", "You", "hello", "Agent", "done") {
		t.Fatalf("view missing transcript content:\n%s", view)
	}
	if strings.Contains(view, "Session") {
		t.Fatalf("view should not render legacy session box label:\n%s", view)
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
	if !containsAll(view, "Tokens 34108 (p32665 c1443, cache 800)", "Ask the agent") {
		t.Fatalf("token footer missing expected summary:\n%s", view)
	}
	if strings.LastIndex(view, "Tokens 34108 (p32665 c1443, cache 800)") > strings.LastIndex(view, "Ask the agent") {
		t.Fatalf("token footer should render above the input box:\n%s", view)
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
		CumulativeTotal: 700,
		CachedTokens:    50,
	}})

	view := model.View()
	if !strings.Contains(view, "Tokens 2200 total | main 1500 | sub 700 | cache 250") {
		t.Fatalf("expected combined main/subagent token footer:\n%s", view)
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

	if model.tokens.Total != 0 || model.tokens.Prompt != 0 || model.tokens.Completion != 0 {
		t.Fatalf("tokens should reset on new run start, got %+v", model.tokens)
	}
	if strings.Contains(model.View(), "Tokens ") {
		t.Fatalf("token footer should disappear after run start reset:\n%s", model.View())
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
		CumulativeTotal: 1500,
		CachedTokens:    800,
	}})
	model.Update(runtimeEventMsg{Event: runtimeevent.Event{
		Type:            runtimeevent.TypeTokenUsage,
		Source:          "subagent",
		ParentTool:      "Trace scheduler",
		SubagentIndex:   2,
		CumulativeTotal: 2200,
		CachedTokens:    900,
	}})

	view := model.View()
	if strings.Contains(view, "cache 900") {
		t.Fatalf("unselected subagent row should not show cache breakdown:\n%s", view)
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
