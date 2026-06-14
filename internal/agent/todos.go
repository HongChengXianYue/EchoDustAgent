package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

const updateTodosToolName = "update_todos"

type updateTodosArgs struct {
	Items []runtimeevent.TodoItem `json:"items"`
}

func todoFunctionTool() llm.FunctionTool {
	return llm.FunctionTool{
		Name:        updateTodosToolName,
		Description: "Create or update the current task todo list before and during concrete workspace work. Keep exactly one item in_progress when work is underway.",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"items":{
					"type":"array",
					"items":{
						"type":"object",
						"properties":{
							"text":{"type":"string"},
							"status":{"type":"string","enum":["pending","in_progress","completed"]}
						},
						"required":["text","status"],
						"additionalProperties":false
					}
				}
			},
			"required":["items"],
			"additionalProperties":false
		}`),
	}
}

func (a *Agent) executeTodoTool(step int, index int, call llm.ToolCall) executedToolCall {
	result := a.updateTodos(step, call.ArgumentsJSON())
	return executedToolCall{index: index, call: call, result: result}
}

func (a *Agent) updateTodos(step int, args json.RawMessage) tools.Result {
	items, err := parseTodoItems(args)
	if err != nil {
		return tools.Error(err.Error())
	}
	a.todos = items
	a.todosReady = true
	a.emit(runtimeevent.Event{
		Step:  step,
		Type:  runtimeevent.TypeTodoUpdate,
		Todos: append([]runtimeevent.TodoItem(nil), items...),
	})
	return tools.Success(fmt.Sprintf("updated %d todo item(s)", len(items)), "")
}

func parseTodoItems(args json.RawMessage) ([]runtimeevent.TodoItem, error) {
	var params updateTodosArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid todo arguments: %w", err)
	}
	if len(params.Items) == 0 {
		return nil, fmt.Errorf("items must contain at least one todo")
	}

	items := make([]runtimeevent.TodoItem, 0, len(params.Items))
	inProgress := 0
	for _, item := range params.Items {
		item.Text = strings.TrimSpace(item.Text)
		if item.Text == "" {
			return nil, fmt.Errorf("todo text must be non-empty")
		}
		switch item.Status {
		case runtimeevent.TodoPending, runtimeevent.TodoInProgress, runtimeevent.TodoCompleted:
		default:
			return nil, fmt.Errorf("invalid todo status %q", item.Status)
		}
		if item.Status == runtimeevent.TodoInProgress {
			inProgress++
		}
		items = append(items, item)
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("only one todo can be in_progress")
	}
	return items, nil
}
