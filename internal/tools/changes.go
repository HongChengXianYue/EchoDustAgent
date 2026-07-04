package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
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

func fileChangeFromText(path string, oldText string, newText string, action string, previewLines int) (FileChange, bool) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return FileChange{}, false
	}
	diffText := unifiedDiffForText(path, oldText, newText, action)
	if strings.TrimSpace(diffText) == "" {
		return FileChange{}, false
	}
	added, removed := unifiedDiffLineTotals(diffText)
	if action == "" {
		action = inferDiffAction(added, removed)
	}
	return FileChange{
		Path:         path,
		Action:       action,
		AddedLines:   added,
		RemovedLines: removed,
		Diff:         diffText,
		Preview:      trimUnifiedDiff(diffText, previewLines),
	}, true
}

func unifiedDiffForText(path string, oldText string, newText string, action string) string {
	oldName, newName := diffNames(path, action)
	edits := myers.ComputeEdits(span.URIFromPath(path), oldText, newText)
	return normalizeDiffText(fmt.Sprint(gotextdiff.ToUnified(oldName, newName, oldText, edits)))
}

func diffNames(path string, action string) (string, string) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	switch strings.ToLower(action) {
	case "added", "add":
		return "/dev/null", "b/" + path
	case "deleted", "delete":
		return "a/" + path, "/dev/null"
	default:
		return "a/" + path, "b/" + path
	}
}

func unifiedDiffLineTotals(diffText string) (int, int) {
	added := 0
	removed := 0
	for _, line := range diffLines(diffText) {
		switch {
		case isDiffAddedLine(line):
			added++
		case isDiffRemovedLine(line):
			removed++
		}
	}
	return added, removed
}

// trimUnifiedDiff keeps diff headers and hunk markers visible while limiting
// the number of body lines so transcript previews stay readable.
func trimUnifiedDiff(diffText string, maxBodyLines int) string {
	if maxBodyLines <= 0 {
		maxBodyLines = DefaultOptions().FileChangePreviewLines
	}
	lines := diffLines(diffText)
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	bodyLines := 0
	truncated := false
	for _, line := range lines {
		if isDiffMetaLine(line) {
			if maxBodyLines > 0 && bodyLines >= maxBodyLines {
				break
			}
			out = append(out, line)
			continue
		}
		if maxBodyLines > 0 && bodyLines >= maxBodyLines {
			truncated = true
			break
		}
		out = append(out, line)
		bodyLines++
	}
	if truncated {
		out = append(out, "…")
	}
	return strings.Join(out, "\n")
}

func parseUnifiedDiffChanges(patchText string, previewLines int) []FileChange {
	if previewLines <= 0 {
		previewLines = DefaultOptions().FileChangePreviewLines
	}
	lines := diffLines(patchText)
	if len(lines) == 0 {
		return nil
	}

	out := make([]FileChange, 0, 4)
	var pending []string
	var current *trackedChange
	flush := func() {
		if current == nil {
			return
		}
		if change, ok := current.fileChange(previewLines); ok {
			out = append(out, change)
		}
		current = nil
	}

	for _, line := range lines {
		switch {
		case isDiffPreludeLine(line):
			if current != nil && (current.oldPath != "" || current.newPath != "") {
				flush()
			}
			pending = append(pending, line)
		case strings.HasPrefix(line, "--- "):
			flush()
			current = &trackedChange{
				oldPath: firstPatchPath(line[4:]),
				lines:   append([]string{}, pending...),
			}
			pending = nil
			current.lines = append(current.lines, line)
		case strings.HasPrefix(line, "+++ "):
			if current == nil {
				current = &trackedChange{lines: append([]string{}, pending...)}
				pending = nil
			}
			current.newPath = firstPatchPath(line[4:])
			current.lines = append(current.lines, line)
		default:
			if current != nil {
				current.lines = append(current.lines, line)
			}
		}
	}
	flush()
	return out
}

func diffLines(text string) []string {
	text = normalizeDiffText(text)
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

type trackedChange struct {
	oldPath string
	newPath string
	lines   []string
}

func (c *trackedChange) fileChange(previewLines int) (FileChange, bool) {
	if c == nil {
		return FileChange{}, false
	}
	path := cleanDiffPath(c.newPath)
	if path == "" {
		path = cleanDiffPath(c.oldPath)
	}
	if path == "" {
		return FileChange{}, false
	}
	diffText := normalizeDiffText(strings.Join(c.lines, "\n"))
	if diffText == "" {
		return FileChange{}, false
	}
	added, removed := unifiedDiffLineTotals(diffText)
	action := "edited"
	switch {
	case c.oldPath == "/dev/null":
		action = "added"
	case c.newPath == "/dev/null":
		action = "deleted"
	case removed == 0 && added > 0:
		action = "added"
	}
	return FileChange{
		Path:         path,
		Action:       action,
		AddedLines:   added,
		RemovedLines: removed,
		Diff:         diffText,
		Preview:      trimUnifiedDiff(diffText, previewLines),
	}, true
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

func normalizeDiffText(text string) string {
	return strings.TrimRight(text, "\n")
}

func inferDiffAction(added int, removed int) string {
	switch {
	case removed == 0 && added > 0:
		return "added"
	case added == 0 && removed > 0:
		return "deleted"
	default:
		return "edited"
	}
}

func isDiffAddedLine(line string) bool {
	return strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++ ")
}

func isDiffRemovedLine(line string) bool {
	return strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "--- ")
}

func isDiffMetaLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "--- "),
		strings.HasPrefix(line, "+++ "),
		strings.HasPrefix(line, "@@"),
		isDiffPreludeLine(line):
		return true
	default:
		return false
	}
}

func isDiffPreludeLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "diff --git "),
		strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "new file mode "),
		strings.HasPrefix(line, "deleted file mode "),
		strings.HasPrefix(line, "old mode "),
		strings.HasPrefix(line, "new mode "),
		strings.HasPrefix(line, "similarity index "),
		strings.HasPrefix(line, "rename from "),
		strings.HasPrefix(line, "rename to "):
		return true
	default:
		return false
	}
}
