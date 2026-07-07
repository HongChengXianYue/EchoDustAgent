package agent

import (
	"context"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func (a *Agent) executeTodoTool(ctx context.Context, step int, index int, call llm.ToolCall) executedToolCall {
	args := call.ArgumentsJSON()
	result, err := a.todoTool.Execute(ctx, args)
	if err != nil {
		result = tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	if result.Status == "success" {
		a.emit(runtimeevent.Event{
			Step:  step,
			Type:  runtimeevent.TypeTodoUpdate,
			Todos: a.todoTool.Items(),
		})
	} else {
		a.emit(runtimeevent.Event{
			Step:   step,
			Type:   runtimeevent.TypeToolResult,
			Tool:   call.Function.Name,
			Args:   args,
			Result: &result,
		})
	}
	return executedToolCall{index: index, call: call, result: result}
}

func (a *Agent) pruneTransientToolHistory() {
	transientToolIDs := map[string]bool{}
	messages := make([]llm.Message, 0, len(a.messages))
	for _, message := range a.messages {
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			toolCalls := make([]llm.ToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				if tools.IsUpdateTodosTool(call.Function.Name) || tools.IsEngineeringChecklistTool(call.Function.Name) {
					if call.ID != "" {
						transientToolIDs[call.ID] = true
					}
					continue
				}
				toolCalls = append(toolCalls, call)
			}
			message.ToolCalls = toolCalls
			if strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0 {
				continue
			}
		}
		if message.Role == "tool" && transientToolIDs[message.ToolCallID] {
			continue
		}
		messages = append(messages, message)
	}
	a.messages = messages
}
