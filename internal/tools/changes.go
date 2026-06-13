package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

func countLines(text string) int {
	if text == "" {
		return 0
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func addedContentPreview(content string, maxLines int) string {
	return prefixedLines(content, "+", maxLines)
}

func replacementPreview(oldText string, newText string, maxLines int) string {
	oldPreview := prefixedLines(oldText, "-", maxLines)
	newPreview := prefixedLines(newText, "+", maxLines)
	switch {
	case oldPreview == "":
		return newPreview
	case newPreview == "":
		return oldPreview
	default:
		return oldPreview + "\n" + newPreview
	}
}

func prefixedLines(text string, prefix string, maxLines int) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "…")
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if line == "…" {
			out = append(out, line)
			continue
		}
		out = append(out, fmt.Sprintf("%5d %s%s", i+1, prefix, line))
	}
	return strings.Join(out, "\n")
}

func parseUnifiedDiffChanges(patchText string) []FileChange {
	changes := map[string]*trackedChange{}
	order := []string{}
	var current *trackedChange

	ensureChange := func(path string) *trackedChange {
		path = cleanDiffPath(path)
		if path == "" {
			return nil
		}
		if existing, ok := changes[path]; ok {
			return existing
		}
		change := &trackedChange{change: FileChange{Path: path, Action: "edited"}}
		changes[path] = change
		order = append(order, path)
		return change
	}

	for _, line := range strings.Split(patchText, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			path := firstPatchPath(line[4:])
			if path == "/dev/null" {
				current = nil
				continue
			}
			current = ensureChange(path)
		case strings.HasPrefix(line, "+++ "):
			path := firstPatchPath(line[4:])
			if path == "/dev/null" {
				if current != nil {
					current.change.Action = "deleted"
				}
				continue
			}
			current = ensureChange(path)
		case strings.HasPrefix(line, "+"):
			if current == nil {
				continue
			}
			current.change.AddedLines++
			appendPatchPreview(current, line, 20)
		case strings.HasPrefix(line, "-"):
			if current == nil {
				continue
			}
			current.change.RemovedLines++
			appendPatchPreview(current, line, 20)
		}
	}

	out := make([]FileChange, 0, len(order))
	for _, path := range order {
		change := changes[path]
		if change.change.RemovedLines == 0 && change.change.AddedLines > 0 {
			change.change.Action = "added"
		}
		change.change.Preview = strings.Join(change.previewLines, "\n")
		out = append(out, change.change)
	}
	return out
}

func appendPatchPreview(change *trackedChange, line string, maxLines int) {
	if len(change.previewLines) >= maxLines {
		if len(change.previewLines) == maxLines {
			change.previewLines = append(change.previewLines, "…")
		}
		return
	}
	change.previewLines = append(change.previewLines, line)
}

type trackedChange struct {
	change       FileChange
	previewLines []string
}

func firstPatchPath(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func cleanDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		path = path[2:]
	}
	return filepath.ToSlash(path)
}
