package tools

import (
	"context"
	"strings"
	"testing"
)

func TestEngineeringChecklistToolGeneratesTypedChecklist(t *testing.T) {
	tool := NewEngineeringChecklistTool()
	result, err := tool.Execute(context.Background(), []byte(`{
		"task":"add accept-all approval mode",
		"change_type":"ui_interaction",
		"known_entrypoints":["Shift+Tab","BubbleApprover"],
		"expected_behavior":"mode toggles consistently and future approvals are auto-accepted",
		"risk_areas":["approval prompt","layout budget"]
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %q, want success: %#v", result.Status, result)
	}
	for _, want := range []string{
		"real entrypoint",
		"layout height/width budget",
		"Known entrypoints to verify: Shift+Tab, BubbleApprover",
		"Risk areas to cover: approval prompt, layout budget",
		"mode toggles consistently",
	} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("checklist missing %q:\n%s", want, result.Output)
		}
	}
}

func TestEngineeringChecklistToolValidatesRequiredFields(t *testing.T) {
	tool := NewEngineeringChecklistTool()
	result, err := tool.Execute(context.Background(), []byte(`{"task":"x","change_type":"bugfix"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "error" || !strings.Contains(result.Summary, "expected_behavior") {
		t.Fatalf("result = %#v, want expected_behavior error", result)
	}
}
