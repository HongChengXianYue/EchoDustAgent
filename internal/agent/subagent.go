package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
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

const (
	subagentStartedSummary   = "subagent started"
	subagentCompletedSummary = "subagent completed"
)

type asyncSubagentTask struct {
	Index     int
	Task      string
	Args      json.RawMessage
	doneCh    chan struct{}
	done      bool
	collected bool
	result    tools.Result
}

func (t *delegateTaskTool) Name() string {
	return tools.DelegateTaskToolName
}

func (t *delegateTaskTool) Description() string {
	return "Start one independent read-only research subagent in the background. For broad analysis, call this tool multiple times in the same turn with focused tasks such as architecture, tools, UI, config, tests, or security. Continue independent parent work after delegating; the runtime will inject each subagent's final conclusion back before final synthesis."
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
	case <-ctx.Done():
		return tools.Error(ctx.Err().Error())
	default:
	}

	task := &asyncSubagentTask{
		Index:  index,
		Task:   params.Task,
		Args:   append(json.RawMessage(nil), args...),
		doneCh: make(chan struct{}),
	}
	a.registerSubagentTask(task)
	go a.executeSubagentTask(ctx, task, params)
	return tools.Success(
		subagentStartedSummary,
		fmt.Sprintf("Subagent-%d is running in background for task: %s", index, params.Task),
	)
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
	options.StepBudget = a.options.Subagents.StepBudget
	registry := a.subagentRegistry()
	prompt := subagentSystemPrompt(a.workspace, options.MaxParallelToolCalls)
	if suffix := strings.TrimSpace(options.SystemPromptSuffix); suffix != "" {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + suffix
	}
	subagent := newAgent(a.client, registry, a.options.Subagents.MaxSteps, a.workspace, prompt, options)
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

func (a *Agent) executeSubagentTask(ctx context.Context, task *asyncSubagentTask, params delegateTaskArgs) {
	result := a.runSubagentTask(ctx, task, params)
	a.finishSubagentTask(task, result)
}

func (a *Agent) runSubagentTask(ctx context.Context, task *asyncSubagentTask, params delegateTaskArgs) tools.Result {
	select {
	case a.subagentLimiter <- struct{}{}:
		defer func() { <-a.subagentLimiter }()
	case <-ctx.Done():
		return tools.Error(ctx.Err().Error())
	}

	subagent := a.newSubagent(params.Task, task.Index)
	answer, err := subagent.Run(ctx, subagentUserInput(params))
	if err != nil {
		return tools.Error("subagent failed: " + err.Error())
	}
	answer = truncateUTF8Bytes(strings.TrimSpace(answer), a.options.Subagents.ResultMaxBytes)
	if answer == "" {
		answer = "(subagent returned no final answer)"
	}
	return tools.Success(subagentCompletedSummary, answer)
}

func (a *Agent) subagentRegistry() *tools.Registry {
	registry := tools.NewRegistry()
	for _, name := range []string{"list_files", "find_files", "read_file", "read_file_range", "search_files", "find_symbol", "find_references", "find_callers", "find_callees", "git_status", "git_diff", "git_log", "run_command"} {
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
		"# Role",
		"You are a read-only research subagent.",
		"",
		"# Scope",
		"Use the provided tools to inspect the workspace and run safe read-only, search, or build/test commands when needed.",
		"Do not modify files, do not change git state, do not install dependencies, and do not run privileged or destructive commands.",
		"Do not spawn another subagent. The delegate_task tool is intentionally unavailable.",
		"",
		"# Tool Use",
		"An internal todo is initialized for your delegated task. Use update_todos only if you need to revise that plan, and keep at most one item in_progress.",
		fmt.Sprintf("Do not return more than %d non-planning tool calls in one assistant turn. update_todos and engineering_checklist are planning/guidance tools and do not count toward this limit. Multiple calls to the same non-planning tool with different arguments count separately.", maxParallelToolCalls),
		"",
		"# Final Answer",
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

func (a *Agent) registerSubagentTask(task *asyncSubagentTask) {
	if a == nil || task == nil {
		return
	}
	a.subagentMu.Lock()
	defer a.subagentMu.Unlock()
	if a.subagentTasks == nil {
		a.subagentTasks = map[int]*asyncSubagentTask{}
	}
	a.subagentTasks[task.Index] = task
}

func (a *Agent) finishSubagentTask(task *asyncSubagentTask, result tools.Result) {
	if a == nil || task == nil {
		return
	}
	a.subagentMu.Lock()
	stored := a.subagentTasks[task.Index]
	if stored == nil {
		stored = task
		a.subagentTasks[task.Index] = stored
	}
	stored.result = result
	if !stored.done {
		stored.done = true
		close(stored.doneCh)
	}
	args := append(json.RawMessage(nil), stored.Args...)
	index := stored.Index
	a.subagentMu.Unlock()

	a.emit(runtimeevent.Event{
		Type:          runtimeevent.TypeToolResult,
		Tool:          tools.DelegateTaskToolName,
		Category:      approval.CategoryReadOnly,
		Args:          args,
		Result:        &result,
		SubagentIndex: index,
	})
}

func (a *Agent) collectCompletedSubagentResults() {
	if a == nil {
		return
	}
	// Completed delegated work is surfaced back to the parent as synthetic
	// system messages so the next parent turn can reason over the conclusion
	// without blocking every delegate_task call synchronously.
	type completedSubagent struct {
		index  int
		task   string
		result tools.Result
	}
	completed := make([]completedSubagent, 0)

	a.subagentMu.Lock()
	for index, task := range a.subagentTasks {
		if task == nil || !task.done || task.collected {
			continue
		}
		task.collected = true
		completed = append(completed, completedSubagent{
			index:  index,
			task:   task.Task,
			result: task.result,
		})
	}
	a.subagentMu.Unlock()

	sort.Slice(completed, func(i, j int) bool { return completed[i].index < completed[j].index })
	for _, item := range completed {
		a.messages = append(a.messages, llm.Message{
			Role:    "system",
			Content: subagentCompletionMessage(item.index, item.task, item.result),
		})
	}
}

func (a *Agent) awaitOutstandingSubagents(ctx context.Context) (bool, error) {
	if a == nil {
		return false, nil
	}
	// The parent only joins delegated work when it is otherwise ready to stop.
	// This keeps delegate_task asynchronous during active work, while still
	// ensuring the final synthesis sees every delegated conclusion.
	pending := a.outstandingSubagentTasks()
	if len(pending) == 0 {
		return false, nil
	}
	for _, task := range pending {
		select {
		case <-task.doneCh:
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
	a.collectCompletedSubagentResults()
	return true, nil
}

func (a *Agent) outstandingSubagentTasks() []*asyncSubagentTask {
	a.subagentMu.Lock()
	defer a.subagentMu.Unlock()
	pending := make([]*asyncSubagentTask, 0, len(a.subagentTasks))
	for _, task := range a.subagentTasks {
		if task == nil || task.collected {
			continue
		}
		pending = append(pending, task)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Index < pending[j].Index })
	return pending
}

func subagentCompletionMessage(index int, task string, result tools.Result) string {
	task = strings.TrimSpace(task)
	if task == "" {
		task = fmt.Sprintf("Subagent-%d", index)
	}
	lines := []string{
		"Background delegate_task result.",
		fmt.Sprintf("Subagent-%d task: %s", index, task),
	}
	if result.Status == "error" {
		lines = append(lines, "Status: error")
		if summary := strings.TrimSpace(result.Summary); summary != "" {
			lines = append(lines, "Error: "+summary)
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Status: completed")
	if output := strings.TrimSpace(result.Output); output != "" {
		lines = append(lines, "Conclusion:\n"+output)
	} else if summary := strings.TrimSpace(result.Summary); summary != "" {
		lines = append(lines, "Conclusion:\n"+summary)
	}
	return strings.Join(lines, "\n")
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
		runtimeevent.TypeApprovalDecision,
		runtimeevent.TypeTokenUsage,
		runtimeevent.TypeStepBudgetExtend,
		runtimeevent.TypeStepBudgetStop:
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
