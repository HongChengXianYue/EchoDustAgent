package contextmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

const (
	PrunedToolOutputPrefix = "[stale tool result pruned:"
	CompactionSummaryOpen  = "<compaction-summary>"
	CompactionSummaryClose = "</compaction-summary>"
	SummarySystemPrompt    = `You are compacting the earlier part of an agent conversation.
Write a concise but complete briefing that lets the agent continue without rereading the original messages.
Preserve user goals, decisions, constraints, files changed or inspected, commands and outcomes, errors and fixes, and pending next steps.
Do not invent facts.`
)

type Options struct {
	WindowTokens             int
	PruneToolResultMaxBytes  int
	PruneKeepRecentMessages  int
	CompactEnabled           bool
	CompactRatioPercent      int
	CompactForceRatioPercent int
	CompactTargetPercent     int
	CompactKeepTailTokens    int
	CompactMinMessages       int
}

type PruneStats struct {
	Results     int
	BytesBefore int
	BytesAfter  int
}

type CompactionStats struct {
	Messages     int
	TokensBefore int
	TokensAfter  int
}

type SummaryFunc func(ctx context.Context, messages []llm.Message) (string, error)

func PruneStaleToolResults(messages []llm.Message, options Options) PruneStats {
	limit := options.PruneToolResultMaxBytes
	if limit <= 0 {
		return PruneStats{}
	}
	cutoff := len(messages) - options.PruneKeepRecentMessages
	if cutoff <= 1 {
		return PruneStats{}
	}
	stats := PruneStats{}
	for i := 1; i < cutoff && i < len(messages); i++ {
		message := messages[i]
		if message.Role != "tool" || strings.TrimSpace(message.Content) == "" {
			continue
		}
		var result tools.Result
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if len(result.Output) <= limit || strings.HasPrefix(result.Output, PrunedToolOutputPrefix) {
			continue
		}
		before := len(result.Output)
		result.Output = fmt.Sprintf("%s original output was %d bytes; summary and metadata preserved]", PrunedToolOutputPrefix, before)
		messages[i].Content = result.JSON()
		stats.Results++
		stats.BytesBefore += before
		stats.BytesAfter += len(result.Output)
	}
	return stats
}

func CompactionTrigger(messages []llm.Message, options Options) (before int, force bool, ok bool) {
	if !options.CompactEnabled || options.WindowTokens <= 0 {
		return 0, false, false
	}
	before = EstimateMessagesTokens(messages)
	threshold := options.WindowTokens * options.CompactRatioPercent / 100
	forceThreshold := options.WindowTokens * options.CompactForceRatioPercent / 100
	if before < threshold && before < forceThreshold {
		return before, false, false
	}
	return before, before >= forceThreshold, true
}

func Compact(ctx context.Context, messages []llm.Message, options Options, summarize SummaryFunc, force bool) ([]llm.Message, CompactionStats, error) {
	if len(messages) <= 2 {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: not enough history")
	}
	if summarize == nil {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: missing summarizer")
	}
	before := EstimateMessagesTokens(messages)
	tailBudget := options.CompactKeepTailTokens
	targetBudget := options.WindowTokens * options.CompactTargetPercent / 100
	if targetBudget > 0 && targetBudget < tailBudget {
		tailBudget = targetBudget
	}
	tailStart := compactTailStart(messages, 1, tailBudget)
	if tailStart <= 1 || tailStart >= len(messages) {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: recent tail covers the available history")
	}
	fold := append([]llm.Message(nil), messages[1:tailStart]...)
	if !force && len(fold) < options.CompactMinMessages {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: only %d compactable message(s)", len(fold))
	}
	summary, err := summarize(ctx, fold)
	if err != nil {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: %w", err)
	}
	summaryMessage := llm.Message{
		Role: "user",
		Content: CompactionSummaryOpen + "\n" +
			"Summary of earlier conversation (older messages were compacted to save context):\n" +
			strings.TrimSpace(summary) + "\n" +
			CompactionSummaryClose,
	}
	compacted := make([]llm.Message, 0, 1+1+len(messages)-tailStart)
	compacted = append(compacted, messages[0])
	compacted = append(compacted, summaryMessage)
	compacted = append(compacted, messages[tailStart:]...)
	after := EstimateMessagesTokens(compacted)
	if !force && after >= before {
		return nil, CompactionStats{}, fmt.Errorf("compaction skipped: summary would not reduce context")
	}
	return compacted, CompactionStats{Messages: len(fold), TokensBefore: before, TokensAfter: after}, nil
}

func EstimateMessagesTokens(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	return total
}

func compactTailStart(messages []llm.Message, head int, budgetTokens int) int {
	if budgetTokens <= 0 {
		budgetTokens = 1
	}
	used := 0
	start := len(messages)
	for i := len(messages) - 1; i >= head; i-- {
		next := used + estimateMessageTokens(messages[i])
		if start < len(messages) && next > budgetTokens {
			break
		}
		used = next
		start = i
	}
	if start < len(messages) && messages[start].Role == "tool" {
		for start > head {
			start--
			if messages[start].Role == "assistant" && len(messages[start].ToolCalls) > 0 {
				break
			}
		}
	}
	return start
}

func estimateMessageTokens(message llm.Message) int {
	chars := len(message.Role) + len(message.Content) + len(message.ToolCallID)
	for _, call := range message.ToolCalls {
		chars += len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments)
	}
	return chars/4 + 4
}

func FormatMessagesForSummary(messages []llm.Message) string {
	var b strings.Builder
	for i, message := range messages {
		fmt.Fprintf(&b, "## Message %d: %s\n", i+1, message.Role)
		if strings.TrimSpace(message.Content) != "" {
			b.WriteString(message.Content)
			b.WriteString("\n")
		}
		if message.ToolCallID != "" {
			fmt.Fprintf(&b, "tool_call_id: %s\n", message.ToolCallID)
		}
		for _, call := range message.ToolCalls {
			fmt.Fprintf(&b, "tool_call: %s %s\n", call.Function.Name, call.Function.Arguments)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
