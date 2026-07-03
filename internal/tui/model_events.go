package tui

import (
	"strings"

	"local-agent/internal/runtimeevent"
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
		m.running = true
		m.interrupting = false
		m.lastRunHadFinal = false
		m.runErrorReported = false
		m.assistantDraft = ""
		m.tokens = tokenState{}
	case runtimeevent.TypeRunEnd:
		m.running = false
		m.interrupting = false
		m.hideSubagentPanel()
	case runtimeevent.TypeUserMessage:
		if strings.TrimSpace(event.Message) != "" {
			m.appendBlock(transcriptBlock{Kind: blockUser, Title: "You", Body: event.Message})
		}
	case runtimeevent.TypeAssistantDelta:
		if strings.TrimSpace(event.Delta) != "" {
			m.assistantDraft += event.Delta
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
		runtimeevent.TypeError:
		if event.Tool != "" {
			m.lastTool = event.Tool
		}
		if event.Type == runtimeevent.TypeError {
			m.runErrorReported = true
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
	case runtimeevent.TypeTokenUsage:
		if event.Source == "subagent" {
			return
		}
		m.tokens.Prompt += event.PromptTokens
		m.tokens.Completion += event.CompletionTokens
		m.tokens.Cached += event.CachedTokens
		m.tokens.Total = event.CumulativeTotal
	case runtimeevent.TypeFinal:
		m.lastRunHadFinal = true
		m.assistantDraft = ""
		m.hideSubagentPanel()
		if strings.TrimSpace(event.Message) != "" {
			m.appendBlock(transcriptBlock{
				Kind:     blockAssistant,
				Title:    "Agent",
				Body:     event.Message,
				Markdown: true,
			})
		}
	}
}

func (m *Model) appendBlock(block transcriptBlock) {
	m.blocks = append(m.blocks, block)
}
