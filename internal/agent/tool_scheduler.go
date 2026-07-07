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

	category      approval.Category
	writeImpact   approval.WriteImpact
	subagentIndex int
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
		} else if tools.IsEngineeringChecklistTool(call.Function.Name) {
			results[i] = a.executeEngineeringChecklistTool(ctx, step, i, call)
		}
	}

	loopApprovals := map[string]bool{}
	maxParallel := a.options.MaxParallelToolCalls
	if maxParallel <= 0 {
		maxParallel = DefaultOptions().MaxParallelToolCalls
	}
	acceptedToolCalls := 0
	plans := make([]preparedToolCall, 0, len(calls))
	for i, call := range calls {
		if tools.IsUpdateTodosTool(call.Function.Name) || tools.IsEngineeringChecklistTool(call.Function.Name) {
			continue
		}
		if acceptedToolCalls >= maxParallel {
			result := tools.Error(fmt.Sprintf("too many parallel tool calls: maximum is %d non-planning tool call(s) per assistant turn", maxParallel))
			a.emit(runtimeevent.Event{
				Step:   step,
				Type:   runtimeevent.TypeToolResult,
				Tool:   call.Function.Name,
				Args:   call.ArgumentsJSON(),
				Result: &result,
			})
			results[i] = executedToolCall{index: i, call: call, result: result}
			continue
		}
		acceptedToolCalls++
		plans = append(plans, a.prepareToolCall(ctx, step, i, call, loopApprovals))
	}

	locks := newTargetLocks()
	semaphore := make(chan struct{}, maxParallel)
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
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				result := tools.Error(ctx.Err().Error())
				a.emit(runtimeevent.Event{
					Step:     step,
					Type:     runtimeevent.TypeToolResult,
					Tool:     plan.call.Function.Name,
					Category: plan.category,
					Args:     plan.args,
					Result:   &result,
				})
				results[plan.index] = executedToolCall{index: plan.index, call: plan.call, result: result}
				return
			}
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
	if tools.IsDelegateTaskTool(call.Function.Name) {
		plan.subagentIndex = a.nextSubagentIndex()
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
		Step:          step,
		Type:          runtimeevent.TypeToolCall,
		Tool:          plan.call.Function.Name,
		Category:      plan.category,
		Args:          plan.args,
		SubagentIndex: plan.subagentIndex,
	})

	startedAt := time.Now()
	var result tools.Result
	var err error
	if tools.IsDelegateTaskTool(plan.call.Function.Name) && a.subagentTool != nil {
		result = a.runSubagentWithIndex(ctx, plan.args, plan.subagentIndex)
	} else {
		result, err = plan.tool.Execute(ctx, plan.args)
	}
	if err != nil {
		result = tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	a.emit(runtimeevent.Event{
		Step:          step,
		Type:          runtimeevent.TypeToolResult,
		Tool:          plan.call.Function.Name,
		Category:      plan.category,
		Args:          plan.args,
		Result:        &result,
		DurationMS:    time.Since(startedAt).Milliseconds(),
		SubagentIndex: plan.subagentIndex,
	})
	return executedToolCall{index: plan.index, call: plan.call, result: result}
}
