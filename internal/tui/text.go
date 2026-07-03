package tui

import (
	"strings"

	"github.com/muesli/reflow/wordwrap"
)

func cleanTerminalText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripDecorativeRunes(text)

	var out []string
	blank := false
	inFence := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			if isMarkdownRule(trimmed) || isMarkdownTableSeparator(trimmed) {
				continue
			}
			line = cleanMarkdownLine(line)
		}

		if strings.TrimSpace(line) == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, line)
		blank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func cleanMarkdownLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "#")
	for strings.HasPrefix(line, "#") {
		line = strings.TrimPrefix(line, "#")
	}
	line = strings.TrimSpace(line)
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "`", "")
	if isMarkdownTableRow(line) {
		line = formatMarkdownTableRow(line)
	}
	return collapseHorizontalSpace(line)
}

func isMarkdownRule(line string) bool {
	if len(line) < 3 {
		return false
	}
	for _, r := range line {
		if r != '-' && r != '_' && r != '*' && r != ' ' {
			return false
		}
	}
	return true
}

func isMarkdownTableRow(line string) bool {
	return strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && strings.Count(line, "|") >= 2
}

func isMarkdownTableSeparator(line string) bool {
	if !isMarkdownTableRow(line) {
		return false
	}
	for _, r := range strings.Trim(line, "| ") {
		if r != '-' && r != ':' && r != '|' && r != ' ' {
			return false
		}
	}
	return true
}

func formatMarkdownTableRow(line string) string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	switch len(cells) {
	case 0:
		return ""
	case 1:
		return cells[0]
	case 2:
		return cells[0] + ": " + cells[1]
	default:
		return strings.Join(cells, "  ")
	}
}

func stripDecorativeRunes(text string) string {
	var b strings.Builder
	for _, r := range text {
		if isDecorativeRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func collapseHorizontalSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func isDecorativeRune(r rune) bool {
	switch {
	case r == 0xfe0f:
		return true
	case r >= 0x1f000 && r <= 0x1faff:
		return true
	case r >= 0x2600 && r <= 0x27bf:
		return true
	default:
		return false
	}
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = wordwrap.String(line, width)
	}
	return strings.Join(lines, "\n")
}

func indentBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
