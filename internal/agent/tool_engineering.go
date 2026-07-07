package agent

import (
	"context"

	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func (a *Agent) executeEngineeringChecklistTool(ctx context.Context, step int, index int, call llm.ToolCall) executedToolCall {
	args := call.ArgumentsJSON()
	result, err := a.checklistTool.Execute(ctx, args)
	if err != nil {
		result = tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	a.emit(runtimeevent.Event{
		Step:   step,
		Type:   runtimeevent.TypeToolResult,
		Tool:   call.Function.Name,
		Args:   args,
		Result: &result,
	})
	return executedToolCall{index: index, call: call, result: result}
}
