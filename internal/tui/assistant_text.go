package tui

import (
	"strings"
	"unicode"

	"local-agent/internal/runtimeevent"
)

func sanitizeAssistantText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return stripThinkSections(text)
}

func stripThinkSections(text string) string {
	lower := strings.ToLower(text)
	var out strings.Builder
	for i := 0; i < len(text); {
		open := strings.Index(lower[i:], "<think>")
		close := strings.Index(lower[i:], "</think>")
		if open < 0 && close < 0 {
			out.WriteString(text[i:])
			break
		}

		next := len(text)
		tag := ""
		if open >= 0 {
			next = i + open
			tag = "<think>"
		}
		if close >= 0 && i+close < next {
			next = i + close
			tag = "</think>"
		}

		out.WriteString(text[i:next])
		switch tag {
		case "</think>":
			i = next + len("</think>")
		case "<think>":
			afterOpen := next + len("<think>")
			close = strings.Index(lower[afterOpen:], "</think>")
			if close < 0 {
				return out.String()
			}
			i = afterOpen + close + len("</think>")
		default:
			i = next
		}
	}
	return out.String()
}

func stripTodoEchoLines(text string, todos []runtimeevent.TodoItem) string {
	if strings.TrimSpace(text) == "" || len(todos) == 0 {
		return strings.TrimSpace(text)
	}

	todoTexts := make(map[string]struct{}, len(todos))
	for _, todo := range todos {
		normalized := normalizeTodoEchoText(todo.Text)
		if normalized == "" {
			continue
		}
		todoTexts[normalized] = struct{}{}
	}
	if len(todoTexts) == 0 {
		return strings.TrimSpace(text)
	}

	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if candidate, ok := extractChecklistEchoText(line); ok {
			if _, exists := todoTexts[normalizeTodoEchoText(candidate)]; exists {
				removed = true
				continue
			}
		}
		kept = append(kept, line)
	}
	if !removed {
		return strings.TrimSpace(text)
	}

	var out []string
	blank := false
	for _, line := range kept {
		if strings.TrimSpace(line) == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, strings.TrimRight(line, " \t"))
		blank = false
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func normalizeTodoEchoText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "`", "")
	return collapseHorizontalSpace(text)
}

func extractChecklistEchoText(line string) (string, bool) {
	text := strings.TrimSpace(line)
	if text == "" {
		return "", false
	}

	hadMarker := false
	if trimmed, ok := trimChecklistBullet(text); ok {
		text = trimmed
		hadMarker = true
	}
	if trimmed, ok := trimCheckboxMarker(text); ok {
		text = trimmed
		hadMarker = true
	}
	if trimmed, ok := trimUnicodeCheckbox(text); ok {
		text = trimmed
		hadMarker = true
	}

	if !hadMarker {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func trimChecklistBullet(text string) (string, bool) {
	if text == "" {
		return text, false
	}
	switch text[0] {
	case '-', '*', '+':
		if len(text) > 1 && unicode.IsSpace(rune(text[1])) {
			return strings.TrimSpace(text[1:]), true
		}
	}
	return text, false
}

func trimCheckboxMarker(text string) (string, bool) {
	if !strings.HasPrefix(text, "[") {
		return text, false
	}
	end := strings.Index(text, "]")
	if end <= 0 || end > 3 {
		return text, false
	}
	return strings.TrimSpace(text[end+1:]), true
}

func trimUnicodeCheckbox(text string) (string, bool) {
	if text == "" {
		return text, false
	}
	switch []rune(text)[0] {
	case '□', '■', '☐', '☑', '✓', '✔':
		runes := []rune(text)
		return strings.TrimSpace(string(runes[1:])), true
	default:
		return text, false
	}
}
