package ui

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

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
	prefix := eventTitlePrefix(event)
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return prefix + "Assistant"
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return prefix + "Subagent"
		}
		if event.Tool == "run_command" {
			return prefix + "Running " + commandTitle(event.Args, argsLimit)
		}
		if isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return ""
		}
		return prefix + "Tool " + event.Tool
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		if tools.IsDelegateTaskTool(event.Tool) {
			if event.Result.Status == "error" {
				return prefix + "Subagent failed"
			}
			return prefix + "Subagent"
		}
		if event.Tool == "run_command" {
			if event.Result.Status == "error" {
				return prefix + "Failed " + commandTitle(event.Args, argsLimit)
			}
			return prefix + "Ran " + commandTitle(event.Args, argsLimit)
		}
		if isExploreTool(event.Tool) {
			return prefix + "Explored"
		}
		if isEditTool(event.Tool) {
			return prefix + fileChangeTitle(*event.Result)
		}
		if event.Result.Status == "error" {
			return prefix + "Failed " + event.Tool
		}
		return prefix + "Tool " + event.Tool
	case runtimeevent.TypeApprovalRequest:
		return prefix + "Approval requested"
	case runtimeevent.TypeApprovalDecision:
		return prefix + "Approval " + event.Decision
	case runtimeevent.TypeError:
		return prefix + "Error"
	default:
		return ""
	}
}

func (r *BlockRenderer) toolEventDetail(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return withEventSourceDetail(event, cleanTerminalText(event.Message))
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return withEventSourceDetail(event, delegateTaskDetail(event.Args, r.options.ApprovalArgsPreviewChars))
		}
		if event.Tool == "run_command" || isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return withEventSourceDetail(event, "")
		}
		return withEventSourceDetail(event, compactJSON(event.Args, r.options.ApprovalArgsPreviewChars))
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		result := *event.Result
		switch {
		case tools.IsDelegateTaskTool(event.Tool):
			return withEventSourceDetail(event, subagentResultDetail(result, r.options.ToolPreviewLongOutputChars))
		case event.Tool == "run_command":
			return withEventSourceDetail(event, summarizeResultOutput(result, r.options.ToolPreviewLongOutputChars))
		case isExploreTool(event.Tool):
			if strings.TrimSpace(result.Output) == "" {
				return withEventSourceDetail(event, exploreDetail(event, r.options.ApprovalArgsPreviewChars))
			}
			return withEventSourceDetail(event, exploreDetail(event, r.options.ApprovalArgsPreviewChars)+"\n"+truncate(result.Output, r.options.ToolPreviewOutputChars))
		case isEditTool(event.Tool):
			return withEventSourceDetail(event, fileChangeDetail(result, r.options.FileChangePreviewChars))
		default:
			if strings.TrimSpace(result.Output) != "" {
				return withEventSourceDetail(event, result.Summary+"\n"+truncate(result.Output, r.options.ToolPreviewOutputChars))
			}
			return withEventSourceDetail(event, result.Summary)
		}
	case runtimeevent.TypeApprovalRequest:
		return withEventSourceDetail(event, approvalDetail(event, r.options.ApprovalArgsPreviewChars))
	case runtimeevent.TypeApprovalDecision:
		return withEventSourceDetail(event, event.Reason)
	case runtimeevent.TypeError:
		return withEventSourceDetail(event, event.Error)
	default:
		return ""
	}
}

func (r *BlockRenderer) fullToolLogText() string {
	return r.fullToolLogTextWithState(fullLogState{})
}

func (r *BlockRenderer) fullToolLogTextWithState(state fullLogState) string {
	var out strings.Builder
	fmt.Fprintln(&out, "Full Tool Log")
	fmt.Fprintf(&out, "%d event(s)\n\n", len(r.toolEvents))
	if len(r.toolEvents) == 0 {
		fmt.Fprintln(&out, "(no tool events)")
		return out.String()
	}

	mainEvents, subagentBlocks := r.fullToolLogBlocks()
	r.writeFullLogMainBlock(&out, mainEvents)
	for _, block := range subagentBlocks {
		out.WriteByte('\n')
		r.writeFullLogSubagentBlock(&out, block, state.SubagentExpanded(block.index))
	}
	return out.String()
}

type fullLogIndexedEvent struct {
	number int
	event  runtimeevent.Event
}

type fullLogSubagentBlock struct {
	index  int
	task   string
	events []fullLogIndexedEvent
}

func (r *BlockRenderer) fullToolLogBlocks() ([]fullLogIndexedEvent, []fullLogSubagentBlock) {
	maxIndex := 0
	for _, event := range r.toolEvents {
		if event.Source == "subagent" && event.SubagentIndex > maxIndex {
			maxIndex = event.SubagentIndex
		}
	}

	fallbackIndex := maxIndex + 1
	if fallbackIndex == 1 {
		fallbackIndex = 1
	}
	fallbackByTask := map[string]int{}
	mainEvents := []fullLogIndexedEvent{}
	subagentByIndex := map[int]*fullLogSubagentBlock{}

	for i, event := range r.toolEvents {
		indexed := fullLogIndexedEvent{number: i + 1, event: event}
		if event.Source != "subagent" {
			mainEvents = append(mainEvents, indexed)
			continue
		}

		index := event.SubagentIndex
		if index <= 0 {
			key := strings.TrimSpace(event.ParentTool)
			if key == "" {
				key = "(unknown subagent)"
			}
			if existing, ok := fallbackByTask[key]; ok {
				index = existing
			} else {
				index = fallbackIndex
				fallbackByTask[key] = index
				fallbackIndex++
			}
		}

		block := subagentByIndex[index]
		if block == nil {
			block = &fullLogSubagentBlock{index: index}
			subagentByIndex[index] = block
		}
		if block.task == "" {
			block.task = strings.TrimSpace(event.ParentTool)
		}
		block.events = append(block.events, indexed)
	}

	subagentBlocks := make([]fullLogSubagentBlock, 0, len(subagentByIndex))
	for _, block := range subagentByIndex {
		subagentBlocks = append(subagentBlocks, *block)
	}
	sort.Slice(subagentBlocks, func(i, j int) bool {
		return subagentBlocks[i].index < subagentBlocks[j].index
	})
	return mainEvents, subagentBlocks
}

func (r *BlockRenderer) writeFullLogMainBlock(out *strings.Builder, events []fullLogIndexedEvent) {
	fmt.Fprintf(out, "Main (%d event(s))\n", len(events))
	if len(events) == 0 {
		fmt.Fprintln(out, "  (no main events)")
		return
	}
	for i, event := range events {
		r.writeFullLogEvent(out, event, "", false)
		if i != len(events)-1 {
			out.WriteByte('\n')
		}
	}
}

func (r *BlockRenderer) writeFullLogSubagentBlock(out *strings.Builder, block fullLogSubagentBlock, expanded bool) {
	color := subagentFullLogColor(block.index)
	state := "collapsed"
	action := "expand"
	if expanded {
		state = "expanded"
		action = "collapse"
	}
	shortcut := "no shortcut"
	if block.index >= 1 && block.index <= 5 {
		shortcut = fmt.Sprintf("Ctrl+%d to %s", block.index, action)
	}
	writeFullLogColoredLine(out, color, fmt.Sprintf("Subagent-%d (%s, %s) | %d event(s)", block.index, state, shortcut, len(block.events)))
	if strings.TrimSpace(block.task) != "" {
		writeFullLogColoredLine(out, color, "  Task: "+block.task)
	}
	if len(block.events) == 0 {
		writeFullLogColoredLine(out, color, "  (no events)")
		return
	}
	if !expanded {
		latest := block.events[len(block.events)-1].event
		latest.Source = ""
		latest.ParentTool = ""
		title := fullToolEventTitle(latest)
		if title == "" {
			title = "Event"
		}
		writeFullLogColoredLine(out, color, "  Latest: "+title)
		return
	}
	for i, event := range block.events {
		r.writeFullLogEvent(out, event, color, true)
		if i != len(block.events)-1 {
			out.WriteByte('\n')
		}
	}
}

func (r *BlockRenderer) writeFullLogEvent(out *strings.Builder, indexed fullLogIndexedEvent, color string, stripSource bool) {
	event := indexed.event
	if stripSource {
		event.Source = ""
		event.ParentTool = ""
	}
	title := fullToolEventTitle(event)
	if title == "" {
		title = "Event"
	}
	fmt.Fprintf(out, "[%d] ", indexed.number)
	writeFullLogColoredLine(out, color, title)
	if detail := r.fullToolEventDetail(event); strings.TrimSpace(detail) != "" {
		if color == "" {
			printIndented(out, "    ", detail)
		} else {
			printColoredIndented(out, "    ", detail, color)
		}
	}
}

func subagentFullLogColor(index int) string {
	colors := []string{
		"\x1b[36m",
		"\x1b[33m",
		"\x1b[35m",
		"\x1b[32m",
		"\x1b[34m",
	}
	if index < 1 || index > len(colors) {
		return ""
	}
	return colors[index-1]
}

func writeFullLogColoredLine(out io.Writer, color string, text string) {
	if color == "" {
		fmt.Fprintln(out, text)
		return
	}
	fmt.Fprintln(out, color+text+"\x1b[0m")
}

func printColoredIndented(output io.Writer, prefix string, text string, color string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		writeFullLogColoredLine(output, color, prefix+"(no output)")
		return
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i == 0 {
			writeFullLogColoredLine(output, color, prefix+line)
			continue
		}
		writeFullLogColoredLine(output, color, strings.Repeat(" ", utf8.RuneCountInString(prefix))+line)
	}
}

func fullToolEventTitle(event runtimeevent.Event) string {
	prefix := eventTitlePrefix(event)
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return prefix + "Assistant"
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return prefix + "Calling subagent"
		}
		if event.Tool == "run_command" {
			return prefix + "Running " + commandTitle(event.Args, 0)
		}
		if event.Tool == "" {
			return prefix + "Tool call"
		}
		return prefix + "Calling " + event.Tool
	case runtimeevent.TypeToolResult:
		return toolEventTitle(event, 0)
	case runtimeevent.TypeApprovalRequest:
		return prefix + "Approval requested"
	case runtimeevent.TypeApprovalDecision:
		return prefix + "Approval " + event.Decision
	case runtimeevent.TypeError:
		return prefix + "Error"
	default:
		return prefix + string(event.Type)
	}
}

func (r *BlockRenderer) fullToolEventDetail(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return withEventSourceDetail(event, cleanTerminalText(event.Message))
	case runtimeevent.TypeToolCall:
		if tools.IsDelegateTaskTool(event.Tool) {
			return withEventSourceDetail(event, delegateTaskDetail(event.Args, 0))
		}
		if event.Tool == "run_command" {
			if command := jsonArgString(event.Args, "command"); command != "" {
				return withEventSourceDetail(event, "Command: "+command)
			}
		}
		return withEventSourceDetail(event, "Args: "+compactJSON(event.Args, 0))
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		result := *event.Result
		switch {
		case tools.IsDelegateTaskTool(event.Tool):
			return withEventSourceDetail(event, subagentResultDetail(result, 0))
		case event.Tool == "run_command":
			return withEventSourceDetail(event, fullResultOutput(result))
		case isExploreTool(event.Tool):
			detail := exploreDetail(event, 0)
			if strings.TrimSpace(result.Output) != "" {
				return withEventSourceDetail(event, detail+"\n"+result.Output)
			}
			return withEventSourceDetail(event, detail)
		case isEditTool(event.Tool):
			return withEventSourceDetail(event, fullFileChangeDetail(result))
		default:
			if strings.TrimSpace(result.Output) != "" {
				if strings.TrimSpace(result.Summary) != "" {
					return withEventSourceDetail(event, result.Summary+"\n"+result.Output)
				}
				return withEventSourceDetail(event, result.Output)
			}
			return withEventSourceDetail(event, result.Summary)
		}
	case runtimeevent.TypeApprovalRequest:
		return withEventSourceDetail(event, approvalDetail(event, 0))
	case runtimeevent.TypeApprovalDecision:
		return withEventSourceDetail(event, event.Reason)
	case runtimeevent.TypeError:
		return withEventSourceDetail(event, event.Error)
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
	prefix := eventTitlePrefix(event)
	if tools.IsDelegateTaskTool(event.Tool) {
		r.block(prefix+"Subagent", delegateTaskDetail(event.Args, r.options.ApprovalArgsPreviewChars))
		return
	}
	if event.Tool == "run_command" {
		command := jsonArgString(event.Args, "command")
		if command == "" {
			command = compactJSON(event.Args, r.options.ApprovalArgsPreviewChars)
		}
		r.block(prefix+"Running "+command, "")
		return
	}
	if isEditTool(event.Tool) {
		return
	}
	if isExploreTool(event.Tool) {
		return
	}
	r.block(prefix+"Tool "+event.Tool, compactJSON(event.Args, r.options.ApprovalArgsPreviewChars))
}

func (r *BlockRenderer) renderToolResult(event runtimeevent.Event) {
	if event.Result == nil {
		return
	}
	prefix := eventTitlePrefix(event)
	result := *event.Result
	switch {
	case tools.IsDelegateTaskTool(event.Tool):
		title := prefix + "Subagent"
		if result.Status == "error" {
			title = prefix + "Subagent failed"
		}
		r.block(title, subagentResultDetail(result, r.options.ToolPreviewLongOutputChars))
	case event.Tool == "run_command":
		title := prefix + "Ran " + commandTitle(event.Args, r.options.ApprovalArgsPreviewChars)
		if result.Status == "error" {
			title = prefix + "Failed " + commandTitle(event.Args, r.options.ApprovalArgsPreviewChars)
		}
		r.block(title, "")
		printIndented(r.output, "  └ ", withEventSourceDetail(event, summarizeResultOutput(result, r.options.ToolPreviewLongOutputChars)))
	case isExploreTool(event.Tool):
		r.block(prefix+"Explored", withEventSourceDetail(event, exploreDetail(event, r.options.ApprovalArgsPreviewChars)))
		if strings.TrimSpace(result.Output) != "" {
			printIndented(r.output, "  └ ", truncate(result.Output, r.options.ToolPreviewOutputChars))
		}
	case isEditTool(event.Tool):
		r.renderFileChanges(result)
	default:
		title := prefix + "Tool " + event.Tool
		if result.Status == "error" {
			title = prefix + "Failed " + event.Tool
		}
		r.block(title, withEventSourceDetail(event, result.Summary))
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
