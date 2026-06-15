package ui

import (
	"fmt"
	"strings"

	"local-agent/internal/tools"
)

func fullFileChangeDetail(result tools.Result) string {
	if len(result.Changes) == 0 {
		if strings.TrimSpace(result.Output) != "" {
			return result.Output
		}
		return result.Summary
	}
	var out strings.Builder
	for _, change := range result.Changes {
		if len(result.Changes) > 1 {
			fmt.Fprintf(&out, "%s %s (+%d -%d)\n", changeVerb(change), change.Path, change.AddedLines, change.RemovedLines)
		}
		if strings.TrimSpace(change.Preview) != "" {
			out.WriteString(change.Preview)
			out.WriteByte('\n')
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func fileChangeTitle(result tools.Result) string {
	if len(result.Changes) == 0 {
		if result.Status == "error" {
			return "Edit failed"
		}
		return "Edited"
	}
	added, removed := changeTotals(result.Changes)
	if len(result.Changes) == 1 {
		change := result.Changes[0]
		return changeVerb(change) + " " + change.Path + fmt.Sprintf(" (+%d -%d)", change.AddedLines, change.RemovedLines)
	}
	return fmt.Sprintf("Edited %d files (+%d -%d)", len(result.Changes), added, removed)
}

func fileChangeDetail(result tools.Result, previewLimit int) string {
	if len(result.Changes) == 0 {
		if strings.TrimSpace(result.Output) != "" {
			return truncate(result.Output, previewLimit)
		}
		return result.Summary
	}
	var out strings.Builder
	for _, change := range result.Changes {
		if len(result.Changes) > 1 {
			fmt.Fprintf(&out, "%s %s (+%d -%d)\n", changeVerb(change), change.Path, change.AddedLines, change.RemovedLines)
		}
		if strings.TrimSpace(change.Preview) != "" {
			out.WriteString(truncate(change.Preview, previewLimit))
			out.WriteByte('\n')
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func (r *BlockRenderer) renderFileChanges(result tools.Result) {
	if len(result.Changes) == 0 {
		title := "Edited"
		if result.Status == "error" {
			title = "Edit failed"
		}
		r.block(title, result.Summary)
		if strings.TrimSpace(result.Output) != "" {
			printIndented(r.output, "  └ ", truncate(result.Output, r.options.ToolPreviewOutputChars))
		}
		return
	}

	added, removed := changeTotals(result.Changes)
	if len(result.Changes) == 1 {
		change := result.Changes[0]
		title := changeVerb(change) + " " + change.Path + fmt.Sprintf(" (+%d -%d)", change.AddedLines, change.RemovedLines)
		r.block(title, "")
		if strings.TrimSpace(change.Preview) != "" {
			printIndented(r.output, "  └ ", truncate(change.Preview, r.options.ToolPreviewOutputChars))
		}
		return
	}

	r.block(fmt.Sprintf("Edited %d files (+%d -%d)", len(result.Changes), added, removed), "")
	for _, change := range result.Changes {
		fmt.Fprintf(r.output, "  └ %s %s (+%d -%d)\n", changeVerb(change), change.Path, change.AddedLines, change.RemovedLines)
		if strings.TrimSpace(change.Preview) != "" {
			printIndented(r.output, "    ", truncate(change.Preview, r.options.FileChangePreviewChars))
		}
	}
}

func changeVerb(change tools.FileChange) string {
	switch strings.ToLower(change.Action) {
	case "added", "add":
		return "Added"
	case "deleted", "delete":
		return "Deleted"
	default:
		return "Edited"
	}
}
