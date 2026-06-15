package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestUpdateTodosToolValidatesInput(t *testing.T) {
	tests := []struct {
		name    string
		args    json.RawMessage
		wantErr string
	}{
		{name: "empty", args: json.RawMessage(`{"items":[]}`), wantErr: "at least one"},
		{name: "blank text", args: json.RawMessage(`{"items":[{"text":" ","status":"pending"}]}`), wantErr: "non-empty"},
		{name: "invalid status", args: json.RawMessage(`{"items":[{"text":"Read","status":"started"}]}`), wantErr: "invalid todo status"},
		{name: "multiple in progress", args: json.RawMessage(`{"items":[{"text":"Read","status":"in_progress"},{"text":"Write","status":"in_progress"}]}`), wantErr: "only one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewUpdateTodosTool()
			result, err := tool.Execute(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("Execute() unexpected error = %v", err)
			}
			if result.Status != "error" || !strings.Contains(result.Summary, tt.wantErr) {
				t.Fatalf("Execute() result = %#v, want error containing %q", result, tt.wantErr)
			}
		})
	}
}

func TestUpdateTodosToolStoresCurrentItems(t *testing.T) {
	tool := NewUpdateTodosTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"items":[{"text":" Read ","status":"in_progress"},{"text":"Write","status":"pending"}]}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("Execute() result = %#v, want success", result)
	}
	if !tool.Ready() {
		t.Fatalf("tool should be ready after successful update")
	}
	items := tool.Items()
	if len(items) != 2 || items[0].Text != "Read" || items[0].Status != TodoInProgress {
		t.Fatalf("items = %#v", items)
	}

	tool.Reset()
	if tool.Ready() || len(tool.Items()) != 0 {
		t.Fatalf("tool should reset todo state")
	}
}
