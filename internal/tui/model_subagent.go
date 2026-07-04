package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func (m *Model) updateActiveViewport(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if m.viewingSubagent {
		m.subagentViewport, cmd = m.subagentViewport.Update(msg)
		return cmd
	}
	m.viewport, cmd = m.viewport.Update(msg)
	return cmd
}

func (m *Model) gotoActiveTop() {
	if m.viewingSubagent {
		m.subagentViewport.GotoTop()
		return
	}
	m.viewport.GotoTop()
}

func (m *Model) gotoActiveBottom() {
	if m.viewingSubagent {
		m.subagentViewport.GotoBottom()
		return
	}
	m.viewport.GotoBottom()
}

func (m *Model) shouldNavigateSubagents() bool {
	return m.showSubagents && !m.viewingSubagent && len(m.subagentOrder) > 0 && strings.TrimSpace(m.input.Value()) == "" && len(m.matchedSlashCommands()) == 0
}

func (m *Model) shouldOpenSelectedSubagent() bool {
	return m.shouldNavigateSubagents() && m.selectedSubagent != 0
}

func (m *Model) selectPreviousSubagent() bool {
	if len(m.subagentOrder) == 0 {
		return false
	}
	pos := m.selectedSubagentPosition()
	if pos <= 0 {
		m.selectedSubagent = m.subagentOrder[0]
		m.markSubagentViewportDirty()
		return true
	}
	m.selectedSubagent = m.subagentOrder[pos-1]
	m.markSubagentViewportDirty()
	return true
}

func (m *Model) selectNextSubagent() bool {
	if len(m.subagentOrder) == 0 {
		return false
	}
	pos := m.selectedSubagentPosition()
	if pos < 0 {
		m.selectedSubagent = m.subagentOrder[0]
		m.markSubagentViewportDirty()
		return true
	}
	if pos >= len(m.subagentOrder)-1 {
		m.selectedSubagent = m.subagentOrder[len(m.subagentOrder)-1]
		m.markSubagentViewportDirty()
		return true
	}
	m.selectedSubagent = m.subagentOrder[pos+1]
	m.markSubagentViewportDirty()
	return true
}

func (m *Model) selectedSubagentPosition() int {
	for i, index := range m.subagentOrder {
		if index == m.selectedSubagent {
			return i
		}
	}
	return -1
}

func (m *Model) selectedSubagentSession() *subagentSession {
	if m.selectedSubagent == 0 {
		return nil
	}
	return m.subagents[m.selectedSubagent]
}

func (m *Model) computeSubagentHeight(headerHeight, inputHeight int) int {
	if !m.showSubagents || len(m.subagentOrder) == 0 {
		return 0
	}
	desiredInner := 0
	if m.viewingSubagent {
		desiredInner = min(14, max(8, m.height/3))
	} else {
		rows := min(len(m.subagentOrder), 5)
		desiredInner = max(3, rows+1)
	}
	desired := desiredInner + m.subagentBoxStyle.GetVerticalFrameSize()
	maxAllowed := m.height - headerHeight - inputHeight - 5
	if maxAllowed <= m.subagentBoxStyle.GetVerticalFrameSize() {
		return 0
	}
	if desired > maxAllowed {
		return maxAllowed
	}
	return desired
}

func (m *Model) rebuildSubagentViewportContent() {
	session := m.selectedSubagentSession()
	if session == nil {
		m.subagentViewport.SetContent("")
		return
	}
	bodyWidth := max(20, m.subagentViewport.Width)
	parts := make([]string, 0, len(session.Blocks))
	for _, block := range session.Blocks {
		rendered := m.renderBlock(block, bodyWidth)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
	}
	content := strings.Join(parts, "\n\n")
	if strings.TrimSpace(content) == "" {
		content = "No subagent output yet."
	}
	wasAtBottom := m.subagentViewport.AtBottom()
	offset := m.subagentViewport.YOffset
	m.subagentViewport.SetContent(content)
	if wasAtBottom {
		m.subagentViewport.GotoBottom()
		return
	}
	m.subagentViewport.SetYOffset(offset)
}

func (m *Model) renderSubagentList() string {
	innerWidth := max(20, m.width-m.subagentBoxStyle.GetHorizontalFrameSize())
	innerHeight := max(1, m.subagentHeight-m.subagentBoxStyle.GetVerticalFrameSize())
	lines := []string{m.subagentTitleStyle.Render("Subagents")}
	lines = append(lines, m.visibleSubagentRows(max(1, innerHeight-1), innerWidth)...)
	content := joinPaddedLines(lines, innerHeight)
	return m.subagentBoxStyle.Width(innerWidth).Height(innerHeight).Render(content)
}

func (m *Model) visibleSubagentRows(limit, width int) []string {
	if limit <= 0 || len(m.subagentOrder) == 0 {
		return nil
	}
	pos := m.selectedSubagentPosition()
	if pos < 0 {
		pos = 0
	}
	start := 0
	if len(m.subagentOrder) > limit {
		start = pos - limit/2
		if start < 0 {
			start = 0
		}
		if end := start + limit; end > len(m.subagentOrder) {
			start = len(m.subagentOrder) - limit
		}
	}
	end := min(len(m.subagentOrder), start+limit)
	rows := make([]string, 0, end-start)
	for _, index := range m.subagentOrder[start:end] {
		session := m.subagents[index]
		if session == nil {
			continue
		}
		rows = append(rows, m.renderSubagentRow(session, width))
	}
	return rows
}

func (m *Model) renderSubagentRow(session *subagentSession, width int) string {
	if session == nil {
		return ""
	}
	marker := "○"
	style := m.subagentIdleStyle
	isSelected := m.selectedSubagent == session.Index
	if isSelected {
		style = m.subagentSelectedStyle
	}
	if m.viewingSubagent && isSelected {
		marker = "●"
		style = m.subagentOpenStyle
	}
	label := summarizeSubagentTask(session.Task, min(20, max(12, width/4)))
	if label == "" {
		label = "Pending task"
	}
	prefix := fmt.Sprintf("%s Subagent-%d  ", marker, session.Index)
	status := fmt.Sprintf("  [%s]", session.Status)
	tokenSummary := m.subagentRowTokenSummary(session, isSelected)
	labelWidth := width - lipgloss.Width(prefix) - lipgloss.Width(status) - lipgloss.Width(tokenSummary)
	if labelWidth < 8 {
		labelWidth = 8
	}
	label = truncateSingleLine(label, labelWidth)
	row := prefix + label + status + tokenSummary
	return style.Render(truncateSingleLine(row, max(8, width)))
}

func (m *Model) subagentRowTokenSummary(session *subagentSession, selected bool) string {
	if session == nil || session.TokenTotal <= 0 {
		return ""
	}
	summary := "  · " + formatCompactTokenCount(session.TokenTotal)
	if selected && session.Cached > 0 {
		summary += " | cache " + formatCompactTokenCount(session.Cached)
		if hitRate, ok := formatCacheHitRate(session.Cached, session.Prompt); ok {
			summary += " | hit " + hitRate
		}
	}
	return summary
}

func (m *Model) renderSubagentDetail() string {
	session := m.selectedSubagentSession()
	if session == nil {
		return ""
	}
	innerWidth := max(20, m.width-m.subagentBoxStyle.GetHorizontalFrameSize())
	innerHeight := max(2, m.subagentHeight-m.subagentBoxStyle.GetVerticalFrameSize())
	title := fmt.Sprintf("Subagent-%d  %s  [%s]", session.Index, summarizeSubagentTask(session.Task, max(12, min(28, innerWidth-24))), session.Status)
	content := m.subagentTitleStyle.Render(truncateSingleLine(title, innerWidth)) + "\n" + m.subagentViewport.View()
	return m.subagentBoxStyle.Width(innerWidth).Height(innerHeight).Render(content)
}

func joinPaddedLines(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func summarizeSubagentTask(task string, limit int) string {
	raw := collapseHorizontalSpace(strings.TrimSpace(task))
	if raw == "" {
		return ""
	}
	task = stripBracketedHints(raw)
	task = firstSubagentClause(task)
	task = strings.TrimSpace(task)
	for _, suffix := range []string{
		"相关代码",
		"相关实现",
		"代码实现",
		"的代码",
		"代码",
		"部分",
		"相关内容",
	} {
		task = strings.TrimSpace(strings.TrimSuffix(task, suffix))
	}
	if task == "" {
		task = raw
	}
	return truncateSingleLine(task, limit)
}

func firstSubagentClause(task string) string {
	best := len(task)
	for _, sep := range []string{"\n", "：", ":", "；", ";", "，", ","} {
		if idx := strings.Index(task, sep); idx >= 0 && idx < best {
			best = idx
		}
	}
	return task[:best]
}

func stripBracketedHints(text string) string {
	var out []rune
	depthRound := 0
	depthFull := 0
	for _, r := range text {
		switch r {
		case '(':
			depthRound++
			continue
		case ')':
			if depthRound > 0 {
				depthRound--
				continue
			}
		case '（':
			depthFull++
			continue
		case '）':
			if depthFull > 0 {
				depthFull--
				continue
			}
		}
		if depthRound == 0 && depthFull == 0 {
			out = append(out, r)
		}
	}
	return collapseHorizontalSpace(strings.TrimSpace(string(out)))
}

func (m *Model) resetSubagents() {
	m.subagents = map[int]*subagentSession{}
	m.subagentOrder = nil
	m.subagentTaskMap = map[string]int{}
	m.nextSubagentID = 1
	m.showSubagents = false
	m.selectedSubagent = 0
	m.viewingSubagent = false
	m.subagentViewport.SetContent("")
	m.subagentViewport.GotoTop()
	m.markAllDirty()
}

func (m *Model) hideSubagentPanel() {
	m.showSubagents = false
	m.viewingSubagent = false
	m.markLayoutDirty()
}

// captureSubagentEvent keeps delegated work out of the main transcript so the
// parent viewport stays readable while the child task is still running.
func (m *Model) captureSubagentEvent(event runtimeevent.Event) bool {
	if !m.isSubagentEvent(event) {
		return false
	}
	m.showSubagents = true
	m.markLayoutDirty()
	session := m.ensureSubagentSession(event)
	if session == nil {
		return true
	}
	m.updateSubagentStatus(session, event)
	m.appendSubagentBlock(session, event)
	return true
}

func (m *Model) isSubagentEvent(event runtimeevent.Event) bool {
	if event.Source == "subagent" {
		return true
	}
	if event.SubagentIndex > 0 && tools.IsDelegateTaskTool(event.Tool) {
		switch event.Type {
		case runtimeevent.TypeToolCall, runtimeevent.TypeToolResult, runtimeevent.TypeError:
			return true
		}
	}
	return false
}

func (m *Model) ensureSubagentSession(event runtimeevent.Event) *subagentSession {
	index := event.SubagentIndex
	task := strings.TrimSpace(event.ParentTool)
	if task == "" && tools.IsDelegateTaskTool(event.Tool) {
		task = strings.TrimSpace(jsonArgString(event.Args, "task"))
	}
	if index <= 0 {
		if task == "" {
			task = fmt.Sprintf("Subagent %d", m.nextSubagentID)
		}
		if existing, ok := m.subagentTaskMap[task]; ok {
			index = existing
		} else {
			for {
				index = m.nextSubagentID
				m.nextSubagentID++
				if _, exists := m.subagents[index]; !exists {
					break
				}
			}
			m.subagentTaskMap[task] = index
		}
	}
	session := m.subagents[index]
	if session == nil {
		session = &subagentSession{
			Index:  index,
			Status: "running",
		}
		m.subagents[index] = session
		m.subagentOrder = append(m.subagentOrder, index)
		if m.selectedSubagent == 0 {
			m.selectedSubagent = index
		}
	}
	if task != "" {
		session.Task = task
		m.subagentTaskMap[task] = index
	}
	if session.Task == "" {
		session.Task = fmt.Sprintf("Subagent-%d", index)
	}
	return session
}

func (m *Model) updateSubagentStatus(session *subagentSession, event runtimeevent.Event) {
	if session == nil {
		return
	}
	switch event.Type {
	case runtimeevent.TypeToolResult:
		if event.Result != nil && event.Result.Status == "error" {
			session.Status = "error"
		} else if tools.IsDelegateTaskTool(event.Tool) {
			if event.Result != nil && strings.TrimSpace(event.Result.Summary) == "subagent completed" {
				session.Status = "done"
			} else {
				session.Status = "running"
			}
		} else if session.Status != "done" {
			session.Status = "running"
		}
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			session.Status = "running"
		} else if session.Status != "done" {
			session.Status = "running"
		}
	case runtimeevent.TypeError:
		session.Status = "error"
	case runtimeevent.TypeTokenUsage:
		session.Prompt += event.PromptTokens
		session.TokenTotal = event.CumulativeTotal
		session.Cached += event.CachedTokens
	default:
		if session.Status == "" {
			session.Status = "running"
		}
	}
}

func (m *Model) appendSubagentBlock(session *subagentSession, event runtimeevent.Event) {
	if session == nil {
		return
	}
	displayEvent := event
	displayEvent.Source = ""
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		message := cleanTerminalText(event.Message)
		if message == "" {
			return
		}
		session.Blocks = append(session.Blocks, transcriptBlock{
			Kind:  blockAssistant,
			Title: "Agent",
			Body:  message,
		})
		session.LastTitle = "Agent"
		m.markSubagentViewportDirty()
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
		title := toolEventTitle(displayEvent, m.options.ApprovalArgsPreviewChars)
		if title == "" {
			return
		}
		kind := blockInfo
		if event.Type == runtimeevent.TypeToolCall {
			kind = blockToolCall
		}
		if event.Type == runtimeevent.TypeError || (event.Type == runtimeevent.TypeToolResult && event.Result != nil && event.Result.Status == "error") {
			kind = blockError
		}
		session.Blocks = append(session.Blocks, transcriptBlock{
			Kind:  kind,
			Title: title,
			Body: toolEventDetail(
				displayEvent,
				m.options.ApprovalArgsPreviewChars,
				m.options.ToolPreviewOutputChars,
				m.options.ToolPreviewLongOutputChars,
				m.options.FileChangePreviewChars,
			),
		})
		session.LastTitle = title
		m.markSubagentViewportDirty()
	}
}
