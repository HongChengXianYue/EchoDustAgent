package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"local-agent/internal/tools"
)

func compactJSON(raw json.RawMessage, limit int) string {
	if len(raw) == 0 {
		return "{}"
	}

	var buf bytes.Buffer
	text := strings.TrimSpace(string(raw))
	if err := json.Compact(&buf, raw); err == nil {
		text = buf.String()
	}
	return truncate(text, limit)
}

func jsonArgString(raw json.RawMessage, key string) string {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return ""
	}
	value, ok := args[key].(string)
	if !ok {
		return ""
	}
	return value
}

func jsonArgInt(raw json.RawMessage, key string) int {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return 0
	}
	value, ok := args[key].(float64)
	if !ok {
		return 0
	}
	return int(value)
}

func printIndented(output io.Writer, prefix string, text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(output, prefix+"(no output)")
		return
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintln(output, prefix+line)
			continue
		}
		fmt.Fprintln(output, strings.Repeat(" ", utf8.RuneCountInString(prefix))+line)
	}
}

func separatorLine(width int) string {
	if width <= 0 {
		width = DefaultOptions().SeparatorWidth
	}
	return strings.Repeat("─", width)
}

func limitTerminalText(text string, maxLines int, maxWidth int) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		keep := maxLines - 1
		if keep < 0 {
			keep = 0
		}
		lines = append(lines[:keep], "… truncated")
	}

	if maxWidth > 0 {
		for i, line := range lines {
			lines[i] = truncateDisplayLine(line, maxWidth)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func truncateDisplayLine(line string, maxWidth int) string {
	if maxWidth <= 0 {
		return line
	}

	var out strings.Builder
	width := 0
	truncated := false
	containsANSI := false
	visibleLimit := maxWidth
	if visibleLimit > 1 {
		visibleLimit--
	}

	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			next := copyANSISequence(&out, line, i)
			if next > i {
				containsANSI = true
				i = next
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(line[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		cellWidth := runeCellWidth(r)
		if width+cellWidth > visibleLimit {
			truncated = true
			break
		}
		out.WriteRune(r)
		width += cellWidth
		i += size
	}

	if !truncated {
		return line
	}
	out.WriteRune('…')
	if containsANSI {
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

func stripANSI(text string) string {
	if text == "" {
		return ""
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\x1b' {
			next := copyANSISequence(nil, text, i)
			if next > i {
				i = next
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

func copyANSISequence(out *strings.Builder, text string, start int) int {
	if start+1 >= len(text) || text[start+1] != '[' {
		return start
	}
	for i := start + 2; i < len(text); i++ {
		b := text[i]
		if b >= 0x40 && b <= 0x7e {
			if out != nil {
				out.WriteString(text[start : i+1])
			}
			return i + 1
		}
	}
	return start
}

func truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "\n… truncated"
}

func changeTotals(changes []tools.FileChange) (int, int) {
	added := 0
	removed := 0
	for _, change := range changes {
		added += change.AddedLines
		removed += change.RemovedLines
	}
	return added, removed
}

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
