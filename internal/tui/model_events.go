package tui

import (
	"strings"
	"time"

	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func (m *Model) applyRuntimeEvent(event runtimeevent.Event) {
	if event.Type == runtimeevent.TypeRunStart && event.Source == "" {
		m.resetSubagents()
	}
	if m.captureSubagentEvent(event) {
		return
	}
	switch event.Type {
	case runtimeevent.TypeRunStart:
		m.resumePicker = nil
		m.running = true
		if m.runStartedAt.IsZero() {
			m.runStartedAt = time.Now()
		}
		m.runElapsedMS = 0
		m.runStartBlock = len(m.blocks)
		m.interrupting = false
		m.lastRunHadFinal = false
		m.runErrorReported = false
		m.assistantDraft = ""
		m.todos = nil
		m.tokens = tokenState{}
		m.markLayoutDirty()
		m.markViewportDirty()
	case runtimeevent.TypeRunEnd:
		m.running = false
		if !m.runStartedAt.IsZero() {
			m.runElapsedMS = time.Since(m.runStartedAt).Milliseconds()
		}
		m.interrupting = false
		m.hideSubagentPanel()
		m.markLayoutDirty()
		m.markViewportDirty()
	case runtimeevent.TypeUserMessage:
		if strings.TrimSpace(event.Message) != "" {
			m.appendBlock(transcriptBlock{Kind: blockUser, Title: "You", Body: event.Message})
		}
	case runtimeevent.TypeAssistantDelta:
		if strings.TrimSpace(event.Delta) != "" {
			m.assistantDraft += event.Delta
			m.markViewportDirty()
		}
	case runtimeevent.TypeAssistantMessage:
		if strings.TrimSpace(event.Message) != "" {
			m.appendBlock(transcriptBlock{
				Kind:  blockAssistant,
				Title: "Agent",
				Body:  cleanTerminalText(event.Message),
			})
		}
	case runtimeevent.TypeTodoUpdate:
		m.todos = append([]runtimeevent.TodoItem(nil), event.Todos...)
		m.markViewportDirty()
	case runtimeevent.TypeToolCall,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeApprovalRequest,
		runtimeevent.TypeApprovalDecision,
		runtimeevent.TypeContextPruned,
		runtimeevent.TypeCompactionStart,
		runtimeevent.TypeCompactionDone,
		runtimeevent.TypeCompactionSkip,
		runtimeevent.TypeStepBudgetExtend,
		runtimeevent.TypeStepBudgetStop,
		runtimeevent.TypeStepTiming,
		runtimeevent.TypeRunTiming,
		runtimeevent.TypeError:
		if event.Tool != "" {
			m.lastTool = event.Tool
		}
		if event.Type == runtimeevent.TypeError {
			m.runErrorReported = true
		}
		if event.Type == runtimeevent.TypeRunTiming {
			m.runElapsedMS = event.DurationMS
		}
		if event.Type == runtimeevent.TypeToolResult && event.Result != nil && event.Result.Status == "success" && hasRenderableDiffChanges(*event.Result) {
			m.appendDiffBlocks(*event.Result)
		}
		title := toolEventTitle(event, m.options.ApprovalArgsPreviewChars)
		if title == "" {
			return
		}
		kind := blockInfo
		if event.Type == runtimeevent.TypeToolCall {
			kind = blockToolCall
		}
		if event.Type == runtimeevent.TypeApprovalRequest {
			kind = blockApprovalRequest
		}
		if event.Type == runtimeevent.TypeError || (event.Type == runtimeevent.TypeToolResult && event.Result != nil && event.Result.Status == "error") {
			kind = blockError
		}
		m.appendBlock(transcriptBlock{
			Kind:  kind,
			Title: title,
			Body: toolEventDetail(
				event,
				m.options.ApprovalArgsPreviewChars,
				m.options.ToolPreviewOutputChars,
				m.options.ToolPreviewLongOutputChars,
				m.options.FileChangePreviewChars,
			),
		})
		if event.Type == runtimeevent.TypeError {
			m.persistSessionSnapshot()
		}
	case runtimeevent.TypeTokenUsage:
		if event.Source == "subagent" {
			return
		}
		m.tokens.Prompt += event.PromptTokens
		m.tokens.Completion += event.CompletionTokens
		m.tokens.Cached += event.CachedTokens
		m.tokens.Total = event.CumulativeTotal
		m.markLayoutDirty()
	case runtimeevent.TypeFinal:
		m.lastRunHadFinal = true
		m.assistantDraft = ""
		m.markViewportDirty()
		m.hideSubagentPanel()
		if strings.TrimSpace(event.Message) != "" {
			m.appendBlock(transcriptBlock{
				Kind:     blockAssistant,
				Title:    "Agent",
				Body:     event.Message,
				Markdown: true,
			})
		}
		m.persistSessionSnapshot()
	}
}

func (m *Model) appendBlock(block transcriptBlock) {
	m.blocks = append(m.blocks, block)
	m.markViewportDirty()
}

func (m *Model) appendDiffBlocks(result tools.Result) {
	for _, change := range result.Changes {
		body := diffBlockBody(change)
		if strings.TrimSpace(body) == "" {
			continue
		}
		m.appendBlock(transcriptBlock{
			Kind:  blockDiff,
			Title: diffBlockTitle(change),
			Body:  body,
		})
	}
}

func hasRenderableDiffChanges(result tools.Result) bool {
	for _, change := range result.Changes {
		if strings.TrimSpace(diffBlockBody(change)) != "" {
			return true
		}
	}
	return false
}
