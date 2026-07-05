package tui

import (
	"strings"
	"testing"

	"local-agent/internal/runtimeevent"
)

func TestSanitizeAssistantTextStripsThinkSections(t *testing.T) {
	text := "<think>\ninternal plan\n</think>\nVisible answer\n</think>"

	got := sanitizeAssistantText(text)
	if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
		t.Fatalf("think tags should be removed, got %q", got)
	}
	if !strings.Contains(got, "Visible answer") {
		t.Fatalf("visible answer should remain, got %q", got)
	}
}

func TestStripTodoEchoLinesRemovesChecklistDuplicates(t *testing.T) {
	text := "Working on it.\n- [x] Add selectionMode field to Model\n- [x] Create internal/tui/selection_mode.go\n"
	todos := []runtimeevent.TodoItem{
		{Text: "Add selectionMode field to Model", Status: runtimeevent.TodoCompleted},
		{Text: "Create internal/tui/selection_mode.go", Status: runtimeevent.TodoCompleted},
	}

	got := stripTodoEchoLines(text, todos)
	if strings.Contains(got, "selectionMode field") || strings.Contains(got, "selection_mode.go") {
		t.Fatalf("todo checklist echo should be removed, got %q", got)
	}
	if got != "Working on it." {
		t.Fatalf("non-todo prose should remain, got %q", got)
	}
}
