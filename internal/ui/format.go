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

func separatorLine() string {
	return strings.Repeat("─", 80)
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
