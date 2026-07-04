package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/session"
)

func TestSessionSnapshotRoundTrip(t *testing.T) {
	model := newSizedTestModel()
	model.appendBlock(transcriptBlock{
		Kind:     blockAssistant,
		Title:    "Agent",
		Body:     "**done**",
		Markdown: true,
	})
	model.appendBlock(transcriptBlock{
		Kind:  blockToolCall,
		Title: "Tool git_log",
		Body:  `{"limit":10}`,
	})
	model.subagents[1] = &subagentSession{
		Index:      1,
		Task:       "Inspect config",
		Status:     "done",
		Prompt:     10,
		TokenTotal: 15,
		Cached:     2,
		Blocks: []transcriptBlock{
			{Kind: blockInfo, Title: "Tool read_file", Body: "config.yaml"},
			{Kind: blockToolCall, Title: "Tool git_log", Body: `{"limit":10}`},
		},
	}
	model.subagentOrder = []int{1}
	model.showSubagents = true
	model.tokens = tokenState{Prompt: 20, Completion: 5, Total: 25, Cached: 3}

	snapshot := model.SessionSnapshot()
	restored := newSizedTestModel()
	restored.LoadSessionSnapshot(snapshot)

	if len(restored.blocks) != 1 || restored.blocks[0].Body != "**done**" || !restored.blocks[0].Markdown {
		t.Fatalf("restored blocks = %#v", restored.blocks)
	}
	if len(snapshot.Subagents) != 0 {
		t.Fatalf("snapshot should not persist subagents: %#v", snapshot.Subagents)
	}
	if len(restored.subagentOrder) != 0 || len(restored.subagents) != 0 || restored.showSubagents {
		t.Fatalf("restored subagents = %#v %#v show=%v", restored.subagentOrder, restored.subagents, restored.showSubagents)
	}
	if restored.tokens.Total != 25 || restored.tokens.Cached != 3 {
		t.Fatalf("restored tokens = %+v", restored.tokens)
	}
}

func TestSessionSnapshotPersistsDiffBlocks(t *testing.T) {
	model := newSizedTestModel()
	model.appendBlock(transcriptBlock{
		Kind:  blockDiff,
		Title: "Diff hello.txt (+1 -1)",
		Body:  "--- a/hello.txt\n+++ b/hello.txt\n@@ -1 +1 @@\n-old\n+new",
	})

	snapshot := model.SessionSnapshot()
	restored := newSizedTestModel()
	restored.LoadSessionSnapshot(snapshot)

	if len(restored.blocks) != 1 {
		t.Fatalf("restored blocks = %#v", restored.blocks)
	}
	if restored.blocks[0].Kind != blockDiff || !strings.Contains(restored.blocks[0].Body, "+new") {
		t.Fatalf("restored diff block = %#v", restored.blocks[0])
	}
}

func TestLoadSessionSnapshotClearsStateAndAppendInfoBlock(t *testing.T) {
	model := newSizedTestModel()
	model.running = true
	model.todos = []runtimeevent.TodoItem{{Text: "pending", Status: runtimeevent.TodoInProgress}}
	model.assistantDraft = "streaming"

	model.LoadSessionSnapshot(session.UISnapshot{})
	if model.running || len(model.todos) != 0 || model.assistantDraft != "" {
		t.Fatalf("expected running/todos/draft to reset: running=%v todos=%d draft=%q", model.running, len(model.todos), model.assistantDraft)
	}

	model.AppendInfoBlock("Session", "Resumed session demo")
	if len(model.blocks) != 1 {
		t.Fatalf("blocks len = %d", len(model.blocks))
	}
	if !strings.Contains(model.View(), "Resumed session demo") {
		t.Fatalf("resume notice missing from view:\n%s", model.View())
	}
}

func TestLoadSessionSnapshotIgnoresLegacySubagents(t *testing.T) {
	model := newSizedTestModel()
	model.LoadSessionSnapshot(session.UISnapshot{
		Blocks: []session.TranscriptBlockSnapshot{
			{Kind: "assistant", Body: "done"},
		},
		Subagents: []session.SubagentSnapshot{
			{
				Index:      1,
				Task:       "Inspect config",
				Status:     "done",
				TokenTotal: 42,
				Blocks: []session.TranscriptBlockSnapshot{
					{Kind: "info", Title: "Tool read_file", Body: "config.yaml"},
				},
			},
		},
		Tokens: session.TokenSnapshot{Prompt: 10, Completion: 5, Total: 15, Cached: 2},
	})

	if len(model.blocks) != 1 || model.blocks[0].Body != "done" {
		t.Fatalf("restored blocks = %#v", model.blocks)
	}
	if len(model.subagentOrder) != 0 || len(model.subagents) != 0 || model.showSubagents {
		t.Fatalf("legacy subagents should be ignored: %#v %#v show=%v", model.subagentOrder, model.subagents, model.showSubagents)
	}
	if strings.Contains(model.View(), "Subagents") {
		t.Fatalf("resume view should not render subagent panel:\n%s", model.View())
	}
}

func TestResumePickerOpensAndSelectsSession(t *testing.T) {
	model := newSizedTestModel()
	selected := ""
	model.SetResumePickerHandlers(
		func() ([]session.Meta, error) {
			return []session.Meta{
				{SessionID: "20260703T162408Z-f81a", Title: "hello"},
				{SessionID: "20260703T162900Z-a1b2", Title: "world"},
			}, nil
		},
		func(sessionID string) (string, error) {
			selected = sessionID
			return "resumed " + sessionID, nil
		},
	)

	model.input.SetValue("/resume")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := model.View()
	if !containsAll(view, "Resume Session", "20260703T162408Z-f81a", "Up/Down or J/K choose") {
		t.Fatalf("resume picker missing expected content:\n%s", view)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if selected != "20260703T162900Z-a1b2" {
		t.Fatalf("selected session = %q", selected)
	}
	if model.resumePicker != nil {
		t.Fatalf("resume picker should close after selection")
	}
	if !strings.Contains(model.View(), "resumed 20260703T162900Z-a1b2") {
		t.Fatalf("resume selection result missing from view:\n%s", model.View())
	}
}

func TestResumePickerCancelsOnEsc(t *testing.T) {
	model := newSizedTestModel()
	model.SetResumePickerHandlers(
		func() ([]session.Meta, error) {
			return []session.Meta{
				{SessionID: "20260703T162408Z-f81a", Title: "hello"},
			}, nil
		},
		func(sessionID string) (string, error) {
			return "", nil
		},
	)

	model.input.SetValue("/resume")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.resumePicker == nil {
		t.Fatal("expected resume picker to open")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.resumePicker != nil {
		t.Fatal("expected resume picker to close on esc")
	}
	if strings.Contains(model.View(), "Resume Session") {
		t.Fatalf("resume picker should not remain in view:\n%s", model.View())
	}
}
