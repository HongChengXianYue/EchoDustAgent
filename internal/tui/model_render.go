package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/ui"
)

func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "loading..."
	}
	m.syncLayout()

	parts := []string{m.renderHeader(), m.renderContent()}
	if panel := m.renderSubagentPanel(); panel != "" {
		parts = append(parts, panel)
	}
	if footer := m.renderFooter(); footer != "" {
		parts = append(parts, footer)
	}
	if suggestions := m.renderSuggestions(); suggestions != "" {
		parts = append(parts, suggestions)
	}
	parts = append(parts, m.renderInputBox())
	return strings.Join(parts, "\n")
}

func (m *Model) renderHeader() string {
	if m.shouldHideHeader() {
		return ""
	}
	return m.renderBanner()
}

func (m *Model) renderBanner() string {
	bannerWidth := 0
	for _, line := range tuiBannerLines {
		if width := lipgloss.Width(line); width > bannerWidth {
			bannerWidth = width
		}
	}
	if m.width <= 0 || m.width < bannerWidth+4 {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, m.bannerStyle.Render("ECHO DUST CODE"))
	}

	lines := make([]string, 0, len(tuiBannerLines))
	for i, line := range tuiBannerLines {
		style := m.bannerStyle
		if i%2 == 1 {
			style = m.bannerAltStyle
		}
		lines = append(lines, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, style.Render(line)))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) shouldHideHeader() bool {
	return len(m.blocks) > 0 ||
		strings.TrimSpace(m.assistantDraft) != "" ||
		m.running ||
		m.resumePickerActive() ||
		m.approval != nil ||
		len(m.todos) > 0 ||
		m.showSubagents
}

func (m *Model) renderContent() string {
	content := m.viewport.View()
	if m.hasCopySelection() {
		content = m.renderSelectedViewport()
	}
	if content != "" {
		content = indentBlock(content, strings.Repeat(" ", contentLeftInset))
	}
	style := m.contentStyle.
		Width(max(20, m.width-m.contentStyle.GetHorizontalFrameSize())).
		Height(m.viewport.Height)
	return style.Render(content)
}

func (m *Model) renderSubagentPanel() string {
	if !m.showSubagents || len(m.subagentOrder) == 0 || m.subagentHeight <= 0 {
		return ""
	}
	if m.viewingSubagent {
		return m.renderSubagentDetail()
	}
	return m.renderSubagentList()
}

func (m *Model) renderInputBox() string {
	width := max(10, m.width-m.inputBoxStyle.GetHorizontalFrameSize())
	return m.inputBoxStyle.Width(width).Render(m.input.View())
}

func (m *Model) renderFooter() string {
	width := max(20, m.width)
	left := ""
	if m.copyNotice != "" {
		if m.copyNoticeError {
			left = m.errorStyle.Render(m.copyNotice)
		} else {
			left = m.titleStyle.Render(m.copyNotice)
		}
	} else if m.shouldRenderLiveRunTimer() {
		left = "Total · " + formatDuration(m.runElapsedMS)
	}
	rightLimit := max(12, width-2)
	if left != "" {
		rightLimit = max(12, width-lipgloss.Width(left)-3)
	}
	right := m.footerSummary(rightLimit)
	if left == "" && right == "" {
		return ""
	}
	if left == "" {
		return lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Right).
			Render(m.mutedStyle.Render(right))
	}
	if right == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		combined := truncateSingleLine(left+" "+right, width)
		return combined
	}
	return left + strings.Repeat(" ", gap) + m.mutedStyle.Render(right)
}

func (m *Model) shouldRenderLiveRunTimer() bool {
	return m.running
}

func (m *Model) shouldRenderStatusBar(limit int) bool {
	return m.copyNotice != "" || m.shouldRenderLiveRunTimer() || m.footerSummary(limit) != ""
}

func (m *Model) renderSuggestions() string {
	if m.resumePickerActive() {
		return ""
	}
	matches := m.matchedSlashCommands()
	if len(matches) == 0 {
		return ""
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	m.clampSlashSuggestion()
	selected := m.slashSuggest
	var lines []string
	for i, match := range matches {
		prefix := "  "
		if i == selected {
			prefix = "› "
		}
		line := fmt.Sprintf("%s/%-10s %s", prefix, match.Name, match.Desc)
		if i == selected {
			lines = append(lines, m.titleStyle.Render(line))
		} else {
			lines = append(lines, m.mutedStyle.Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) matchedSlashCommands() []ui.CommandSuggestion {
	line := strings.SplitN(m.input.Value(), "\n", 2)[0]
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return nil
	}
	if strings.ContainsAny(line, " \t") {
		return nil
	}
	if len(m.slashCommands) == 0 {
		return nil
	}
	if line == "/" {
		return append([]ui.CommandSuggestion(nil), m.slashCommands...)
	}
	matches := make([]ui.CommandSuggestion, 0, len(m.slashCommands))
	for _, command := range m.slashCommands {
		name := "/" + command.Name
		if strings.HasPrefix(name, line) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (m *Model) todoSummary() string {
	if len(m.todos) == 0 {
		return "0/0"
	}
	completed := 0
	current := ""
	for _, todo := range m.todos {
		if todo.Status == runtimeevent.TodoCompleted {
			completed++
		}
		if todo.Status == runtimeevent.TodoInProgress {
			current = todo.Text
		}
	}
	summary := fmt.Sprintf("%d/%d", completed, len(m.todos))
	if strings.TrimSpace(current) != "" {
		return summary + " " + truncateSingleLine(current, 48)
	}
	return summary
}

func (m *Model) tokenSummary() string {
	if m.tokens.Total <= 0 {
		return "-"
	}
	summary := fmt.Sprintf(
		"%s (p%s c%s",
		formatCompactTokenCount(m.tokens.Total),
		formatCompactTokenCount(m.tokens.Prompt),
		formatCompactTokenCount(m.tokens.Completion),
	)
	if m.tokens.Cached > 0 {
		summary += ", cache " + formatCompactTokenCount(m.tokens.Cached)
		if hitRate, ok := formatCacheHitRate(m.tokens.Cached, m.tokens.Prompt); ok {
			summary += ", hit " + hitRate
		}
	}
	return summary + ")"
}

func formatCompactTokenCount(count int) string {
	if count >= 1_000_000 {
		return fmt.Sprintf("%.1fm", float64(count)/1_000_000)
	}
	if count >= 1000 {
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	return fmt.Sprintf("%d", count)
}

func (m *Model) subagentTokenTotal() int {
	total := 0
	for _, index := range m.subagentOrder {
		session := m.subagents[index]
		if session == nil || session.TokenTotal <= 0 {
			continue
		}
		total += session.TokenTotal
	}
	return total
}

func (m *Model) subagentPromptTokenTotal() int {
	total := 0
	for _, index := range m.subagentOrder {
		session := m.subagents[index]
		if session == nil || session.Prompt <= 0 {
			continue
		}
		total += session.Prompt
	}
	return total
}

func (m *Model) subagentCachedTokenTotal() int {
	total := 0
	for _, index := range m.subagentOrder {
		session := m.subagents[index]
		if session == nil || session.Cached <= 0 {
			continue
		}
		total += session.Cached
	}
	return total
}

func (m *Model) footerSummary(limit int) string {
	mainTotal := m.tokens.Total
	subTotal := m.subagentTokenTotal()
	totalPrompt := m.tokens.Prompt + m.subagentPromptTokenTotal()
	totalCached := m.tokens.Cached + m.subagentCachedTokenTotal()
	if mainTotal <= 0 && subTotal <= 0 {
		return ""
	}

	var candidates []string
	switch {
	case mainTotal > 0 && subTotal > 0:
		total := mainTotal + subTotal
		if totalCached > 0 {
			cacheSummary := "cache " + formatCompactTokenCount(totalCached)
			if hitRate, ok := formatCacheHitRate(totalCached, totalPrompt); ok {
				cacheSummary += " | hit " + hitRate
			}
			candidates = append(candidates,
				fmt.Sprintf(
					"Tokens %s total | main %s | sub %s | %s",
					formatCompactTokenCount(total),
					formatCompactTokenCount(mainTotal),
					formatCompactTokenCount(subTotal),
					cacheSummary,
				),
				fmt.Sprintf("Tokens %s total | %s", formatCompactTokenCount(total), cacheSummary),
			)
		}
		candidates = append(candidates,
			fmt.Sprintf(
				"Tokens %s total | main %s | sub %s",
				formatCompactTokenCount(total),
				formatCompactTokenCount(mainTotal),
				formatCompactTokenCount(subTotal),
			),
			fmt.Sprintf("Tokens %s total", formatCompactTokenCount(total)),
			fmt.Sprintf("Tokens %s", formatCompactTokenCount(total)),
		)
	case mainTotal > 0:
		candidates = append(candidates, "Tokens "+m.tokenSummary())
		if totalCached > 0 {
			cacheSummary := "cache " + formatCompactTokenCount(totalCached)
			if hitRate, ok := formatCacheHitRate(totalCached, totalPrompt); ok {
				cacheSummary += " | hit " + hitRate
			}
			candidates = append(candidates, fmt.Sprintf("Tokens %s | %s", formatCompactTokenCount(mainTotal), cacheSummary))
		}
		candidates = append(candidates, fmt.Sprintf("Tokens %s", formatCompactTokenCount(mainTotal)))
	default:
		if totalCached > 0 {
			cacheSummary := "cache " + formatCompactTokenCount(totalCached)
			if hitRate, ok := formatCacheHitRate(totalCached, totalPrompt); ok {
				cacheSummary += " | hit " + hitRate
			}
			candidates = append(candidates, fmt.Sprintf("Tokens %s subagents | %s", formatCompactTokenCount(subTotal), cacheSummary))
		}
		candidates = append(candidates,
			fmt.Sprintf("Tokens %s subagents", formatCompactTokenCount(subTotal)),
			fmt.Sprintf("Tokens %s", formatCompactTokenCount(subTotal)),
		)
	}

	for _, candidate := range candidates {
		if lipgloss.Width(candidate) <= limit {
			return candidate
		}
	}
	return truncateSingleLine(candidates[len(candidates)-1], limit)
}

func formatCacheHitRate(cached, prompt int) (string, bool) {
	if cached <= 0 || prompt <= 0 {
		return "", false
	}
	// Cache reuse only applies to prompt-side tokens. Using total tokens here
	// would understate long answers and skew parent/subagent comparisons.
	return fmt.Sprintf("%.1f%%", (float64(cached)/float64(prompt))*100), true
}

func truncatePath(path string, limit int) string {
	if limit <= 0 || len(path) <= limit {
		return path
	}
	if limit <= 3 {
		return path[:limit]
	}
	return "..." + path[len(path)-limit+3:]
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func approvalOptionLabel(request approval.Request, decision approval.Decision) string {
	switch decision {
	case approval.DecisionAllow:
		return "Allow once"
	case approval.DecisionAlways:
		if request.Scope == approval.ScopeSession && request.Key == "workspace_write" {
			return "Always allow workspace writes this session"
		}
		if request.Scope == approval.ScopeLoop {
			return "Always allow this loop"
		}
		return "Always allow exact call"
	case approval.DecisionDeny:
		return "Deny"
	default:
		return string(decision)
	}
}
