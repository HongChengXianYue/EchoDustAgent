package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

func TestPruneStaleToolResultsTrimsOldOutputAndKeepsPairing(t *testing.T) {
	agent := New(&fakeClient{}, tools.NewRegistry(), 3)
	agent.options.Context.PruneToolResultMaxBytes = 8
	agent.options.Context.PruneKeepRecentMessages = 1
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_read", "read_file", `{"path":"big.txt"}`)}},
		{Role: "tool", ToolCallID: "call_read", Content: tools.Success("read", strings.Repeat("x", 40)).JSON()},
		{Role: "user", Content: "newer"},
	}

	stats := agent.pruneStaleToolResults()
	if stats.Results != 1 {
		t.Fatalf("pruned results = %d, want 1", stats.Results)
	}
	if len(agent.messages[1].ToolCalls) != 1 || agent.messages[1].ToolCalls[0].ID != "call_read" {
		t.Fatalf("assistant tool call pairing changed: %#v", agent.messages[1])
	}
	if agent.messages[2].Role != "tool" || agent.messages[2].ToolCallID != "call_read" {
		t.Fatalf("tool message pairing changed: %#v", agent.messages[2])
	}
	if !strings.Contains(agent.messages[2].Content, prunedToolOutputPrefix) {
		t.Fatalf("tool result was not pruned: %s", agent.messages[2].Content)
	}
}

func TestPruneStaleToolResultsProtectsRecentTail(t *testing.T) {
	agent := New(&fakeClient{}, tools.NewRegistry(), 3)
	agent.options.Context.PruneToolResultMaxBytes = 8
	agent.options.Context.PruneKeepRecentMessages = 4
	largeResult := tools.Success("read", strings.Repeat("x", 40)).JSON()
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_read", "read_file", `{"path":"big.txt"}`)}},
		{Role: "tool", ToolCallID: "call_read", Content: largeResult},
		{Role: "user", Content: "recent"},
	}

	stats := agent.pruneStaleToolResults()
	if stats.Results != 0 {
		t.Fatalf("pruned recent results = %d, want 0", stats.Results)
	}
	if agent.messages[2].Content != largeResult {
		t.Fatalf("recent tool result changed:\n%s", agent.messages[2].Content)
	}
}

func TestCompactInsertsSummaryAndKeepsTailToolPair(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{{Content: "Earlier work was summarized."}}}
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 3, "/tmp/work", compactTestOptions())
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("old user ", 80)},
		{Role: "assistant", Content: strings.Repeat("old assistant ", 80)},
		{Role: "user", Content: strings.Repeat("old decision ", 80)},
		{Role: "assistant", Content: strings.Repeat("old result ", 80)},
		{Role: "user", Content: strings.Repeat("recent prelude ", 200)},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_recent", "read_file", `{"path":"fresh.txt"}`)}},
		{Role: "tool", ToolCallID: "call_recent", Content: tools.Success("read", "fresh output").JSON()},
	}

	stats, err := agent.compact(context.Background(), true)
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}
	if stats.Messages == 0 {
		t.Fatalf("compacted message count = 0")
	}
	if len(agent.messages) != 4 {
		t.Fatalf("message count after compact = %d, want 4: %#v", len(agent.messages), agent.messages)
	}
	if agent.messages[0].Role != "system" {
		t.Fatalf("system message not preserved: %#v", agent.messages[0])
	}
	if !strings.Contains(agent.messages[1].Content, compactionSummaryOpen) || !strings.Contains(agent.messages[1].Content, "Earlier work was summarized.") {
		t.Fatalf("missing summary message:\n%s", agent.messages[1].Content)
	}
	if agent.messages[2].Role != "assistant" || len(agent.messages[2].ToolCalls) != 1 {
		t.Fatalf("tail assistant tool call not preserved: %#v", agent.messages[2])
	}
	if agent.messages[3].Role != "tool" || agent.messages[3].ToolCallID != "call_recent" {
		t.Fatalf("tail tool result not preserved: %#v", agent.messages[3])
	}
}

func TestCompactFailureKeepsOriginalMessages(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{{
		ToolCalls: []llm.ToolCall{testToolCall("call_bad", "read_file", `{"path":"x"}`)},
	}}}
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 3, "/tmp/work", compactTestOptions())
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("old user ", 80)},
		{Role: "assistant", Content: strings.Repeat("old assistant ", 80)},
		{Role: "user", Content: strings.Repeat("recent ", 200)},
	}
	before := append([]llm.Message(nil), agent.messages...)

	if _, err := agent.compact(context.Background(), true); err == nil {
		t.Fatalf("compact() error = nil, want summary tool-call error")
	}
	if !reflect.DeepEqual(agent.messages, before) {
		t.Fatalf("messages changed after failed compact:\n%#v\nwant\n%#v", agent.messages, before)
	}
}

func compactTestOptions() Options {
	options := DefaultOptions()
	options.Context.CompactKeepTailTokens = 80
	options.Context.CompactTargetPercent = 50
	options.Context.WindowTokens = 1000
	options.Context.CompactMinMessages = 1
	return options
}
