package runtimeevent

import (
	"encoding/json"

	"local-agent/internal/approval"
	"local-agent/internal/tools"
)

type Type string

const (
	TypeRunStart         Type = "run_start"
	TypeRunEnd           Type = "run_end"
	TypeUserMessage      Type = "user_message"
	TypeAssistantDelta   Type = "assistant_delta"
	TypeAssistantMessage Type = "assistant_message"
	TypeTodoUpdate       Type = "todo_update"
	TypeToolCall         Type = "tool_call"
	TypeToolResult       Type = "tool_result"
	TypeFinal            Type = "final"
	TypeError            Type = "error"
	TypeApprovalRequest  Type = "approval_request"
	TypeApprovalDecision Type = "approval_decision"
	TypeContextPruned    Type = "context_pruned"
	TypeCompactionStart  Type = "compaction_started"
	TypeCompactionDone   Type = "compaction_done"
	TypeCompactionSkip   Type = "compaction_skipped"
	TypeTokenUsage       Type = "token_usage"
	TypeStepBudgetExtend Type = "step_budget_extended"
	TypeStepBudgetStop   Type = "step_budget_exhausted"
	TypeStepTiming       Type = "step_timing"
	TypeRunTiming        Type = "run_timing"
)

type TodoStatus = tools.TodoStatus

const (
	TodoPending    = tools.TodoPending
	TodoInProgress = tools.TodoInProgress
	TodoCompleted  = tools.TodoCompleted
)

type TodoItem = tools.TodoItem

type Handler interface {
	HandleEvent(Event)
}

type Event struct {
	Step       int                 `json:"step,omitempty"`
	Type       Type                `json:"type"`
	Message    string              `json:"message,omitempty"`
	Error      string              `json:"error,omitempty"`
	Tool       string              `json:"tool,omitempty"`
	Category   approval.Category   `json:"category,omitempty"`
	Args       json.RawMessage     `json:"args,omitempty"`
	Result     *tools.Result       `json:"result,omitempty"`
	DurationMS int64               `json:"duration_ms,omitempty"`
	Decision   string              `json:"decision,omitempty"`
	Decisions  []approval.Decision `json:"decisions,omitempty"`
	Reason     string              `json:"reason,omitempty"`
	Todos      []TodoItem          `json:"todos,omitempty"`
	ParentTool string              `json:"parent_tool,omitempty"`
	Source     string              `json:"source,omitempty"`
	Before     int                 `json:"before,omitempty"`
	After      int                 `json:"after,omitempty"`
	Count      int                 `json:"count,omitempty"`
	Delta      string              `json:"delta,omitempty"`
	// SubagentIndex is assigned by the parent agent for grouping forwarded
	// subagent events in the UI. It is scoped to one parent Run.
	SubagentIndex int `json:"subagent_index,omitempty"`

	// Token usage fields (TypeTokenUsage events).
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
	CumulativeTotal  int `json:"cumulative_total,omitempty"`
}
