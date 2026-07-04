package tui

import (
	"fmt"
	"strings"

	"local-agent/internal/session"
)

func (m *Model) openResumePicker() (bool, string) {
	if m.resumeList == nil {
		return false, ""
	}
	sessions, err := m.resumeList()
	if err != nil {
		return true, err.Error()
	}
	if len(sessions) == 0 {
		return true, "no saved sessions for the current workspace"
	}
	m.resumePicker = &resumePickerState{
		Sessions: sessions,
	}
	m.markViewportDirty()
	return true, ""
}

func (m *Model) cancelResumePicker() {
	m.resumePicker = nil
	m.markViewportDirty()
}

func (m *Model) moveResumeSelection(delta int) {
	if m.resumePicker == nil || len(m.resumePicker.Sessions) == 0 {
		return
	}
	count := len(m.resumePicker.Sessions)
	m.resumePicker.Selected = (m.resumePicker.Selected + delta + count) % count
	m.markViewportDirty()
}

func (m *Model) confirmResumeSelection() string {
	if m.resumePicker == nil || len(m.resumePicker.Sessions) == 0 {
		return ""
	}
	if m.resumeSelect == nil {
		m.resumePicker = nil
		return "resume selection is unavailable"
	}
	selected := m.resumePicker.Sessions[m.resumePicker.Selected]
	m.resumePicker = nil
	m.markViewportDirty()
	output, err := m.resumeSelect(selected.SessionID)
	if err != nil {
		if strings.TrimSpace(output) != "" {
			return output + "\n" + err.Error()
		}
		return err.Error()
	}
	return output
}

func (m *Model) renderResumePicker(width int) string {
	if m.resumePicker == nil || len(m.resumePicker.Sessions) == 0 {
		return ""
	}
	lines := []string{m.titleStyle.Render("Resume Session")}
	for i, meta := range m.resumePicker.Sessions {
		lines = append(lines, m.renderResumeSessionLine(i, meta, width)...)
	}
	hint := "Up/Down or J/K choose  •  Enter resume  •  Esc cancel"
	lines = append(lines, m.approvalHintStyle.Render(hint))
	return strings.Join(lines, "\n")
}

func (m *Model) renderResumeSessionLine(index int, meta session.Meta, width int) []string {
	available := max(20, width)
	selected := m.resumePicker != nil && index == m.resumePicker.Selected
	style := m.mutedStyle
	if selected {
		style = m.approvalSelectedStyle
	}
	header := renderResumePickerHeader(selected, meta.SessionID, meta.UpdatedAt.UTC().Format("2006-01-02 15:04:05Z"))
	lines := []string{style.Render(truncateSingleLine(header, available))}
	summary := strings.TrimSpace(meta.Title)
	if summary == "" {
		summary = strings.TrimSpace(meta.LastUserPreview)
	}
	if summary == "" {
		return lines
	}
	wrapped := strings.Split(wrapText(summary, max(12, available-4)), "\n")
	for _, line := range wrapped {
		lines = append(lines, "    "+style.Render(line))
	}
	return lines
}

func (m *Model) resumePickerActive() bool {
	return m.resumePicker != nil
}

func (m *Model) hideResumePicker() {
	m.resumePicker = nil
	m.markViewportDirty()
}

func renderResumePickerHeader(selected bool, id string, updated string) string {
	prefix := "  "
	if selected {
		prefix = "› "
	}
	return fmt.Sprintf("%s%s  %s", prefix, id, updated)
}
