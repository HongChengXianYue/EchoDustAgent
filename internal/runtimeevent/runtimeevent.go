package runtimeevent

import (
	"encoding/json"

	"local-agent/internal/tools"
)

type Type string

const (
	TypeAssistantMessage Type = "assistant_message"
	TypeToolCall         Type = "tool_call"
	TypeToolResult       Type = "tool_result"
	TypeFinal            Type = "final"
	TypeError            Type = "error"
	TypeApprovalRequest  Type = "approval_request"
	TypeApprovalDecision Type = "approval_decision"
)

type Handler interface {
	HandleEvent(Event)
}

type Event struct {
	Step       int             `json:"step,omitempty"`
	Type       Type            `json:"type"`
	Message    string          `json:"message,omitempty"`
	Error      string          `json:"error,omitempty"`
	Tool       string          `json:"tool,omitempty"`
	Category   tools.Category  `json:"category,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Result     *tools.Result   `json:"result,omitempty"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	Decision   string          `json:"decision,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}
