package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const UpdateTodosToolName = "update_todos"
const DelegateTaskToolName = "delegate_task"

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type TodoItem struct {
	Text   string     `json:"text"`
	Status TodoStatus `json:"status"`
}

type UpdateTodosTool struct {
	mu    sync.Mutex
	items []TodoItem
	ready bool
}

type updateTodosArgs struct {
	Items []TodoItem `json:"items"`
}

func NewUpdateTodosTool() *UpdateTodosTool {
	return &UpdateTodosTool{}
}

func IsUpdateTodosTool(name string) bool {
	return name == UpdateTodosToolName
}

func IsDelegateTaskTool(name string) bool {
	return name == DelegateTaskToolName
}

func (t *UpdateTodosTool) Name() string {
	return UpdateTodosToolName
}

func (t *UpdateTodosTool) Description() string {
	return "Create or update the current task todo list before and during concrete workspace work. Keep exactly one item in_progress when work is underway."
}

func (t *UpdateTodosTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
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
	}`)
}

func (t *UpdateTodosTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	items, err := parseTodoItems(args)
	if err != nil {
		return Error(err.Error()), nil
	}

	t.mu.Lock()
	t.items = items
	t.ready = true
	t.mu.Unlock()

	return Success(fmt.Sprintf("updated %d todo item(s)", len(items)), ""), nil
}

func (t *UpdateTodosTool) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.items = nil
	t.ready = false
}

func (t *UpdateTodosTool) Ready() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

func (t *UpdateTodosTool) Items() []TodoItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TodoItem(nil), t.items...)
}

func parseTodoItems(args json.RawMessage) ([]TodoItem, error) {
	var params updateTodosArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("invalid todo arguments: %w", err)
	}
	if len(params.Items) == 0 {
		return nil, fmt.Errorf("items must contain at least one todo")
	}

	items := make([]TodoItem, 0, len(params.Items))
	inProgress := 0
	for _, item := range params.Items {
		item.Text = strings.TrimSpace(item.Text)
		if item.Text == "" {
			return nil, fmt.Errorf("todo text must be non-empty")
		}
		switch item.Status {
		case TodoPending, TodoInProgress, TodoCompleted:
		default:
			return nil, fmt.Errorf("invalid todo status %q", item.Status)
		}
		if item.Status == TodoInProgress {
			inProgress++
		}
		items = append(items, item)
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("only one todo can be in_progress")
	}
	return items, nil
}
