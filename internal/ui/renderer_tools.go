package ui

import (
	"fmt"
	"io"
	"strings"

	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func (r *BlockRenderer) writeToolsBlock(output io.Writer) {
	fmt.Fprintln(output, separatorLine(r.options.SeparatorWidth))
	if r.expandedTools {
		fmt.Fprintln(output, "• Tools (expanded, Ctrl+E to collapse)")
		if len(r.toolEvents) == 0 {
			fmt.Fprintln(output, "  └ (waiting)")
			return
		}
		events := r.expandedToolEvents()
		if hidden := len(r.toolEvents) - len(events); hidden > 0 {
			fmt.Fprintf(output, "  └ … %d earlier event(s) hidden\n", hidden)
		}
		for _, event := range events {
			r.writeToolEvent(output, event)
		}
		return
	}

	fmt.Fprintln(output, "• Tools (collapsed, Ctrl+E to expand)")
	if len(r.toolEvents) == 0 {
		fmt.Fprintln(output, "  └ (waiting)")
		return
	}
	fmt.Fprintf(output, "  └ %d event(s), latest: %s\n", len(r.toolEvents), r.toolEventTitle(r.toolEvents[len(r.toolEvents)-1]))
}

func (r *BlockRenderer) expandedToolEvents() []runtimeevent.Event {
	maxEvents := r.options.MaxExpandedLiveToolEvents
	if !r.rewriteFrame || len(r.toolEvents) <= maxEvents {
		return r.toolEvents
	}
	return r.toolEvents[len(r.toolEvents)-maxEvents:]
}

func (r *BlockRenderer) writeToolEvent(output io.Writer, event runtimeevent.Event) {
	title := r.toolEventTitle(event)
	if title == "" {
		return
	}
	fmt.Fprintln(output, "  └ "+title)
	detail := r.toolEventDetail(event)
	if strings.TrimSpace(detail) != "" {
		printIndented(output, "    ", detail)
	}
}

func (r *BlockRenderer) toolEventTitle(event runtimeevent.Event) string {
	return toolEventTitle(event, r.options.ApprovalArgsPreviewChars)
}

func toolEventTitle(event runtimeevent.Event, argsLimit int) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return "Assistant"
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return "Subagent"
		}
		if event.Tool == "run_command" {
			return "Running " + commandTitle(event.Args, argsLimit)
		}
		if isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return ""
		}
		return "Tool " + event.Tool
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		if tools.IsDelegateTaskTool(event.Tool) {
			if event.Result.Status == "error" {
				return "Subagent failed"
			}
			return "Subagent"
		}
		if event.Tool == "run_command" {
			if event.Result.Status == "error" {
				return "Failed " + commandTitle(event.Args, argsLimit)
			}
			return "Ran " + commandTitle(event.Args, argsLimit)
		}
		if isExploreTool(event.Tool) {
			return "Explored"
		}
		if isEditTool(event.Tool) {
			return fileChangeTitle(*event.Result)
		}
		if event.Result.Status == "error" {
			return "Failed " + event.Tool
		}
		return "Tool " + event.Tool
	case runtimeevent.TypeApprovalRequest:
		return "Approval requested"
	case runtimeevent.TypeApprovalDecision:
		return "Approval " + event.Decision
	case runtimeevent.TypeError:
		return "Error"
	default:
		return ""
	}
}

func (r *BlockRenderer) toolEventDetail(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return cleanTerminalText(event.Message)
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return delegateTaskDetail(event.Args, r.options.ApprovalArgsPreviewChars)
		}
		if event.Tool == "run_command" || isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return ""
		}
		return compactJSON(event.Args, r.options.ApprovalArgsPreviewChars)
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		result := *event.Result
		switch {
		case tools.IsDelegateTaskTool(event.Tool):
			return subagentResultDetail(result, r.options.ToolPreviewLongOutputChars)
		case event.Tool == "run_command":
			return summarizeResultOutput(result, r.options.ToolPreviewLongOutputChars)
		case isExploreTool(event.Tool):
			if strings.TrimSpace(result.Output) == "" {
				return exploreDetail(event, r.options.ApprovalArgsPreviewChars)
			}
			return exploreDetail(event, r.options.ApprovalArgsPreviewChars) + "\n" + truncate(result.Output, r.options.ToolPreviewOutputChars)
		case isEditTool(event.Tool):
			return fileChangeDetail(result, r.options.FileChangePreviewChars)
		default:
			if strings.TrimSpace(result.Output) != "" {
				return result.Summary + "\n" + truncate(result.Output, r.options.ToolPreviewOutputChars)
			}
			return result.Summary
		}
	case runtimeevent.TypeApprovalRequest:
		return approvalDetail(event, r.options.ApprovalArgsPreviewChars)
	case runtimeevent.TypeApprovalDecision:
		return event.Reason
	case runtimeevent.TypeError:
		return event.Error
	default:
		return ""
	}
}

func (r *BlockRenderer) fullToolLogText() string {
	var out strings.Builder
	fmt.Fprintln(&out, "Full Tool Log")
	fmt.Fprintf(&out, "%d event(s)\n\n", len(r.toolEvents))
	if len(r.toolEvents) == 0 {
		fmt.Fprintln(&out, "(no tool events)")
		return out.String()
	}

	for i, event := range r.toolEvents {
		title := fullToolEventTitle(event)
		if title == "" {
			title = "Event"
		}
		fmt.Fprintf(&out, "[%d] %s\n", i+1, title)
		if detail := r.fullToolEventDetail(event); strings.TrimSpace(detail) != "" {
			printIndented(&out, "    ", detail)
		}
		if i != len(r.toolEvents)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func fullToolEventTitle(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return "Assistant"
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return "Calling subagent"
		}
		if event.Tool == "run_command" {
			return "Running " + commandTitle(event.Args, 0)
		}
		if event.Tool == "" {
			return "Tool call"
		}
		return "Calling " + event.Tool
	case runtimeevent.TypeToolResult:
		return toolEventTitle(event, 0)
	case runtimeevent.TypeApprovalRequest:
		return "Approval requested"
	case runtimeevent.TypeApprovalDecision:
		return "Approval " + event.Decision
	case runtimeevent.TypeError:
		return "Error"
	default:
		return string(event.Type)
	}
}

func (r *BlockRenderer) fullToolEventDetail(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return cleanTerminalText(event.Message)
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return delegateTaskDetail(event.Args, 0)
		}
		if event.Tool == "run_command" {
			if command := jsonArgString(event.Args, "command"); command != "" {
				return "Command: " + command
			}
		}
		return "Args: " + compactJSON(event.Args, 0)
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		result := *event.Result
		switch {
		case tools.IsDelegateTaskTool(event.Tool):
			return subagentResultDetail(result, 0)
		case event.Tool == "run_command":
			return fullResultOutput(result)
		case isExploreTool(event.Tool):
			detail := exploreDetail(event, 0)
			if strings.TrimSpace(result.Output) != "" {
				return detail + "\n" + result.Output
			}
			return detail
		case isEditTool(event.Tool):
			return fullFileChangeDetail(result)
		default:
			if strings.TrimSpace(result.Output) != "" {
				if strings.TrimSpace(result.Summary) != "" {
					return result.Summary + "\n" + result.Output
				}
				return result.Output
			}
			return result.Summary
		}
	case runtimeevent.TypeApprovalRequest:
		return approvalDetail(event, 0)
	case runtimeevent.TypeApprovalDecision:
		return event.Reason
	case runtimeevent.TypeError:
		return event.Error
	default:
		return ""
	}
}

func fullResultOutput(result tools.Result) string {
	if strings.TrimSpace(result.Output) != "" {
		return result.Output
	}
	if strings.TrimSpace(result.Summary) != "" {
		return result.Summary
	}
	return "(no output)"
}

func (r *BlockRenderer) renderToolCall(event runtimeevent.Event) {
	if tools.IsDelegateTaskTool(event.Tool) {
		r.block("Subagent", delegateTaskDetail(event.Args, r.options.ApprovalArgsPreviewChars))
		return
	}
	if event.Tool == "run_command" {
		command := jsonArgString(event.Args, "command")
		if command == "" {
			command = compactJSON(event.Args, r.options.ApprovalArgsPreviewChars)
		}
		r.block("Running "+command, "")
		return
	}
	if isEditTool(event.Tool) {
		return
	}
	if isExploreTool(event.Tool) {
		return
	}
	r.block("Tool "+event.Tool, compactJSON(event.Args, r.options.ApprovalArgsPreviewChars))
}

func (r *BlockRenderer) renderToolResult(event runtimeevent.Event) {
	if event.Result == nil {
		return
	}
	result := *event.Result
	switch {
	case tools.IsDelegateTaskTool(event.Tool):
		title := "Subagent"
		if result.Status == "error" {
			title = "Subagent failed"
		}
		r.block(title, subagentResultDetail(result, r.options.ToolPreviewLongOutputChars))
	case event.Tool == "run_command":
		title := "Ran " + commandTitle(event.Args, r.options.ApprovalArgsPreviewChars)
		if result.Status == "error" {
			title = "Failed " + commandTitle(event.Args, r.options.ApprovalArgsPreviewChars)
		}
		r.block(title, "")
		printIndented(r.output, "  └ ", summarizeResultOutput(result, r.options.ToolPreviewLongOutputChars))
	case isExploreTool(event.Tool):
		r.block("Explored", exploreDetail(event, r.options.ApprovalArgsPreviewChars))
		if strings.TrimSpace(result.Output) != "" {
			printIndented(r.output, "  └ ", truncate(result.Output, r.options.ToolPreviewOutputChars))
		}
	case isEditTool(event.Tool):
		r.renderFileChanges(result)
	default:
		title := "Tool " + event.Tool
		if result.Status == "error" {
			title = "Failed " + event.Tool
		}
		r.block(title, result.Summary)
		if strings.TrimSpace(result.Output) != "" {
			printIndented(r.output, "  └ ", truncate(result.Output, r.options.ToolPreviewOutputChars))
		}
	}
}

func isExploreTool(tool string) bool {
	switch tool {
	case "list_files", "find_files", "read_file", "search_files":
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
	case "search_files":
		query := jsonArgString(event.Args, "query")
		path := jsonArgString(event.Args, "path")
		if path == "" {
			path = "."
		}
		return "Search " + query + " in " + path
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
