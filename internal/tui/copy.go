package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	chansi "github.com/charmbracelet/x/ansi"
)

type copySelectionPoint struct {
	Line int
	Cell int
}

type copySelectionState struct {
	Anchor  copySelectionPoint
	Focus   copySelectionPoint
	Dragged bool
}

func buildRenderedTextBuffer(content string) renderedTextBuffer {
	if content == "" {
		return renderedTextBuffer{}
	}
	styledLines := strings.Split(content, "\n")
	plainLines := make([]string, len(styledLines))
	for i, line := range styledLines {
		plainLines[i] = chansi.Strip(line)
	}
	return renderedTextBuffer{
		StyledLines: styledLines,
		PlainLines:  plainLines,
	}
}

func (m *Model) updateMouse(msg tea.MouseMsg) tea.Cmd {
	m.clearCopyNotice()
	if cmd, handled := m.updateCopySelection(msg); handled {
		return cmd
	}
	return m.updateActiveViewport(msg)
}

func (m *Model) updateCopySelection(msg tea.MouseMsg) (tea.Cmd, bool) {
	if m.viewingSubagent {
		return nil, false
	}
	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		point, ok := m.mainViewportPointForMouse(msg, false)
		if !ok {
			m.clearCopySelection()
			return nil, false
		}
		m.copySelection = &copySelectionState{Anchor: point, Focus: point}
		return nil, true
	case msg.Action == tea.MouseActionMotion && m.copySelection != nil:
		point, ok := m.mainViewportPointForMouse(msg, true)
		if !ok {
			return nil, true
		}
		m.copySelection.Focus = point
		m.copySelection.Dragged = true
		return nil, true
	case msg.Action == tea.MouseActionRelease && m.copySelection != nil:
		point, ok := m.mainViewportPointForMouse(msg, true)
		if ok {
			m.copySelection.Focus = point
		}
		if !m.copySelection.Dragged || !m.hasCopySelection() {
			m.clearCopySelection()
			return nil, true
		}
		text := m.selectedViewportText()
		if strings.TrimSpace(text) == "" {
			m.setCopyNotice("未选中可复制文本", true)
			m.clearCopySelection()
			return nil, true
		}
		return copySelectionCmd(text, m.copyText), true
	default:
		return nil, false
	}
}

func (m *Model) mainViewportPointForMouse(msg tea.MouseMsg, clampToVisible bool) (copySelectionPoint, bool) {
	if len(m.mainViewportBuffer.PlainLines) == 0 || m.viewport.Height <= 0 {
		return copySelectionPoint{}, false
	}
	top := lipgloss.Height(m.renderHeader())
	if clampToVisible {
		y := msg.Y - top
		if y < 0 {
			y = 0
		}
		if y >= m.viewport.Height {
			y = m.viewport.Height - 1
		}
		line := m.viewport.YOffset + y
		if line < 0 {
			line = 0
		}
		if line >= len(m.mainViewportBuffer.PlainLines) {
			line = len(m.mainViewportBuffer.PlainLines) - 1
		}
		x := msg.X - contentLeftInset
		if x < 0 {
			x = 0
		}
		return copySelectionPoint{
			Line: line,
			Cell: cellIndexForColumn(m.mainViewportBuffer.PlainLines[line], x),
		}, true
	}
	if msg.Y < top || msg.Y >= top+m.viewport.Height {
		return copySelectionPoint{}, false
	}
	if msg.X < contentLeftInset || msg.X >= contentLeftInset+m.viewport.Width {
		return copySelectionPoint{}, false
	}
	line := m.viewport.YOffset + (msg.Y - top)
	if line < 0 || line >= len(m.mainViewportBuffer.PlainLines) {
		return copySelectionPoint{}, false
	}
	return copySelectionPoint{
		Line: line,
		Cell: cellIndexForColumn(m.mainViewportBuffer.PlainLines[line], msg.X-contentLeftInset),
	}, true
}

func (m *Model) hasCopySelection() bool {
	if m.copySelection == nil {
		return false
	}
	start, end := m.copySelection.normalized()
	return start.Line != end.Line || start.Cell != end.Cell
}

func (m *Model) clearCopySelection() {
	m.copySelection = nil
}

func (m *Model) setCopyNotice(text string, isError bool) {
	if m.copyNotice == text && m.copyNoticeError == isError {
		return
	}
	m.copyNotice = text
	m.copyNoticeError = isError
	m.markLayoutDirty()
}

func (m *Model) clearCopyNotice() {
	if m.copyNotice == "" && !m.copyNoticeError {
		return
	}
	m.copyNotice = ""
	m.copyNoticeError = false
	m.markLayoutDirty()
}

func (m *Model) selectedViewportText() string {
	if !m.hasCopySelection() {
		return ""
	}
	start, end := m.copySelection.normalized()
	lines := make([]string, 0, end.Line-start.Line+1)
	for lineIndex := start.Line; lineIndex <= end.Line; lineIndex++ {
		line := m.mainViewportBuffer.PlainLines[lineIndex]
		width := lineCellWidth(line)
		switch {
		case start.Line == end.Line:
			lines = append(lines, sliceTextByCells(line, start.Cell, end.Cell+1))
		case lineIndex == start.Line:
			lines = append(lines, sliceTextByCells(line, start.Cell, width))
		case lineIndex == end.Line:
			lines = append(lines, sliceTextByCells(line, 0, end.Cell+1))
		default:
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) selectedCellsForLine(lineIndex int) (int, int, bool) {
	if !m.hasCopySelection() || lineIndex < 0 || lineIndex >= len(m.mainViewportBuffer.PlainLines) {
		return 0, 0, false
	}
	start, end := m.copySelection.normalized()
	if lineIndex < start.Line || lineIndex > end.Line {
		return 0, 0, false
	}
	lineWidth := lineCellWidth(m.mainViewportBuffer.PlainLines[lineIndex])
	switch {
	case start.Line == end.Line:
		return clamp(start.Cell, 0, lineWidth), clamp(end.Cell+1, 0, lineWidth), start.Cell != end.Cell
	case lineIndex == start.Line:
		return clamp(start.Cell, 0, lineWidth), lineWidth, true
	case lineIndex == end.Line:
		return 0, clamp(end.Cell+1, 0, lineWidth), true
	default:
		return 0, lineWidth, true
	}
}

func (m *Model) renderSelectedViewport() string {
	if len(m.mainViewportBuffer.StyledLines) == 0 {
		return ""
	}
	start := max(0, m.viewport.YOffset)
	if start >= len(m.mainViewportBuffer.StyledLines) {
		return ""
	}
	end := min(len(m.mainViewportBuffer.StyledLines), start+m.viewport.Height)
	lines := make([]string, 0, end-start)
	for lineIndex := start; lineIndex < end; lineIndex++ {
		startCell, endCell, ok := m.selectedCellsForLine(lineIndex)
		if !ok {
			lines = append(lines, m.mainViewportBuffer.StyledLines[lineIndex])
			continue
		}
		plain := m.mainViewportBuffer.PlainLines[lineIndex]
		before := sliceTextByCells(plain, 0, startCell)
		selected := sliceTextByCells(plain, startCell, endCell)
		after := sliceTextByCells(plain, endCell, lineCellWidth(plain))
		if selected == "" {
			lines = append(lines, plain)
			continue
		}
		lines = append(lines, before+m.selectionStyle.Render(selected)+after)
	}
	return strings.Join(lines, "\n")
}

func (s *copySelectionState) normalized() (copySelectionPoint, copySelectionPoint) {
	start := s.Anchor
	end := s.Focus
	if end.Line < start.Line || (end.Line == start.Line && end.Cell < start.Cell) {
		start, end = end, start
	}
	return start, end
}

func cellIndexForColumn(text string, col int) int {
	if col <= 0 {
		return 0
	}
	totalWidth := lineCellWidth(text)
	if totalWidth <= 0 {
		return 0
	}
	if col >= totalWidth {
		return max(0, totalWidth-1)
	}
	cursor := 0
	lastStart := 0
	for _, r := range text {
		width := lipgloss.Width(string(r))
		if width <= 0 {
			width = 1
		}
		if col < cursor+width {
			return cursor
		}
		lastStart = cursor
		cursor += width
	}
	return lastStart
}

func lineCellWidth(text string) int {
	width := lipgloss.Width(text)
	if width < 0 {
		return 0
	}
	return width
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func sliceTextByCells(text string, startCell, endCell int) string {
	if endCell <= startCell {
		return ""
	}
	if startCell < 0 {
		startCell = 0
	}
	width := lineCellWidth(text)
	if endCell > width {
		endCell = width
	}
	if startCell >= endCell {
		return ""
	}
	var out strings.Builder
	cursor := 0
	for _, r := range text {
		runeText := string(r)
		runeWidth := lipgloss.Width(runeText)
		if runeWidth <= 0 {
			runeWidth = 1
		}
		next := cursor + runeWidth
		if next <= startCell {
			cursor = next
			continue
		}
		if cursor >= endCell {
			break
		}
		out.WriteString(runeText)
		cursor = next
	}
	return out.String()
}

func copySelectionCmd(text string, copyText func(string) error) tea.Cmd {
	return func() tea.Msg {
		err := errors.New("clipboard backend is not configured")
		if copyText != nil {
			err = copyText(text)
		}
		return copySelectionResultMsg{
			Text: text,
			Err:  err,
		}
	}
}

func writeClipboardText(text string) error {
	var errs []error
	if !clipboard.Unsupported {
		if err := clipboard.WriteAll(text); err == nil {
			return nil
		} else {
			errs = append(errs, err)
		}
	}

	sequence := osc52.New(text)
	switch {
	case os.Getenv("TMUX") != "":
		sequence = sequence.Tmux()
	case os.Getenv("STY") != "":
		sequence = sequence.Screen()
	}
	if _, err := fmt.Fprint(os.Stderr, sequence.String()); err == nil {
		return nil
	} else {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
