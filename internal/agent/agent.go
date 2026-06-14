package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type Agent struct {
	client     llm.Client
	registry   *tools.Registry
	messages   []llm.Message
	maxSteps   int
	workspace  string
	renderer   runtimeevent.Handler
	approver   approval.Approver
	emitMu     sync.Mutex
	todos      []runtimeevent.TodoItem
	todosReady bool
}

func New(client llm.Client, registry *tools.Registry, maxSteps int) *Agent {
	return NewWithWorkspace(client, registry, maxSteps, "")
}

func NewWithWorkspace(client llm.Client, registry *tools.Registry, maxSteps int, workspace string) *Agent {
	if maxSteps <= 0 {
		maxSteps = 10
	}
	return &Agent{
		client:    client,
		registry:  registry,
		maxSteps:  maxSteps,
		workspace: strings.TrimSpace(workspace),
		messages: []llm.Message{
			{
				Role:    "system",
				Content: systemPrompt(workspace),
			},
		},
	}
}

func systemPrompt(workspace string) string {
	lines := []string{
		"You are a local coding agent.",
		"Use the provided function tools when you need workspace information or need to modify files.",
		"For concrete workspace tasks, call update_todos before any workspace tool. Keep the todo list current: mark one item in_progress, mark completed items as completed, then move the next item to in_progress.",
		"You may return multiple tool calls in one assistant turn when the calls are independent.",
		"Use workspace-relative paths for file tools unless the user explicitly asks for an absolute path.",
		"Run commands in the configured workspace. Do not cd into guessed absolute paths.",
		"When locating a file or directory, or checking whether a path exists anywhere under the workspace, use find_files first. Use list_files only when the user asks to inspect one specific directory level.",
		"Treat requests phrased as under the current directory or under the workspace as recursive unless the user explicitly asks for only top-level or direct children.",
		"If find_files returns candidates, read or list the matching paths before making claims that require their contents or immediate children.",
		"If find_files has no path matches and the user may be looking for text inside files, then use search_files.",
		"Do not inspect the workspace for greetings, small talk, thanks, or general capability questions.",
		"Only call tools when the user asks for a concrete workspace action such as reading, listing, searching, editing files, or running commands.",
		"Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.",
		"When responding in the terminal, keep final answers concise. Markdown is allowed for final summaries when it improves readability, but avoid decorative emoji and excessive detail.",
		"When summarizing recent code changes, prefer git log/stat or the worklog first; do not run a full diff unless the user asks for exact diff details.",
		"When you are done, answer directly and concisely.",
	}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		lines = append(lines[:1], append([]string{"Current workspace: " + workspace}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) SetRenderer(renderer runtimeevent.Handler) {
	a.renderer = renderer
}

func (a *Agent) SetApprover(approver approval.Approver) {
	a.approver = approver
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	a.todos = nil
	a.todosReady = false
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.client.ChatWithTools(ctx, a.messages, a.functionTools())
		if err != nil {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeError, Error: err.Error()})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			return "", err
		}
		assistantMessage := llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.messages = append(a.messages, assistantMessage)

		if len(resp.ToolCalls) == 0 {
			final := strings.TrimSpace(resp.Content)
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeFinal, Message: final})
			return final, nil
		}
		if strings.TrimSpace(resp.Content) != "" {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeAssistantMessage, Message: strings.TrimSpace(resp.Content)})
		}
		for _, executed := range a.executeToolCalls(ctx, step, resp.ToolCalls) {
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: executed.call.ID,
				Content:    executed.result.JSON(),
			})
		}
	}
	err := fmt.Errorf("agent stopped after %d steps without a final response", a.maxSteps)
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeError, Error: err.Error()})
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	return "", err
}

func (a *Agent) Messages() []llm.Message {
	out := make([]llm.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

type preparedToolCall struct {
	index  int
	call   llm.ToolCall
	tool   tools.Tool
	args   json.RawMessage
	result *tools.Result

	category    approval.Category
	writeImpact approval.WriteImpact
}

type executedToolCall struct {
	index  int
	call   llm.ToolCall
	result tools.Result
}

func (a *Agent) executeToolCalls(ctx context.Context, step int, calls []llm.ToolCall) []executedToolCall {
	results := make([]executedToolCall, len(calls))
	for i, call := range calls {
		if call.Function.Name == updateTodosToolName {
			results[i] = a.executeTodoTool(step, i, call)
		}
	}

	loopApprovals := map[string]bool{}
	plans := make([]preparedToolCall, 0, len(calls))
	for i, call := range calls {
		if call.Function.Name == updateTodosToolName {
			continue
		}
		plans = append(plans, a.prepareToolCall(ctx, step, i, call, loopApprovals))
	}

	locks := newTargetLocks()
	var wg sync.WaitGroup
	for _, plan := range plans {
		plan := plan
		if plan.result != nil {
			results[plan.index] = executedToolCall{index: plan.index, call: plan.call, result: *plan.result}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := locks.lock(plan.writeImpact.Targets)
			defer unlock()
			results[plan.index] = a.executePreparedTool(ctx, step, plan)
		}()
	}
	wg.Wait()
	return results
}

func (a *Agent) prepareToolCall(ctx context.Context, step int, index int, call llm.ToolCall, loopApprovals map[string]bool) preparedToolCall {
	plan := preparedToolCall{index: index, call: call, args: call.ArgumentsJSON()}
	tool, ok := a.registry.Get(call.Function.Name)
	if !ok {
		result := tools.Error(fmt.Sprintf("unknown tool %q", call.Function.Name))
		a.emit(runtimeevent.Event{
			Step:   step,
			Type:   runtimeevent.TypeToolResult,
			Tool:   call.Function.Name,
			Args:   call.ArgumentsJSON(),
			Result: &result,
		})
		plan.result = &result
		return plan
	}
	plan.tool = tool
	plan.category = approval.Classify(call.Function.Name, plan.args)
	if !a.todosReady {
		result := tools.Error("tool execution requires a todo list; call update_todos before workspace tools")
		a.emit(runtimeevent.Event{
			Step:     step,
			Type:     runtimeevent.TypeToolResult,
			Tool:     call.Function.Name,
			Category: plan.category,
			Args:     plan.args,
			Result:   &result,
		})
		plan.result = &result
		return plan
	}
	plan.writeImpact = approval.AnalyzeWrite(call.Function.Name, plan.args, a.workspace, plan.category)
	if reason, blocked := approval.BlockReason(call.Function.Name, plan.args); blocked {
		result := tools.Error("command blocked by permanent safety policy: " + reason)
		a.emit(runtimeevent.Event{
			Step:     step,
			Type:     runtimeevent.TypeToolResult,
			Tool:     call.Function.Name,
			Category: plan.category,
			Args:     plan.args,
			Result:   &result,
		})
		plan.result = &result
		return plan
	}
	if !a.approveTool(ctx, step, call.Function.Name, plan.category, plan.args, plan.writeImpact, loopApprovals) {
		result := tools.Error("tool execution denied by approval policy")
		a.emit(runtimeevent.Event{
			Step:     step,
			Type:     runtimeevent.TypeToolResult,
			Tool:     call.Function.Name,
			Category: plan.category,
			Args:     plan.args,
			Result:   &result,
		})
		plan.result = &result
		return plan
	}
	return plan
}

func (a *Agent) executePreparedTool(ctx context.Context, step int, plan preparedToolCall) executedToolCall {
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeToolCall,
		Tool:     plan.call.Function.Name,
		Category: plan.category,
		Args:     plan.args,
	})

	startedAt := time.Now()
	result, err := plan.tool.Execute(ctx, plan.args)
	if err != nil {
		result = tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	a.emit(runtimeevent.Event{
		Step:       step,
		Type:       runtimeevent.TypeToolResult,
		Tool:       plan.call.Function.Name,
		Category:   plan.category,
		Args:       plan.args,
		Result:     &result,
		DurationMS: time.Since(startedAt).Milliseconds(),
	})
	return executedToolCall{index: plan.index, call: plan.call, result: result}
}

func (a *Agent) approveTool(ctx context.Context, step int, tool string, category approval.Category, args json.RawMessage, impact approval.WriteImpact, loopApprovals map[string]bool) bool {
	if !approval.RequiresApproval(category) && !impact.Writes {
		return true
	}
	if a.approver == nil {
		return true
	}
	request := approvalRequest(tool, category, args, impact)
	key := approval.CacheKey(request)
	if request.Scope == approval.ScopeLoop && loopApprovals[key] {
		return true
	}
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeApprovalRequest,
		Tool:     tool,
		Category: category,
		Args:     args,
		Reason:   request.Reason,
	})
	decision := a.approver.Approve(ctx, request)
	if request.Scope == approval.ScopeLoop && decision == approval.DecisionAlways {
		loopApprovals[key] = true
	}
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeApprovalDecision,
		Tool:     tool,
		Category: category,
		Args:     args,
		Decision: string(decision),
		Reason:   request.Reason,
	})
	return decision == approval.DecisionAllow || decision == approval.DecisionAlways
}

func approvalRequest(tool string, category approval.Category, args json.RawMessage, impact approval.WriteImpact) approval.Request {
	request := approval.Request{
		Tool:     tool,
		Category: category,
		Args:     args,
		Reason:   "tool execution requested",
		Scope:    approval.ScopeSession,
	}
	switch {
	case impact.Writes && impact.External:
		request.Reason = "external write requested"
		request.Scope = approval.ScopeLoop
		request.Key = approval.ExternalWriteApprovalKey()
		request.Options = []approval.Decision{approval.DecisionAllow, approval.DecisionAlways, approval.DecisionDeny}
	case impact.Writes:
		request.Reason = "workspace write requested"
		request.Scope = approval.ScopeSession
		request.Key = approval.WorkspaceWriteApprovalKey()
		request.Options = []approval.Decision{approval.DecisionAlways, approval.DecisionDeny}
	}
	return request
}

type targetLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newTargetLocks() *targetLocks {
	return &targetLocks{locks: map[string]*sync.Mutex{}}
}

func (l *targetLocks) lock(targets []string) func() {
	if len(targets) == 0 {
		return func() {}
	}
	targets = append([]string(nil), targets...)
	sort.Strings(targets)
	held := make([]*sync.Mutex, 0, len(targets))
	for _, target := range targets {
		lock := l.lockFor(target)
		lock.Lock()
		held = append(held, lock)
	}
	return func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Unlock()
		}
	}
}

func (l *targetLocks) lockFor(target string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	lock := l.locks[target]
	if lock == nil {
		lock = &sync.Mutex{}
		l.locks[target] = lock
	}
	return lock
}

func (a *Agent) emit(event runtimeevent.Event) {
	a.emitMu.Lock()
	defer a.emitMu.Unlock()
	if a.renderer != nil {
		a.renderer.HandleEvent(event)
	}
}

func (a *Agent) functionTools() []llm.FunctionTool {
	specs := a.registry.Specs()
	out := make([]llm.FunctionTool, 0, len(specs)+1)
	out = append(out, todoFunctionTool())
	for _, spec := range specs {
		out = append(out, llm.FunctionTool{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  spec.Parameters,
		})
	}
	return out
}
