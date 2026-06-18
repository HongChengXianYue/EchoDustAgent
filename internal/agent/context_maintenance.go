package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

const (
	prunedToolOutputPrefix  = "[stale tool result pruned:"
	compactionSummaryOpen   = "<compaction-summary>"
	compactionSummaryClose  = "</compaction-summary>"
	summarySystemPromptText = `You are compacting the earlier part of a coding agent conversation.
Write a concise but complete briefing that lets the agent continue without rereading the original messages.
Preserve user goals, decisions, constraints, files changed or inspected, commands and outcomes, errors and fixes, and pending next steps.
Do not invent facts.`
)

type pruneStats struct {
	Results     int
	BytesBefore int
	BytesAfter  int
}

type compactionStats struct {
	Messages     int
	TokensBefore int
	TokensAfter  int
}

func (a *Agent) pruneStaleToolResults() pruneStats {
	options := a.options.Context
	limit := options.PruneToolResultMaxBytes
	if limit <= 0 {
		return pruneStats{}
	}
	cutoff := len(a.messages) - options.PruneKeepRecentMessages
	if cutoff <= 1 {
		return pruneStats{}
	}
	stats := pruneStats{}
	for i := 1; i < cutoff && i < len(a.messages); i++ {
		message := a.messages[i]
		if message.Role != "tool" || strings.TrimSpace(message.Content) == "" {
			continue
		}
		var result tools.Result
		if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
			continue
		}
		if len(result.Output) <= limit || strings.HasPrefix(result.Output, prunedToolOutputPrefix) {
			continue
		}
		before := len(result.Output)
		result.Output = fmt.Sprintf("%s original output was %d bytes; summary and metadata preserved]", prunedToolOutputPrefix, before)
		encoded := result.JSON()
		a.messages[i].Content = encoded
		stats.Results++
		stats.BytesBefore += before
		stats.BytesAfter += len(result.Output)
	}
	if stats.Results > 0 {
		a.emit(runtimeevent.Event{
			Type:    runtimeevent.TypeContextPruned,
			Message: fmt.Sprintf("pruned %d stale tool result(s), saved about %d bytes", stats.Results, stats.BytesBefore-stats.BytesAfter),
			Count:   stats.Results,
			Before:  stats.BytesBefore,
			After:   stats.BytesAfter,
		})
	}
	return stats
}

func (a *Agent) maybeCompact(ctx context.Context) {
	options := a.options.Context
	if !options.CompactEnabled || options.WindowTokens <= 0 {
		return
	}
	before := estimateMessagesTokens(a.messages)
	threshold := options.WindowTokens * options.CompactRatioPercent / 100
	forceThreshold := options.WindowTokens * options.CompactForceRatioPercent / 100
	if before < threshold && before < forceThreshold {
		return
	}
	a.emit(runtimeevent.Event{
		Type:    runtimeevent.TypeCompactionStart,
		Message: fmt.Sprintf("compacting context at ~%d tokens", before),
		Before:  before,
	})
	stats, err := a.compact(ctx, before >= forceThreshold)
	if err != nil {
		a.emit(runtimeevent.Event{
			Type:    runtimeevent.TypeCompactionSkip,
			Message: err.Error(),
			Before:  before,
		})
		return
	}
	a.emit(runtimeevent.Event{
		Type:    runtimeevent.TypeCompactionDone,
		Message: fmt.Sprintf("compacted %d message(s), ~%d -> ~%d tokens", stats.Messages, stats.TokensBefore, stats.TokensAfter),
		Count:   stats.Messages,
		Before:  stats.TokensBefore,
		After:   stats.TokensAfter,
	})
}

func (a *Agent) compact(ctx context.Context, force bool) (compactionStats, error) {
	options := a.options.Context
	if len(a.messages) <= 2 {
		return compactionStats{}, fmt.Errorf("compaction skipped: not enough history")
	}
	before := estimateMessagesTokens(a.messages)
	tailBudget := options.CompactKeepTailTokens
	targetBudget := options.WindowTokens * options.CompactTargetPercent / 100
	if targetBudget > 0 && targetBudget < tailBudget {
		tailBudget = targetBudget
	}
	tailStart := compactTailStart(a.messages, 1, tailBudget)
	if tailStart <= 1 || tailStart >= len(a.messages) {
		return compactionStats{}, fmt.Errorf("compaction skipped: recent tail covers the available history")
	}
	fold := append([]llm.Message(nil), a.messages[1:tailStart]...)
	if !force && len(fold) < options.CompactMinMessages {
		return compactionStats{}, fmt.Errorf("compaction skipped: only %d compactable message(s)", len(fold))
	}
	summary, err := a.summarizeMessages(ctx, fold)
	if err != nil {
		return compactionStats{}, fmt.Errorf("compaction skipped: %w", err)
	}
	summaryMessage := llm.Message{
		Role: "user",
		Content: compactionSummaryOpen + "\n" +
			"Summary of earlier conversation (older messages were compacted to save context):\n" +
			strings.TrimSpace(summary) + "\n" +
			compactionSummaryClose,
	}
	compacted := make([]llm.Message, 0, 1+1+len(a.messages)-tailStart)
	compacted = append(compacted, a.messages[0])
	compacted = append(compacted, summaryMessage)
	compacted = append(compacted, a.messages[tailStart:]...)
	after := estimateMessagesTokens(compacted)
	if !force && after >= before {
		return compactionStats{}, fmt.Errorf("compaction skipped: summary would not reduce context")
	}
	a.messages = compacted
	return compactionStats{Messages: len(fold), TokensBefore: before, TokensAfter: after}, nil
}

func (a *Agent) summarizeMessages(ctx context.Context, messages []llm.Message) (string, error) {
	input := formatMessagesForSummary(messages)
	resp, err := a.client.ChatWithTools(ctx, []llm.Message{
		{Role: "system", Content: summarySystemPromptText},
		{Role: "user", Content: input},
	}, nil)
	if err != nil {
		return "", err
	}
	if len(resp.ToolCalls) > 0 {
		return "", fmt.Errorf("summary model returned tool calls")
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("summary model returned empty content")
	}
	return summary, nil
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

func estimateMessagesTokens(messages []llm.Message) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	return total
}

func estimateMessageTokens(message llm.Message) int {
	chars := len(message.Role) + len(message.Content) + len(message.ToolCallID)
	for _, call := range message.ToolCalls {
		chars += len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments)
	}
	return chars/4 + 4
}

func formatMessagesForSummary(messages []llm.Message) string {
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
