package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

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
		if tools.IsUpdateTodosTool(call.Function.Name) {
			results[i] = a.executeTodoTool(ctx, step, i, call)
		}
	}

	loopApprovals := map[string]bool{}
	plans := make([]preparedToolCall, 0, len(calls))
	for i, call := range calls {
		if tools.IsUpdateTodosTool(call.Function.Name) {
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
	tool, ok := a.lookupTool(call.Function.Name)
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
	if !a.todoTool.Ready() {
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
