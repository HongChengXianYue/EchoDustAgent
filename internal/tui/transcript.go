package tui

import (
	"bytes"
	"encoding/json"
	"strings"
)

type transcriptKind int

const (
	blockInfo transcriptKind = iota
	blockUser
	blockAssistant
	blockError
	blockToolCall
	blockApprovalRequest
)

type transcriptBlock struct {
	Kind     transcriptKind
	Title    string
	Body     string
	Markdown bool
}

func compactJSON(raw json.RawMessage, limit int) string {
	if len(raw) == 0 {
		return "{}"
	}
	text := strings.TrimSpace(string(raw))
	var buf bytes.Buffer
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

func truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "\n... truncated"
}

func truncateSingleLine(text string, limit int) string {
	text = collapseHorizontalSpace(strings.ReplaceAll(strings.TrimSpace(text), "\n", " "))
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
