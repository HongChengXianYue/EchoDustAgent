package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/runtimeevent"
)

const contentLeftInset = 4

func (m *Model) syncLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	suggestionCount := len(m.matchedSlashCommands())
	if suggestionCount > 5 {
		suggestionCount = 5
	}
	headerHeight := lipgloss.Height(m.renderHeader())
	inputHeight := 1 + m.inputBoxStyle.GetVerticalFrameSize() + suggestionCount
	panelHeight := m.computeSubagentHeight(headerHeight, inputHeight)
	m.subagentHeight = panelHeight

	innerWidth := max(20, m.width-m.contentStyle.GetHorizontalFrameSize())
	viewportHeight := m.height - headerHeight - inputHeight - panelHeight
	if viewportHeight < 5 {
		viewportHeight = 5
	}

	m.viewport.Width = max(20, innerWidth-contentLeftInset)
	m.viewport.Height = viewportHeight
	inputInnerWidth := max(10, m.width-m.inputBoxStyle.GetHorizontalFrameSize())
	m.input.Width = max(10, inputInnerWidth-lipgloss.Width(m.input.Prompt)-1)
	subagentInnerWidth := max(20, m.width-m.subagentBoxStyle.GetHorizontalFrameSize())
	subagentInnerHeight := max(1, panelHeight-m.subagentBoxStyle.GetVerticalFrameSize())
	if m.viewingSubagent {
		subagentInnerHeight = max(2, subagentInnerHeight)
	}
	m.subagentViewport.Width = subagentInnerWidth
	if m.viewingSubagent {
		m.subagentViewport.Height = max(1, subagentInnerHeight-1)
	} else {
		m.subagentViewport.Height = max(1, subagentInnerHeight)
	}

	m.rebuildViewportContent()
	m.rebuildSubagentViewportContent()
}

func (m *Model) rebuildViewportContent() {
	bodyWidth := max(20, m.viewport.Width)
	parts := make([]string, 0, len(m.blocks)+1)
	attachedApproval := false
	for i, block := range m.blocks {
		rendered := m.renderBlock(block, bodyWidth)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
		if m.shouldAttachInlineApproval(i, block) {
			parts = append(parts, m.renderInlineApprovalOptions(bodyWidth))
			attachedApproval = true
		}
	}
	// approvalPromptMsg and runtimeEventMsg are sent separately; if the prompt
	// arrives first, keep the inline approval visible instead of waiting for the
	// approval_request event block to catch up.
	if m.approval != nil && !attachedApproval {
		parts = append(parts, m.renderBlock(m.pendingApprovalBlock(), bodyWidth))
		parts = append(parts, m.renderInlineApprovalOptions(bodyWidth))
	}
	if strings.TrimSpace(m.assistantDraft) != "" {
		parts = append(parts, m.renderBlock(transcriptBlock{
			Kind:  blockAssistant,
			Title: "Agent (streaming)",
			Body:  cleanTerminalText(m.assistantDraft),
		}, bodyWidth))
	}
	content := strings.Join(parts, "\n\n")
	wasAtBottom := m.viewport.AtBottom()
	offset := m.viewport.YOffset
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(offset)
}

func (m *Model) renderBlock(block transcriptBlock, width int) string {
	title := block.Title
	if strings.TrimSpace(title) == "" && strings.TrimSpace(block.Body) == "" {
		return ""
	}
	var titleLine string
	switch block.Kind {
	case blockUser:
		titleLine = m.userStyle.Render(title)
	case blockAssistant:
		titleLine = m.infoStyle.Render(title)
	case blockError:
		titleLine = m.errorStyle.Render(title)
	case blockToolCall:
		titleLine = m.toolCallDotStyle.Render("●") + " " + m.toolCallTitleStyle.Render(title)
	case blockApprovalRequest:
		titleLine = m.titleStyle.Render(title)
	default:
		titleLine = m.titleStyle.Render(title)
	}
	if strings.TrimSpace(block.Body) == "" {
		return titleLine
	}
	body := block.Body
	if block.Markdown {
		renderer := m.markdownForWidth(max(20, width))
		rendered, err := renderMarkdown(renderer, body)
		if err == nil {
			return titleLine + "\n" + indentBlock(strings.TrimRight(rendered, "\n"), "  ")
		}
	}
	return titleLine + "\n" + indentBlock(wrapText(body, max(20, width)), "  ")
}

func (m *Model) shouldAttachInlineApproval(index int, block transcriptBlock) bool {
	return m.approval != nil && block.Kind == blockApprovalRequest && index == len(m.blocks)-1
}

func (m *Model) pendingApprovalBlock() transcriptBlock {
	if m.approval == nil {
		return transcriptBlock{}
	}
	request := m.approval.Request
	return transcriptBlock{
		Kind:  blockApprovalRequest,
		Title: toolEventTitle(runtimeevent.Event{Type: runtimeevent.TypeApprovalRequest}, m.options.ApprovalArgsPreviewChars),
		Body: approvalDetail(runtimeevent.Event{
			Tool:     request.Tool,
			Category: request.Category,
			Args:     request.Args,
			Reason:   request.Reason,
		}, m.options.ApprovalArgsPreviewChars),
	}
}

func (m *Model) renderInlineApprovalOptions(width int) string {
	if m.approval == nil {
		return ""
	}
	request := m.approval.Request
	lines := make([]string, 0, len(m.approval.Options)+1)
	for i, option := range m.approval.Options {
		line := "  " + approvalOptionLabel(request, option)
		if i == m.approval.Selected {
			line = m.approvalSelectedStyle.Render("› " + approvalOptionLabel(request, option))
		}
		lines = append(lines, line)
	}
	lines = append(lines, m.approvalHintStyle.Render("Left/Right, Up/Down, Tab choose  •  Enter confirm  •  N deny"))
	return indentBlock(wrapText(strings.Join(lines, "\n"), max(20, width)), "  ")
}

func (m *Model) markdownForWidth(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = m.options.MarkdownWordWrap
	}
	if width == m.markdownWrapWidth && m.markdownRenderer != nil {
		return m.markdownRenderer
	}
	renderer, err := newMarkdownRenderer(width)
	if err != nil {
		return nil
	}
	m.markdownWrapWidth = width
	m.markdownRenderer = renderer
	return m.markdownRenderer
}
