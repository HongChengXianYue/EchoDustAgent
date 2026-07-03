package llm

import (
	"bytes"
	"context"
	"encoding/json"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (t ToolCall) ArgumentsJSON() json.RawMessage {
	raw := bytes.TrimSpace([]byte(t.Function.Arguments))
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(raw)
}

type FunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
	Usage     *TokenUsage
}

type StreamDelta struct {
	Content   string
	ToolCalls []ToolCall
	Usage     *TokenUsage
	Done      bool
}

type StreamHandler func(StreamDelta) error

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
}

type Client interface {
	ChatWithTools(ctx context.Context, messages []Message, tools []FunctionTool) (*ChatResponse, error)
}

type StreamingClient interface {
	Client
	ChatWithToolsStream(ctx context.Context, messages []Message, tools []FunctionTool, onDelta StreamHandler) (*ChatResponse, error)
}
