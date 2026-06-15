package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type delegateTaskTool struct {
	agent *Agent
}

type delegateTaskArgs struct {
	Task           string `json:"task"`
	ExpectedOutput string `json:"expected_output,omitempty"`
}

func (t *delegateTaskTool) Name() string {
	return tools.DelegateTaskToolName
}

func (t *delegateTaskTool) Description() string {
	return "Delegate one independent read-only research task to an isolated subagent. For broad analysis, call this tool multiple times in the same turn with focused tasks such as architecture, tools, UI, config, tests, or security. The subagent returns only its final conclusion."
}

func (t *delegateTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"task":{"type":"string"},
			"expected_output":{"type":"string"}
		},
		"required":["task"],
		"additionalProperties":false
	}`)
}

func (t *delegateTaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if t == nil || t.agent == nil {
		return tools.Error("subagent is not configured"), nil
	}
	return t.agent.runSubagent(ctx, args), nil
}

func (a *Agent) runSubagent(ctx context.Context, args json.RawMessage) tools.Result {
	return a.runSubagentWithIndex(ctx, args, a.nextSubagentIndex())
}

func (a *Agent) runSubagentWithIndex(ctx context.Context, args json.RawMessage, index int) tools.Result {
	params, err := parseDelegateTaskArgs(args)
	if err != nil {
		return tools.Error(err.Error())
	}
	if a.subagentLimiter == nil {
		return tools.Error("subagent is disabled")
	}

	select {
	case a.subagentLimiter <- struct{}{}:
		defer func() { <-a.subagentLimiter }()
	case <-ctx.Done():
		return tools.Error(ctx.Err().Error())
	}

	subagent := a.newSubagent(params.Task, index)
	answer, err := subagent.Run(ctx, subagentUserInput(params))
	if err != nil {
		return tools.Error("subagent failed: " + err.Error())
	}
	answer = truncateUTF8Bytes(strings.TrimSpace(answer), a.options.Subagents.ResultMaxBytes)
	if answer == "" {
		answer = "(subagent returned no final answer)"
	}
	return tools.Success("subagent completed", answer)
}

func parseDelegateTaskArgs(args json.RawMessage) (delegateTaskArgs, error) {
	var params delegateTaskArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return delegateTaskArgs{}, fmt.Errorf("invalid delegate_task arguments: %w", err)
	}
	params.Task = strings.TrimSpace(params.Task)
	params.ExpectedOutput = strings.TrimSpace(params.ExpectedOutput)
	if params.Task == "" {
		return delegateTaskArgs{}, fmt.Errorf("task must be non-empty")
	}
	return params, nil
}

func (a *Agent) newSubagent(task string, index int) *Agent {
	options := a.options
	options.Subagents.Enabled = false
	registry := a.subagentRegistry()
	subagent := newAgent(a.client, registry, a.options.Subagents.MaxSteps, a.workspace, subagentSystemPrompt(a.workspace, options.MaxParallelToolCalls), options)
	subagent.autoTodoText = subagentAutoTodoText(task)
	subagent.SetApprover(denyAllApprover{})
	if a.renderer != nil {
		subagent.SetRenderer(subagentEventForwarder{
			parent: a,
			task:   task,
			index:  index,
		})
	}
	return subagent
}

func (a *Agent) subagentRegistry() *tools.Registry {
	registry := tools.NewRegistry()
	for _, name := range []string{"list_files", "find_files", "read_file", "search_files", "run_command"} {
		if tool, ok := a.registry.Get(name); ok {
			registry.Register(tool)
		}
	}
	return registry
}

func subagentUserInput(params delegateTaskArgs) string {
	if params.ExpectedOutput == "" {
		return params.Task
	}
	return "Task:\n" + params.Task + "\n\nExpected output:\n" + params.ExpectedOutput
}

func subagentSystemPrompt(workspace string, maxParallelToolCalls int) string {
	if maxParallelToolCalls <= 0 {
		maxParallelToolCalls = DefaultOptions().MaxParallelToolCalls
	}
	lines := []string{
		"You are a read-only research subagent.",
		"Use the provided tools to inspect the workspace and run safe read-only, search, or build/test commands when needed.",
		"An internal todo is initialized for your delegated task. Use update_todos only if you need to revise that plan, and keep at most one item in_progress.",
		fmt.Sprintf("Do not return more than %d non-update_todos tool calls in one assistant turn. Multiple calls to the same tool with different arguments count separately.", maxParallelToolCalls),
		"Do not modify files, do not change git state, do not install dependencies, and do not run privileged or destructive commands.",
		"Do not spawn another subagent. The delegate_task tool is intentionally unavailable.",
		"Return only your final conclusion with concise evidence such as relevant paths, symbols, or command results.",
		"Do not include your full message history or raw tool trace in the final answer.",
	}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		lines = append(lines[:1], append([]string{"Current workspace: " + workspace}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func subagentAutoTodoText(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return "Research delegated task"
	}
	return "Research: " + task
}

type denyAllApprover struct{}

func (denyAllApprover) Approve(ctx context.Context, request approval.Request) approval.Decision {
	return approval.DecisionDeny
}

type subagentEventForwarder struct {
	parent *Agent
	task   string
	index  int
}

func (f subagentEventForwarder) HandleEvent(event runtimeevent.Event) {
	if f.parent == nil {
		return
	}
	switch event.Type {
	case runtimeevent.TypeAssistantMessage,
		runtimeevent.TypeToolCall,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeError,
		runtimeevent.TypeApprovalRequest,
		runtimeevent.TypeApprovalDecision:
	default:
		return
	}
	event.Source = "subagent"
	event.ParentTool = truncateUTF8Bytes(f.task, 120)
	event.SubagentIndex = f.index
	f.parent.emit(event)
}

func truncateUTF8Bytes(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	used := 0
	var builder strings.Builder
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		if r == utf8.RuneError && size == 0 {
			break
		}
		if used+size > limit {
			break
		}
		builder.WriteRune(r)
		used += size
		text = text[size:]
	}
	return strings.TrimSpace(builder.String()) + "\n… truncated"
}
