package contextmgr

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

func TestPruneStaleToolResultsTrimsOldOutputAndKeepsPairing(t *testing.T) {
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_read", "read_file", `{"path":"big.txt"}`)}},
		{Role: "tool", ToolCallID: "call_read", Content: tools.Success("read", strings.Repeat("x", 40)).JSON()},
		{Role: "user", Content: "newer"},
	}

	stats := PruneStaleToolResults(messages, Options{PruneToolResultMaxBytes: 8, PruneKeepRecentMessages: 1})
	if stats.Results != 1 {
		t.Fatalf("pruned results = %d, want 1", stats.Results)
	}
	if len(messages[1].ToolCalls) != 1 || messages[1].ToolCalls[0].ID != "call_read" {
		t.Fatalf("assistant tool call pairing changed: %#v", messages[1])
	}
	if messages[2].Role != "tool" || messages[2].ToolCallID != "call_read" {
		t.Fatalf("tool message pairing changed: %#v", messages[2])
	}
	if !strings.Contains(messages[2].Content, PrunedToolOutputPrefix) {
		t.Fatalf("tool result was not pruned: %s", messages[2].Content)
	}
}

func TestPruneStaleToolResultsProtectsRecentTail(t *testing.T) {
	largeResult := tools.Success("read", strings.Repeat("x", 40)).JSON()
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_read", "read_file", `{"path":"big.txt"}`)}},
		{Role: "tool", ToolCallID: "call_read", Content: largeResult},
		{Role: "user", Content: "recent"},
	}

	stats := PruneStaleToolResults(messages, Options{PruneToolResultMaxBytes: 8, PruneKeepRecentMessages: 4})
	if stats.Results != 0 {
		t.Fatalf("pruned recent results = %d, want 0", stats.Results)
	}
	if messages[2].Content != largeResult {
		t.Fatalf("recent tool result changed:\n%s", messages[2].Content)
	}
}

func TestCompactInsertsSummaryAndKeepsTailToolPair(t *testing.T) {
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("old user ", 80)},
		{Role: "assistant", Content: strings.Repeat("old assistant ", 80)},
		{Role: "user", Content: strings.Repeat("old decision ", 80)},
		{Role: "assistant", Content: strings.Repeat("old result ", 80)},
		{Role: "user", Content: strings.Repeat("recent prelude ", 200)},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_recent", "read_file", `{"path":"fresh.txt"}`)}},
		{Role: "tool", ToolCallID: "call_recent", Content: tools.Success("read", "fresh output").JSON()},
	}

	compacted, stats, err := Compact(context.Background(), messages, compactTestOptions(), fixedSummary("Earlier work was summarized."), true)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}
	if stats.Messages == 0 {
		t.Fatalf("compacted message count = 0")
	}
	if len(compacted) != 4 {
		t.Fatalf("message count after compact = %d, want 4: %#v", len(compacted), compacted)
	}
	if compacted[0].Role != "system" {
		t.Fatalf("system message not preserved: %#v", compacted[0])
	}
	if !strings.Contains(compacted[1].Content, CompactionSummaryOpen) || !strings.Contains(compacted[1].Content, "Earlier work was summarized.") {
		t.Fatalf("missing summary message:\n%s", compacted[1].Content)
	}
	if compacted[2].Role != "assistant" || len(compacted[2].ToolCalls) != 1 {
		t.Fatalf("tail assistant tool call not preserved: %#v", compacted[2])
	}
	if compacted[3].Role != "tool" || compacted[3].ToolCallID != "call_recent" {
		t.Fatalf("tail tool result not preserved: %#v", compacted[3])
	}
}

func TestCompactFailureKeepsOriginalMessages(t *testing.T) {
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("old user ", 80)},
		{Role: "assistant", Content: strings.Repeat("old assistant ", 80)},
		{Role: "user", Content: strings.Repeat("recent ", 200)},
	}
	before := append([]llm.Message(nil), messages...)

	if _, _, err := Compact(context.Background(), messages, compactTestOptions(), func(context.Context, []llm.Message) (string, error) {
		return "", errSummaryToolCalls{}
	}, true); err == nil {
		t.Fatalf("Compact() error = nil, want summary error")
	}
	if !reflect.DeepEqual(messages, before) {
		t.Fatalf("messages changed after failed compact:\n%#v\nwant\n%#v", messages, before)
	}
}

func compactTestOptions() Options {
	return Options{
		WindowTokens:             1000,
		CompactEnabled:           true,
		CompactRatioPercent:      80,
		CompactForceRatioPercent: 90,
		CompactTargetPercent:     50,
		CompactKeepTailTokens:    80,
		CompactMinMessages:       1,
	}
}

func fixedSummary(summary string) SummaryFunc {
	return func(context.Context, []llm.Message) (string, error) {
		return summary, nil
	}
}

type errSummaryToolCalls struct{}

func (errSummaryToolCalls) Error() string {
	return "summary model returned tool calls"
}

func testToolCall(id string, name string, args string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.ToolFunction{
			Name:      name,
			Arguments: args,
		},
	}
}
