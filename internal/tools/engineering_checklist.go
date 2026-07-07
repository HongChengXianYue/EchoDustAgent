package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type EngineeringChecklistTool struct{}

type engineeringChecklistArgs struct {
	Task             string   `json:"task"`
	ChangeType       string   `json:"change_type"`
	KnownEntrypoints []string `json:"known_entrypoints,omitempty"`
	ExpectedBehavior string   `json:"expected_behavior"`
	RiskAreas        []string `json:"risk_areas,omitempty"`
}

func NewEngineeringChecklistTool() *EngineeringChecklistTool {
	return &EngineeringChecklistTool{}
}

func (t *EngineeringChecklistTool) Name() string {
	return EngineeringChecklistToolName
}

func (t *EngineeringChecklistTool) Description() string {
	return "Generate a concise engineering checklist before non-trivial code changes. Use it to model entrypoints, state, outputs, side effects, resource budgets, and verification before editing."
}

func (t *EngineeringChecklistTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"task":{"type":"string","description":"The user-facing task or bug being implemented."},
			"change_type":{"type":"string","enum":["bugfix","feature","ui_interaction","api_behavior","config_or_installation","tooling_or_io","refactor","test_only","general"]},
			"known_entrypoints":{"type":"array","items":{"type":"string"},"description":"Current entrypoints, handlers, commands, shortcuts, APIs, config keys, or files already identified."},
			"expected_behavior":{"type":"string","description":"The concrete behavior that should be true when the work is complete."},
			"risk_areas":{"type":"array","items":{"type":"string"},"description":"States, callers, resources, compatibility paths, or edge cases likely to be affected."}
		},
		"required":["task","change_type","expected_behavior"],
		"additionalProperties":false
	}`)
}

func (t *EngineeringChecklistTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params engineeringChecklistArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return Error(fmt.Sprintf("invalid engineering checklist arguments: %v", err)), nil
	}
	params.Task = strings.TrimSpace(params.Task)
	params.ChangeType = strings.TrimSpace(params.ChangeType)
	params.ExpectedBehavior = strings.TrimSpace(params.ExpectedBehavior)
	if params.Task == "" {
		return Error("task must be non-empty"), nil
	}
	if params.ExpectedBehavior == "" {
		return Error("expected_behavior must be non-empty"), nil
	}

	output := renderEngineeringChecklist(params)
	return Success("engineering checklist generated", output), nil
}

func renderEngineeringChecklist(params engineeringChecklistArgs) string {
	lines := []string{
		"Engineering checklist:",
		"1. Restate the expected user-visible behavior and preserve unrelated behavior.",
		"2. Identify the real entrypoint and follow the call path to the state owner, decision logic, output path, and side effects.",
		"3. Check all relevant states, callers, execution modes, and compatibility paths rather than only the easiest path.",
		"4. If the change consumes a shared resource, update the corresponding accounting or lifecycle.",
		"5. Add or update tests for the real entrypoint, externally visible result, key state transitions, side effects, and failure paths.",
		"6. Verify with targeted tests first, then broader tests when behavior touches shared agent, tool, UI, or config code.",
	}

	switch params.ChangeType {
	case "ui_interaction":
		lines = append(lines,
			"UI-specific checks: input routing, active modal/state interception, render location, layout height/width budget, visible affordance, and keyboard/mouse variants.",
		)
	case "api_behavior":
		lines = append(lines,
			"API-specific checks: public contract, request/response shape, error semantics, compatibility with existing callers, and tests at the API boundary.",
		)
	case "config_or_installation":
		lines = append(lines,
			"Config/install checks: active config source, read/write path match, default vs override behavior, existing file merge semantics, and reload or startup verification.",
		)
	case "tooling_or_io":
		lines = append(lines,
			"Tooling/I/O checks: workspace boundaries, approval/risk classification, file-change metadata, command timeout/output limits, and rollback or partial-write behavior.",
		)
	case "refactor":
		lines = append(lines,
			"Refactor checks: behavior-preserving baseline, caller impact, public API compatibility, and tests that prove unchanged semantics.",
		)
	}

	if len(params.KnownEntrypoints) > 0 {
		lines = append(lines, "Known entrypoints to verify: "+strings.Join(cleanStringList(params.KnownEntrypoints), ", "))
	}
	if len(params.RiskAreas) > 0 {
		lines = append(lines, "Risk areas to cover: "+strings.Join(cleanStringList(params.RiskAreas), ", "))
	}
	lines = append(lines,
		"Expected behavior: "+params.ExpectedBehavior,
		"Completion bar: do not report completion until current-state evidence proves each relevant checklist item or you explicitly state the verification gap.",
	)
	return strings.Join(lines, "\n")
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
