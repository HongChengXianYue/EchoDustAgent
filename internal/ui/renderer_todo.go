package ui

import (
	"fmt"
	"io"

	"local-agent/internal/runtimeevent"
)

func (r *BlockRenderer) writeTodoBlock(output io.Writer) {
	fmt.Fprintln(output, separatorLine(r.options.SeparatorWidth))
	fmt.Fprintln(output, "• Todo")
	if len(r.todos) == 0 {
		fmt.Fprintln(output, "  └ "+greenText("[ ] Waiting for todo list"))
		return
	}
	for _, item := range r.todos {
		fmt.Fprintf(output, "  └ %s\n", greenText(todoMarker(item.Status)+" "+item.Text))
	}
}

func (r *BlockRenderer) renderTodos(items []runtimeevent.TodoItem) {
	if len(items) == 0 {
		return
	}
	r.block("Todo", "")
	for _, item := range items {
		fmt.Fprintf(r.output, "  └ %s\n", greenText(todoMarker(item.Status)+" "+item.Text))
	}
}

func todoMarker(status runtimeevent.TodoStatus) string {
	switch status {
	case runtimeevent.TodoCompleted:
		return "[✓]"
	case runtimeevent.TodoInProgress:
		return "[>]"
	default:
		return "[ ]"
	}
}

func greenText(text string) string {
	return "\x1b[32m" + text + "\x1b[0m"
}
