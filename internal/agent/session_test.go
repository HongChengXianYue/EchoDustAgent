package agent

import (
	"testing"

	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

func TestConversationMessagesExcludeSystemPrompt(t *testing.T) {
	agent := NewWithWorkspaceAndOptions(nil, tools.NewRegistry(), 4, "/tmp/project", DefaultOptions())
	agent.messages = append(agent.messages,
		llm.Message{Role: "user", Content: "hello"},
		llm.Message{Role: "system", Content: "Background delegate_task result.\nSubagent-1 task: inspect\nStatus: completed"},
		llm.Message{Role: "assistant", Content: "world"},
	)

	conversation := agent.ConversationMessages()
	if len(conversation) != 3 {
		t.Fatalf("ConversationMessages() len = %d", len(conversation))
	}
	if conversation[0].Role != "user" || conversation[1].Role != "user" || conversation[2].Role != "assistant" {
		t.Fatalf("ConversationMessages() = %#v", conversation)
	}
}

func TestRestoreConversationKeepsCurrentSystemPromptAndResetsTokens(t *testing.T) {
	agent := NewWithWorkspaceAndOptions(nil, tools.NewRegistry(), 4, "/tmp/project", DefaultOptions())
	originalSystem := agent.messages[0].Content
	agent.tokenUsage = tokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14, CachedTokens: 2}

	err := agent.RestoreConversation([]llm.Message{
		{Role: "user", Content: "restore me"},
		{Role: "system", Content: "Background delegate_task result.\nSubagent-1 task: inspect\nStatus: completed"},
		{Role: "assistant", Content: "done"},
	})
	if err != nil {
		t.Fatalf("RestoreConversation() error = %v", err)
	}
	if len(agent.messages) != 4 {
		t.Fatalf("messages len = %d", len(agent.messages))
	}
	if agent.messages[0].Role != "system" || agent.messages[0].Content != originalSystem {
		t.Fatalf("system prompt changed: %#v", agent.messages[0])
	}
	if agent.messages[2].Role != "user" {
		t.Fatalf("restored system message not sanitized: %#v", agent.messages[2])
	}
	if got := agent.TokenUsage(); got.TotalTokens != 0 || got.PromptTokens != 0 || got.CompletionTokens != 0 || got.CachedTokens != 0 {
		t.Fatalf("token usage not reset: %+v", got)
	}
}

func TestRestoreConversationSanitizesSystemMessages(t *testing.T) {
	agent := NewWithWorkspaceAndOptions(nil, tools.NewRegistry(), 4, "/tmp/project", DefaultOptions())
	if err := agent.RestoreConversation([]llm.Message{
		{Role: "system", Content: "bad"},
	}); err != nil {
		t.Fatalf("RestoreConversation() error = %v", err)
	}
	if got := agent.messages[1]; got.Role != "user" || got.Content != "bad" {
		t.Fatalf("restored message = %#v", got)
	}
}
