package tui

import "local-agent/internal/session"

func (m *Model) SessionSnapshot() session.UISnapshot {
	if m == nil {
		return session.UISnapshot{}
	}
	snapshot := session.UISnapshot{
		Blocks: make([]session.TranscriptBlockSnapshot, 0, len(m.blocks)),
		Tokens: session.TokenSnapshot{
			Prompt:     m.tokens.Prompt,
			Completion: m.tokens.Completion,
			Total:      m.tokens.Total,
			Cached:     m.tokens.Cached,
		},
	}
	for _, block := range m.blocks {
		if !persistResumeBlock(block) {
			continue
		}
		snapshot.Blocks = append(snapshot.Blocks, session.TranscriptBlockSnapshot{
			Kind:     snapshotKind(block.Kind),
			Title:    block.Title,
			Body:     block.Body,
			Markdown: block.Markdown,
		})
	}
	return snapshot
}

func (m *Model) LoadSessionSnapshot(snapshot session.UISnapshot) {
	if m == nil {
		return
	}
	m.blocks = make([]transcriptBlock, 0, len(snapshot.Blocks))
	for _, block := range snapshot.Blocks {
		restored := transcriptBlock{
			Kind:     transcriptKindFromSnapshot(block.Kind),
			Title:    block.Title,
			Body:     block.Body,
			Markdown: block.Markdown,
		}
		if !persistResumeBlock(restored) {
			continue
		}
		m.blocks = append(m.blocks, restored)
	}
	m.resetSubagents()
	// Resume restores the main transcript only. Historical subagent state is
	// intentionally ignored so restored sessions do not reopen the subagent
	// panel or carry forward stale delegated-work details from older runs.
	m.tokens = tokenState{
		Prompt:     snapshot.Tokens.Prompt,
		Completion: snapshot.Tokens.Completion,
		Total:      snapshot.Tokens.Total,
		Cached:     snapshot.Tokens.Cached,
	}
	m.resumePicker = nil
	m.running = false
	m.runStartBlock = len(m.blocks)
	m.interrupting = false
	m.cancelCurrent = nil
	m.lastRunHadFinal = false
	m.runErrorReported = false
	m.lastTool = ""
	m.assistantDraft = ""
	m.chatRetry = nil
	m.approval = nil
	m.todos = nil
	m.viewingSubagent = false
	m.mainViewportBuffer = renderedTextBuffer{}
	m.copySelection = nil
	m.copyNotice = ""
	m.copyNoticeError = false
	m.markAllDirty()
	if m.width > 0 && m.height > 0 {
		m.syncLayout()
		m.viewport.GotoBottom()
	}
}

func (m *Model) AppendInfoBlock(title, body string) {
	if m == nil {
		return
	}
	m.appendBlock(transcriptBlock{
		Kind:  blockInfo,
		Title: title,
		Body:  body,
	})
	if m.width > 0 && m.height > 0 {
		m.syncLayout()
		m.viewport.GotoBottom()
	}
}

func (m *Model) persistSessionSnapshot() {
	if m == nil || m.snapshotSaver == nil {
		return
	}
	m.snapshotSaver(m.SessionSnapshot())
}

func snapshotKind(kind transcriptKind) string {
	switch kind {
	case blockUser:
		return "user"
	case blockAssistant:
		return "assistant"
	case blockError:
		return "error"
	case blockToolCall:
		return "tool_call"
	case blockApprovalRequest:
		return "approval_request"
	case blockDiff:
		return "diff"
	default:
		return "info"
	}
}

func transcriptKindFromSnapshot(kind string) transcriptKind {
	switch kind {
	case "user":
		return blockUser
	case "assistant":
		return blockAssistant
	case "error":
		return blockError
	case "tool_call":
		return blockToolCall
	case "approval_request":
		return blockApprovalRequest
	case "diff":
		return blockDiff
	default:
		return blockInfo
	}
}

func persistResumeBlock(block transcriptBlock) bool {
	return block.Kind != blockToolCall
}
