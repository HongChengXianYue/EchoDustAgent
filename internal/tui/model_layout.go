package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/runtimeevent"
)

const contentLeftInset = 4

func (m *Model) syncLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	if !m.layoutDirty && !m.viewportDirty && !m.subagentViewportDirty {
		return
	}
	if m.layoutDirty {
		mainAtBottom := m.viewport.AtBottom()
		subagentAtBottom := m.subagentViewport.AtBottom()
		prevViewportHeight := m.viewport.Height
		prevSubagentHeight := m.subagentViewport.Height
		suggestionCount := len(m.matchedSlashCommands())
		if suggestionCount > maxVisibleSlashSuggestions {
			suggestionCount = maxVisibleSlashSuggestions
		}
		headerHeight := lipgloss.Height(m.renderHeader())
		inputInnerWidth := max(10, m.width-m.inputBoxStyle.GetHorizontalFrameSize())
		m.input.SetWidth(inputInnerWidth)
		m.input.SetHeight(m.desiredInputLines(inputInnerWidth, min(8, max(3, m.height/3))))
		inputHeight := m.input.Height() + m.inputBoxStyle.GetVerticalFrameSize() + suggestionCount
		footerHeight := 0
		if m.shouldRenderStatusBar(max(12, m.width-2)) {
			footerHeight = 1
		}
		panelHeight := m.computeSubagentHeight(headerHeight, inputHeight)
		m.subagentHeight = panelHeight

		innerWidth := max(20, m.width-m.contentStyle.GetHorizontalFrameSize())
		viewportHeight := m.height - headerHeight - inputHeight - panelHeight - footerHeight
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		viewportWidth := max(20, innerWidth-contentLeftInset)
		if m.viewport.Width != viewportWidth {
			m.viewportDirty = true
		}
		m.viewport.Width = viewportWidth
		m.viewport.Height = viewportHeight

		subagentInnerWidth := max(20, m.width-m.subagentBoxStyle.GetHorizontalFrameSize())
		subagentInnerHeight := max(1, panelHeight-m.subagentBoxStyle.GetVerticalFrameSize())
		if m.viewingSubagent {
			subagentInnerHeight = max(2, subagentInnerHeight)
		}
		if m.subagentViewport.Width != subagentInnerWidth {
			m.subagentViewportDirty = true
		}
		m.subagentViewport.Width = subagentInnerWidth
		if m.viewingSubagent {
			m.subagentViewport.Height = max(1, subagentInnerHeight-1)
		} else {
			m.subagentViewport.Height = max(1, subagentInnerHeight)
		}
		if !m.viewportDirty && mainAtBottom && prevViewportHeight != m.viewport.Height {
			m.viewport.GotoBottom()
		}
		if !m.subagentViewportDirty && subagentAtBottom && prevSubagentHeight != m.subagentViewport.Height {
			m.subagentViewport.GotoBottom()
		}
		m.layoutDirty = false
	}
	if m.viewportDirty {
		m.rebuildViewportContent()
		m.viewportDirty = false
	}
	if m.subagentViewportDirty {
		m.rebuildSubagentViewportContent()
		m.subagentViewportDirty = false
	}
}

func (m *Model) rebuildViewportContent() {
	bodyWidth := max(20, m.viewport.Width)
	parts := make([]string, 0, len(m.blocks)+2)
	attachedApproval := false
	retryBlock := m.renderLiveChatRetryBlock(bodyWidth)
	todoBlock := m.renderLiveTodoBlock(bodyWidth)
	for i, block := range m.blocks {
		block = m.prepareBlockForRender(i, block)
		rendered := m.renderBlock(block, bodyWidth)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
		if m.shouldAttachInlineApproval(i, block) {
			parts = append(parts, m.renderInlineApprovalOptions(bodyWidth))
			attachedApproval = true
		}
	}
	if picker := m.renderResumePicker(bodyWidth); strings.TrimSpace(picker) != "" {
		parts = append(parts, picker)
	}
	// approvalPromptMsg and runtimeEventMsg are sent separately; if the prompt
	// arrives first, keep the inline approval visible instead of waiting for the
	// approval_request event block to catch up.
	if m.approval != nil && !attachedApproval {
		parts = append(parts, m.renderBlock(m.pendingApprovalBlock(), bodyWidth))
		parts = append(parts, m.renderInlineApprovalOptions(bodyWidth))
	}
	if draft := m.prepareAssistantBodyForRender(m.assistantDraft, false, true); draft != "" {
		parts = append(parts, m.renderBlock(transcriptBlock{
			Kind:  blockAssistant,
			Title: "Agent (streaming)",
			Body:  draft,
		}, bodyWidth))
	}
	if retryBlock != "" {
		parts = append(parts, retryBlock)
	}
	// Keep the live todo checklist pinned to the end of the content area so the
	// latest agent/tool transcript always stays above it.
	if todoBlock != "" {
		parts = append(parts, todoBlock)
	}
	content := strings.Join(parts, "\n\n")
	wasAtBottom := m.viewport.AtBottom()
	offset := m.viewport.YOffset
	// Copy selection is anchored to the already-wrapped viewport lines. Once the
	// transcript is rebuilt those coordinates are no longer stable, so drop the
	// selection together with the cached line buffer.
	m.clearCopySelection()
	m.mainViewportBuffer = buildRenderedTextBuffer(content)
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
		return
	}
	m.viewport.SetYOffset(offset)
}

func (m *Model) prepareBlockForRender(index int, block transcriptBlock) transcriptBlock {
	if block.Kind != blockAssistant {
		return block
	}
	block.Body = m.prepareAssistantBodyForRender(block.Body, block.Markdown, m.shouldSuppressLiveTodoEcho(index))
	return block
}

func (m *Model) prepareAssistantBodyForRender(body string, markdown bool, suppressTodoEcho bool) string {
	body = sanitizeAssistantText(body)
	if markdown {
		body = strings.TrimSpace(body)
	} else {
		body = cleanTerminalText(body)
	}
	if suppressTodoEcho {
		body = stripTodoEchoLines(body, m.todos)
	}
	return strings.TrimSpace(body)
}

func (m *Model) shouldSuppressLiveTodoEcho(index int) bool {
	return m.running && len(m.todos) > 0 && index >= m.runStartBlock
}

func (m *Model) renderLiveTodoBlock(width int) string {
	if !m.running || m.approval != nil || len(m.todos) == 0 {
		return ""
	}
	return m.renderTodoChecklist(width)
}

func (m *Model) renderLiveChatRetryBlock(width int) string {
	if !m.running || m.approval != nil || m.chatRetry == nil {
		return ""
	}
	return m.renderChatRetryBlock(width)
}

func (m *Model) renderTodoChecklist(width int) string {
	width = max(12, width)
	lines := make([]string, 0, len(m.todos))
	for _, item := range m.todos {
		lines = append(lines, m.renderTodoLine(item, width))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderTodoLine(item runtimeevent.TodoItem, width int) string {
	marker := todoMarker(item.Status)
	style := m.todoStyle
	if item.Status == runtimeevent.TodoCompleted {
		style = m.todoDoneStyle
	}
	body := collapseHorizontalSpace(strings.TrimSpace(item.Text))
	if body == "" {
		body = "Untitled todo"
	}
	prefix := marker + " "
	continuation := strings.Repeat(" ", lipgloss.Width(prefix))
	lines := strings.Split(wrapText(body, max(8, width-lipgloss.Width(prefix))), "\n")
	if len(lines) == 0 {
		lines = []string{body}
	}
	lines[0] = prefix + lines[0]
	for i := 1; i < len(lines); i++ {
		lines[i] = continuation + lines[i]
	}
	return style.Render(strings.Join(lines, "\n"))
}

func todoMarker(status runtimeevent.TodoStatus) string {
	switch status {
	case runtimeevent.TodoCompleted:
		return "■"
	case runtimeevent.TodoInProgress:
		return "□"
	default:
		return "□"
	}
}

func (m *Model) renderChatRetryBlock(width int) string {
	if m.chatRetry == nil {
		return ""
	}
	retry := m.chatRetry
	title := fmt.Sprintf(
		"Retrying chat... %d/%d (%s · esc to interrupt)",
		retry.Attempt,
		retry.MaxRetries,
		formatRetryCountdown(time.Until(retry.Until)),
	)
	titleLine := m.retryDotStyle.Render("•") + " " + m.retryTitleStyle.Render(title)

	detail := strings.TrimSpace(retry.Message)
	if detail == "" {
		detail = strings.TrimSpace(retry.Error)
	}
	detail = collapseHorizontalSpace(detail)
	if detail == "" {
		return titleLine
	}
	prefix := "└ "
	continuation := strings.Repeat(" ", lipgloss.Width(prefix))
	lines := strings.Split(wrapText(detail, max(8, width-lipgloss.Width(prefix))), "\n")
	if len(lines) == 0 {
		lines = []string{detail}
	}
	lines[0] = prefix + lines[0]
	for i := 1; i < len(lines); i++ {
		lines[i] = continuation + lines[i]
	}
	return titleLine + "\n" + m.mutedStyle.Render(strings.Join(lines, "\n"))
}

func formatRetryCountdown(remaining time.Duration) string {
	if remaining <= 0 {
		return "0s"
	}
	if remaining < time.Second {
		return formatDuration(remaining.Milliseconds())
	}
	seconds := int((remaining + time.Second - time.Nanosecond) / time.Second)
	return fmt.Sprintf("%ds", seconds)
}

func (m *Model) desiredInputLines(innerWidth int, maxLines int) int {
	if maxLines <= 0 {
		maxLines = 1
	}
	if strings.TrimSpace(m.input.Value()) == "" {
		return 1
	}
	textWidth := max(1, innerWidth-lipgloss.Width(inputPrompt))
	lines := lipgloss.Height(wrapText(m.input.Value(), textWidth))
	if lines <= 0 {
		lines = 1
	}
	if lines > maxLines {
		return maxLines
	}
	return lines
}

func (m *Model) renderBlock(block transcriptBlock, width int) string {
	if strings.TrimSpace(block.Title) == "" && strings.TrimSpace(block.Body) == "" {
		return ""
	}
	switch block.Kind {
	case blockUser:
		return m.renderUserQuestionBlock(block.Body, width)
	case blockAssistant:
		return m.renderAssistantBodyBlock(block, width)
	case blockDiff:
		return m.renderDiffBlock(block, width)
	}

	title := block.Title
	var titleLine string
	switch block.Kind {
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

// User turns act as anchor points for the next assistant reply, so render them
// as a lightweight prompt marker instead of repeating role labels or large boxes.
func (m *Model) renderUserQuestionBlock(body string, width int) string {
	body = collapseHorizontalSpace(strings.TrimSpace(body))
	if body == "" {
		return ""
	}
	prefix := "* "
	continuation := strings.Repeat(" ", lipgloss.Width(prefix))
	lines := strings.Split(wrapText(body, max(8, width-lipgloss.Width(prefix))), "\n")
	if len(lines) == 0 {
		lines = []string{body}
	}
	lines[0] = m.userPromptMarkerStyle.Render("*") + " " + m.userPromptTextStyle.Render(lines[0])
	for i := 1; i < len(lines); i++ {
		lines[i] = continuation + m.userPromptTextStyle.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderAssistantBodyBlock(block transcriptBlock, width int) string {
	body := strings.TrimSpace(block.Body)
	if body == "" {
		return ""
	}
	if block.Markdown {
		renderer := m.markdownForWidth(max(20, width))
		rendered, err := renderMarkdown(renderer, body)
		if err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return m.assistantBodyStyle.Render(wrapText(body, max(20, width)))
}

// Diff blocks render unified diffs as editor-like inline rows so file edits are
// easier to scan than raw patch headers in the main transcript.
func (m *Model) renderDiffBlock(block transcriptBlock, width int) string {
	titleLine := m.titleStyle.Render(block.Title)
	body := strings.TrimRight(block.Body, "\n")
	if strings.TrimSpace(body) == "" {
		return titleLine
	}
	lineWidth := max(18, width)
	lines := strings.Split(body, "\n")
	rendered := make([]string, 0, len(lines))
	state := diffRenderState{highlighter: newDiffSyntaxHighlighter(body)}
	for _, line := range lines {
		renderedLine := m.renderDiffLine(line, lineWidth, &state)
		if renderedLine == "" {
			continue
		}
		rendered = append(rendered, renderedLine)
	}
	if len(rendered) == 0 {
		return titleLine
	}
	return titleLine + "\n" + strings.Join(rendered, "\n")
}

func (m *Model) renderDiffLine(line string, width int, state *diffRenderState) string {
	style, prefix, content := m.diffLineParts(line)
	switch {
	case line == "…":
		return renderDiffWrappedLine(style, "", content, width, false)
	case prefix == "+" || prefix == "-" || prefix == " ":
		return m.renderDiffBodyLine(style, prefix, content, width, state)
	case strings.HasPrefix(line, "@@"):
		if oldLine, newLine, ok := parseDiffHunkHeader(line); ok {
			state.oldLine = oldLine
			state.newLine = newLine
			state.hasHunk = true
			state.hunkCount++
			if state.hunkCount > 1 {
				return renderDiffWrappedLine(m.diffEllipsisStyle, "", "…", width, false)
			}
		}
		return ""
	case prefix != "":
		return ""
	default:
		return renderDiffWrappedLine(style, "", content, width, false)
	}
}

func (m *Model) diffLineParts(line string) (lipgloss.Style, string, string) {
	switch {
	case line == "…":
		return m.diffEllipsisStyle, "", line
	case strings.HasPrefix(line, "diff --git "):
		return m.diffMetaStyle, "diff --git ", strings.TrimPrefix(line, "diff --git ")
	case strings.HasPrefix(line, "index "):
		return m.diffMetaStyle, "index ", strings.TrimPrefix(line, "index ")
	case strings.HasPrefix(line, "new file mode "):
		return m.diffMetaStyle, "new file mode ", strings.TrimPrefix(line, "new file mode ")
	case strings.HasPrefix(line, "deleted file mode "):
		return m.diffMetaStyle, "deleted file mode ", strings.TrimPrefix(line, "deleted file mode ")
	case strings.HasPrefix(line, "old mode "):
		return m.diffMetaStyle, "old mode ", strings.TrimPrefix(line, "old mode ")
	case strings.HasPrefix(line, "new mode "):
		return m.diffMetaStyle, "new mode ", strings.TrimPrefix(line, "new mode ")
	case strings.HasPrefix(line, "similarity index "):
		return m.diffMetaStyle, "similarity index ", strings.TrimPrefix(line, "similarity index ")
	case strings.HasPrefix(line, "rename from "):
		return m.diffMetaStyle, "rename from ", strings.TrimPrefix(line, "rename from ")
	case strings.HasPrefix(line, "rename to "):
		return m.diffMetaStyle, "rename to ", strings.TrimPrefix(line, "rename to ")
	case strings.HasPrefix(line, "--- "):
		return m.diffMetaStyle, "--- ", strings.TrimPrefix(line, "--- ")
	case strings.HasPrefix(line, "+++ "):
		return m.diffMetaStyle, "+++ ", strings.TrimPrefix(line, "+++ ")
	case strings.HasPrefix(line, "@@ "):
		return m.diffMetaStyle, "@@ ", strings.TrimPrefix(line, "@@ ")
	case strings.HasPrefix(line, "@@"):
		return m.diffMetaStyle, "@@", strings.TrimPrefix(line, "@@")
	case strings.HasPrefix(line, "+"):
		return m.diffAddStyle, "+", line[1:]
	case strings.HasPrefix(line, "-"):
		return m.diffRemoveStyle, "-", line[1:]
	case strings.HasPrefix(line, " "):
		return m.diffContextStyle, " ", line[1:]
	default:
		return m.diffContextStyle, "", line
	}
}

func wrapDiffContent(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, 4)
	var current strings.Builder
	currentWidth := 0
	for _, r := range runes {
		runeText := string(r)
		runeWidth := lipgloss.Width(runeText)
		if runeWidth <= 0 {
			runeWidth = 1
		}
		if currentWidth+runeWidth > width && current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteString(runeText)
		currentWidth += runeWidth
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

const diffLineNumberWidth = 5

type diffRenderState struct {
	oldLine     int
	newLine     int
	hasHunk     bool
	hunkCount   int
	highlighter *diffSyntaxHighlighter
}

func (m *Model) renderDiffBodyLine(style lipgloss.Style, marker string, content string, width int, state *diffRenderState) string {
	lineNumber := state.consumeLineNumber(marker)
	prefix := diffLinePrefix(lineNumber, marker)
	spans := []diffStyledSpan{{Text: content, Style: style}}
	if state != nil && state.highlighter != nil {
		spans = state.highlighter.highlight(content, style)
	}
	return renderDiffWrappedSpans(style, prefix, spans, width, marker == "+" || marker == "-")
}

func renderDiffWrappedLine(style lipgloss.Style, prefix string, content string, width int, fillWidth bool) string {
	return renderDiffWrappedSpans(style, prefix, []diffStyledSpan{{Text: content, Style: style}}, width, fillWidth)
}

func renderDiffWrappedSpans(style lipgloss.Style, prefix string, spans []diffStyledSpan, width int, fillWidth bool) string {
	if prefix == "" {
		lines := wrapStyledDiffSpans(spans, max(8, width))
		return strings.Join(renderWrappedDiffLines(style, lines, "", width, fillWidth), "\n")
	}
	continuation := strings.Repeat(" ", lipgloss.Width(prefix))
	lines := wrapStyledDiffSpans(spans, max(4, width-lipgloss.Width(prefix)))
	return strings.Join(renderWrappedDiffLines(style, lines, prefix, width, fillWidth, continuation), "\n")
}

func (s *diffRenderState) consumeLineNumber(marker string) string {
	if s == nil || !s.hasHunk {
		return ""
	}
	switch marker {
	case " ":
		line := strconv.Itoa(s.newLine)
		s.oldLine++
		s.newLine++
		return line
	case "-":
		line := strconv.Itoa(s.oldLine)
		s.oldLine++
		return line
	case "+":
		line := strconv.Itoa(s.newLine)
		s.newLine++
		return line
	default:
		return ""
	}
}

func diffLinePrefix(lineNumber string, marker string) string {
	if marker == " " {
		return formatDiffLineNumber(lineNumber) + "   "
	}
	return formatDiffLineNumber(lineNumber) + " " + marker + " "
}

func formatDiffLineNumber(line string) string {
	if strings.TrimSpace(line) == "" {
		return strings.Repeat(" ", diffLineNumberWidth)
	}
	return fmt.Sprintf("%*s", diffLineNumberWidth, line)
}

func wrapStyledDiffSpans(spans []diffStyledSpan, width int) []diffWrappedLine {
	if width <= 0 {
		width = 1
	}
	if len(spans) == 0 {
		return []diffWrappedLine{{}}
	}

	lines := make([]diffWrappedLine, 0, 4)
	current := diffWrappedLine{segments: make([]string, 0, 16)}
	flush := func(force bool) {
		if !force && len(current.segments) == 0 && current.width == 0 {
			return
		}
		lines = append(lines, current)
		current = diffWrappedLine{segments: make([]string, 0, 16)}
	}

	for _, span := range spans {
		for _, r := range []rune(span.Text) {
			runeText := string(r)
			runeWidth := lipgloss.Width(runeText)
			if runeWidth <= 0 {
				runeWidth = 1
			}
			if current.width+runeWidth > width && current.width > 0 {
				flush(false)
			}
			current.segments = append(current.segments, span.Style.Render(runeText))
			current.width += runeWidth
		}
	}
	flush(len(lines) == 0 || len(current.segments) > 0 || current.width > 0)
	if len(lines) == 0 {
		return []diffWrappedLine{{}}
	}
	return lines
}

type diffWrappedLine struct {
	segments []string
	width    int
}

func renderWrappedDiffLines(style lipgloss.Style, lines []diffWrappedLine, prefix string, width int, fillWidth bool, continuation ...string) []string {
	if len(lines) == 0 {
		lines = []diffWrappedLine{{}}
	}
	rendered := make([]string, 0, len(lines))
	continuationPrefix := prefix
	if len(continuation) > 0 {
		continuationPrefix = continuation[0]
	}
	prefixStyle := diffInheritedTextStyle(style)
	paddingStyle := diffInheritedTextStyle(style).UnsetForeground()
	for i, line := range lines {
		linePrefix := prefix
		if i > 0 {
			linePrefix = continuationPrefix
		}
		renderedLine := prefixStyle.Render(linePrefix) + strings.Join(line.segments, "")
		usedWidth := lipgloss.Width(linePrefix) + line.width
		if fillWidth && usedWidth < width {
			renderedLine += paddingStyle.Render(strings.Repeat(" ", width-usedWidth))
		}
		rendered = append(rendered, renderedLine)
	}
	return rendered
}

// Unified diff hunk headers define the old/new starting lines for the body
// rows that follow, which lets the TUI annotate inline diffs with line numbers.
func parseDiffHunkHeader(line string) (int, int, bool) {
	if !strings.HasPrefix(line, "@@") {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(line, "@@")
	end := strings.Index(rest, "@@")
	if end < 0 {
		return 0, 0, false
	}
	fields := strings.Fields(strings.TrimSpace(rest[:end]))
	if len(fields) < 2 {
		return 0, 0, false
	}
	oldLine, ok := parseDiffRangeStart(fields[0], '-')
	if !ok {
		return 0, 0, false
	}
	newLine, ok := parseDiffRangeStart(fields[1], '+')
	if !ok {
		return 0, 0, false
	}
	return oldLine, newLine, true
}

func parseDiffRangeStart(field string, marker byte) (int, bool) {
	if len(field) < 2 || field[0] != marker {
		return 0, false
	}
	text := field[1:]
	if comma := strings.IndexByte(text, ','); comma >= 0 {
		text = text[:comma]
	}
	line, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return line, true
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
