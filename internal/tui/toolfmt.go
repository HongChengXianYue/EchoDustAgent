package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

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

func diffBlockTitle(change tools.FileChange) string {
	return fmt.Sprintf("Diff %s (+%d -%d)", change.Path, change.AddedLines, change.RemovedLines)
}

func diffBlockBody(change tools.FileChange) string {
	if strings.TrimSpace(change.Preview) != "" {
		return strings.TrimRight(change.Preview, "\n")
	}
	return strings.TrimRight(change.Diff, "\n")
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

func changeTotals(changes []tools.FileChange) (int, int) {
	added := 0
	removed := 0
	for _, change := range changes {
		added += change.AddedLines
		removed += change.RemovedLines
	}
	return added, removed
}

func approvalDetail(event runtimeevent.Event, argsLimit int) string {
	if event.Tool == "" {
		return event.Reason
	}
	detail := event.Tool + " [" + string(event.Category) + "]"
	if event.Reason != "" {
		detail += ": " + event.Reason
	}
	if args := approvalArgsDetail(event, argsLimit); args != "" {
		detail += "\n" + args
	}
	return detail
}

func approvalArgsDetail(event runtimeevent.Event, argsLimit int) string {
	switch event.Tool {
	case "run_command":
		if command := jsonArgString(event.Args, "command"); command != "" {
			return "Command: " + command
		}
	case "write_file":
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf("Path: %s\nContent: %d lines, %d bytes", args.Path, textLineCount(args.Content), len(args.Content))
		}
	case "replace_in_file":
		var args struct {
			Path    string `json:"path"`
			OldText string `json:"old_text"`
			NewText string `json:"new_text"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf("Path: %s\nReplace: %d bytes -> %d bytes", args.Path, len(args.OldText), len(args.NewText))
		}
	case "apply_patch":
		var args struct {
			Patch string `json:"patch"`
		}
		if json.Unmarshal(event.Args, &args) == nil {
			return fmt.Sprintf("Patch: %d lines, %d bytes", textLineCount(args.Patch), len(args.Patch))
		}
	}
	if len(event.Args) > 0 {
		return "Args: " + compactJSON(event.Args, argsLimit)
	}
	return ""
}

func textLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func toolEventTitle(event runtimeevent.Event, argsLimit int) string {
	prefix := eventTitlePrefix(event)
	switch event.Type {
	case runtimeevent.TypeAssistantDelta:
		return prefix + "Assistant streaming"
	case runtimeevent.TypeAssistantMessage:
		return prefix + "Assistant"
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return prefix + "Subagent"
		}
		return prefix + "Tool " + event.Tool
	case runtimeevent.TypeToolResult:
		return ""
	case runtimeevent.TypeApprovalRequest:
		return prefix + "Approval requested"
	case runtimeevent.TypeApprovalDecision:
		return prefix + "Approval " + event.Decision
	case runtimeevent.TypeContextPruned:
		return prefix + "Pruned context"
	case runtimeevent.TypeCompactionStart:
		return prefix + "Compacting context"
	case runtimeevent.TypeCompactionDone:
		return prefix + "Compacted context"
	case runtimeevent.TypeCompactionSkip:
		return prefix + "Skipped compaction"
	case runtimeevent.TypeStepBudgetExtend:
		return prefix + "Extended step budget"
	case runtimeevent.TypeStepBudgetStop:
		return prefix + "Step budget exhausted"
	case runtimeevent.TypeStepTiming:
		// Display step number as 1-based for human readability.
		return prefix + fmt.Sprintf("Step %d · %s", event.Step+1, formatDuration(event.DurationMS))
	case runtimeevent.TypeRunTiming:
		return prefix + fmt.Sprintf("Total · %s", formatDuration(event.DurationMS))
	case runtimeevent.TypeError:
		return prefix + "Error"
	default:
		return ""
	}
}

// formatDuration renders a millisecond duration as a human-readable string.
// It avoids edge cases like "1m60.0s" by using integer arithmetic for minutes.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	totalSeconds := float64(ms) / 1000.0
	if totalSeconds < 60 {
		return fmt.Sprintf("%.1fs", totalSeconds)
	}
	// Use integer division to avoid floating point edge cases.
	minutes := int(totalSeconds) / 60
	seconds := totalSeconds - float64(minutes*60)
	return fmt.Sprintf("%dm%.1fs", minutes, seconds)
}

func toolEventDetail(event runtimeevent.Event, argsLimit, outputLimit, longOutputLimit, filePreviewLimit int) string {
	switch event.Type {
	case runtimeevent.TypeAssistantDelta:
		return withEventSourceDetail(event, cleanTerminalText(event.Delta))
	case runtimeevent.TypeAssistantMessage:
		return withEventSourceDetail(event, cleanTerminalText(event.Message))
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return withEventSourceDetail(event, delegateTaskDetail(event.Args, argsLimit))
		}
		if len(event.Args) == 0 {
			return ""
		}
		return withEventSourceDetail(event, compactJSON(event.Args, argsLimit))
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		return ""
	case runtimeevent.TypeApprovalRequest:
		return withEventSourceDetail(event, approvalDetail(event, argsLimit))
	case runtimeevent.TypeApprovalDecision:
		return withEventSourceDetail(event, event.Reason)
	case runtimeevent.TypeContextPruned, runtimeevent.TypeCompactionStart, runtimeevent.TypeCompactionDone, runtimeevent.TypeCompactionSkip:
		return withEventSourceDetail(event, cleanTerminalText(event.Message))
	case runtimeevent.TypeStepBudgetExtend, runtimeevent.TypeStepBudgetStop:
		return withEventSourceDetail(event, stepBudgetDetail(event))
	case runtimeevent.TypeStepTiming, runtimeevent.TypeRunTiming:
		// Timing events have no detail body; the title already shows the duration.
		return ""
	case runtimeevent.TypeError:
		return withEventSourceDetail(event, event.Error)
	default:
		return ""
	}
}

func stepBudgetDetail(event runtimeevent.Event) string {
	var parts []string
	if event.Before > 0 && event.After > 0 {
		parts = append(parts, fmt.Sprintf("%d -> %d", event.Before, event.After))
	} else if event.Before > 0 {
		parts = append(parts, fmt.Sprintf("limit %d", event.Before))
	}
	if strings.TrimSpace(event.Reason) != "" {
		parts = append(parts, event.Reason)
	}
	if strings.TrimSpace(event.Message) != "" && len(parts) == 0 {
		parts = append(parts, event.Message)
	}
	return strings.Join(parts, "\n")
}

func isExploreTool(tool string) bool {
	switch tool {
	case "list_files", "find_files", "read_file", "read_file_range", "search_files", "find_symbol", "find_references", "find_callers", "find_callees", "git_status", "git_diff", "git_log":
		return true
	default:
		return false
	}
}

func isEditTool(tool string) bool {
	switch tool {
	case "write_file", "replace_in_file", "apply_patch":
		return true
	default:
		return false
	}
}

func exploreDetail(event runtimeevent.Event, argsLimit int) string {
	switch event.Tool {
	case "list_files":
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "."
		}
		return "List " + path
	case "find_files":
		query := jsonArgString(event.Args, "query")
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "."
		}
		return "Find " + query + " in " + path
	case "read_file":
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "(missing path)"
		}
		return "Read " + path
	case "read_file_range":
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "(missing path)"
		}
		start := jsonArgInt(event.Args, "start_line")
		end := jsonArgInt(event.Args, "end_line")
		return fmt.Sprintf("Read %s lines %d-%d", path, start, end)
	case "search_files":
		query := jsonArgString(event.Args, "query")
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "."
		}
		return "Search " + query + " in " + path
	case "find_symbol":
		query := jsonArgString(event.Args, "query")
		return "Find symbol " + query
	case "find_references":
		path := jsonArgString(event.Args, "path")
		return "Find references in " + path
	case "find_callers":
		path := jsonArgString(event.Args, "path")
		return "Find callers in " + path
	case "find_callees":
		path := jsonArgString(event.Args, "path")
		return "Find callees in " + path
	case "git_status":
		return "Git status"
	case "git_diff":
		path := jsonArgString(event.Args, "path")
		if path == "" {
			return "Git diff"
		}
		return "Git diff " + path
	case "git_log":
		return "Git log"
	default:
		return compactJSON(event.Args, argsLimit)
	}
}

func commandTitle(args []byte, argsLimit int) string {
	command := jsonArgString(args, "command")
	if command == "" {
		return compactJSON(args, argsLimit)
	}
	return command
}

func summarizeResultOutput(result tools.Result, limit int) string {
	if strings.TrimSpace(result.Output) != "" {
		return truncate(result.Output, limit)
	}
	if strings.TrimSpace(result.Summary) != "" {
		return result.Summary
	}
	return "(no output)"
}

func delegateTaskDetail(args []byte, argsLimit int) string {
	task := jsonArgString(args, "task")
	if task == "" {
		return compactJSON(args, argsLimit)
	}
	detail := "Task: " + task
	if expected := jsonArgString(args, "expected_output"); expected != "" {
		detail += "\nExpected output: " + expected
	}
	return truncate(detail, argsLimit)
}

func subagentResultDetail(result tools.Result, limit int) string {
	if strings.TrimSpace(result.Output) != "" {
		if strings.TrimSpace(result.Summary) != "" {
			return result.Summary + "\n" + truncate(result.Output, limit)
		}
		return truncate(result.Output, limit)
	}
	if strings.TrimSpace(result.Summary) != "" {
		return result.Summary
	}
	return "(no output)"
}

func eventTitlePrefix(event runtimeevent.Event) string {
	if event.Source == "subagent" {
		return "Subagent "
	}
	return ""
}

func withEventSourceDetail(event runtimeevent.Event, detail string) string {
	if event.Source != "subagent" || strings.TrimSpace(event.ParentTool) == "" {
		return detail
	}
	prefix := "Task: " + event.ParentTool
	if strings.TrimSpace(detail) == "" {
		return prefix
	}
	return prefix + "\n" + detail
}
