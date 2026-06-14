package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type BlockRenderer struct {
	output           io.Writer
	markdownRenderer *glamour.TermRenderer
	mu               sync.Mutex
	inRun            bool
	expandedTools    bool
	rewriteFrame     bool
	renderedFrame    bool
	frameLines       int
	todos            []runtimeevent.TodoItem
	toolEvents       []runtimeevent.Event
	keyWatcher       *toggleKeyWatcher
}

func NewBlockRenderer(output io.Writer) *BlockRenderer {
	renderer, _ := newMarkdownRenderer()
	return &BlockRenderer{output: output, markdownRenderer: renderer}
}

func NewInteractiveBlockRenderer(input io.Reader, output io.Writer) *BlockRenderer {
	renderer := NewBlockRenderer(output)
	if outputFile, ok := output.(*os.File); ok && isTerminal(outputFile) {
		renderer.rewriteFrame = true
	}
	renderer.keyWatcher = newToggleKeyWatcher(input, renderer.ToggleTools)
	return renderer
}

func (r *BlockRenderer) HandleEvent(event runtimeevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch event.Type {
	case runtimeevent.TypeRunStart:
		r.beginRun()
	case runtimeevent.TypeRunEnd:
		r.endRun()
	case runtimeevent.TypeAssistantMessage:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		message := cleanTerminalText(event.Message)
		if message != "" {
			r.block(message, "")
		}
	case runtimeevent.TypeTodoUpdate:
		if r.inRun {
			r.todos = append([]runtimeevent.TodoItem(nil), event.Todos...)
			r.renderFrame()
			return
		}
		r.renderTodos(event.Todos)
	case runtimeevent.TypeToolCall:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		r.renderToolCall(event)
	case runtimeevent.TypeToolResult:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		r.renderToolResult(event)
	case runtimeevent.TypeFinal:
		r.renderFinal(event.Message)
	case runtimeevent.TypeError:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		r.block("Error", event.Error)
	case runtimeevent.TypeApprovalRequest:
		if r.inRun {
			r.stopKeyWatcher()
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		r.block("Approval requested", approvalDetail(event))
	case runtimeevent.TypeApprovalDecision:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			r.startKeyWatcher()
			return
		}
		r.block("Approval "+event.Decision, event.Reason)
	}
}

func (r *BlockRenderer) beginRun() {
	r.inRun = true
	r.expandedTools = false
	r.renderedFrame = false
	r.frameLines = 0
	r.todos = nil
	r.toolEvents = nil
	r.startKeyWatcher()
}

func (r *BlockRenderer) endRun() {
	if r.inRun && r.renderedFrame {
		r.renderFrame()
	}
	r.stopKeyWatcher()
	r.inRun = false
}

func (r *BlockRenderer) startKeyWatcher() {
	if r.keyWatcher != nil {
		r.keyWatcher.Start()
	}
}

func (r *BlockRenderer) stopKeyWatcher() {
	if r.keyWatcher != nil {
		r.keyWatcher.Stop()
	}
}

func (r *BlockRenderer) ToggleTools() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inRun {
		return
	}
	r.expandedTools = !r.expandedTools
	r.renderFrame()
}

func (r *BlockRenderer) renderFrame() {
	var buf bytes.Buffer
	r.writeTodoBlock(&buf)
	r.writeToolsBlock(&buf)
	text := buf.String()
	if strings.TrimSpace(text) == "" {
		return
	}
	if r.rewriteFrame && r.renderedFrame && r.frameLines > 0 {
		// Cursor-up keeps the current column on most terminals. Return to column
		// zero first so repeated live-frame redraws do not drift diagonally.
		fmt.Fprintf(r.output, "\r\x1b[%dA\x1b[J", r.frameLines)
	}
	fmt.Fprint(r.output, r.frameOutputText(text))
	r.frameLines = countLines(text)
	r.renderedFrame = true
}

func (r *BlockRenderer) frameOutputText(text string) string {
	if !r.rewriteFrame {
		return text
	}
	// The Ctrl+E watcher keeps the terminal in raw mode while a live frame is
	// visible. Raw mode disables the terminal's usual LF -> CRLF translation,
	// so write CRLF explicitly or each new frame line starts at the previous
	// line's ending column.
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func (r *BlockRenderer) writeTodoBlock(output io.Writer) {
	fmt.Fprintln(output, separatorLine())
	fmt.Fprintln(output, "• Todo")
	if len(r.todos) == 0 {
		fmt.Fprintln(output, "  └ "+greenText("[ ] Waiting for todo list"))
		return
	}
	for _, item := range r.todos {
		fmt.Fprintf(output, "  └ %s\n", greenText(todoMarker(item.Status)+" "+item.Text))
	}
}

func (r *BlockRenderer) writeToolsBlock(output io.Writer) {
	fmt.Fprintln(output, separatorLine())
	if r.expandedTools {
		fmt.Fprintln(output, "• Tools (expanded, Ctrl+E to collapse)")
		if len(r.toolEvents) == 0 {
			fmt.Fprintln(output, "  └ (waiting)")
			return
		}
		for _, event := range r.toolEvents {
			r.writeToolEvent(output, event)
		}
		return
	}

	fmt.Fprintln(output, "• Tools (collapsed, Ctrl+E to expand)")
	if len(r.toolEvents) == 0 {
		fmt.Fprintln(output, "  └ (waiting)")
		return
	}
	fmt.Fprintf(output, "  └ %d event(s), latest: %s\n", len(r.toolEvents), toolEventTitle(r.toolEvents[len(r.toolEvents)-1]))
}

func (r *BlockRenderer) writeToolEvent(output io.Writer, event runtimeevent.Event) {
	title := toolEventTitle(event)
	if title == "" {
		return
	}
	fmt.Fprintln(output, "  └ "+title)
	detail := toolEventDetail(event)
	if strings.TrimSpace(detail) != "" {
		printIndented(output, "    ", detail)
	}
}

func toolEventTitle(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return "Assistant"
	case runtimeevent.TypeToolCall:
		if event.Tool == "run_command" {
			return "Running " + commandTitle(event.Args)
		}
		if isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return ""
		}
		return "Tool " + event.Tool
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		if event.Tool == "run_command" {
			if event.Result.Status == "error" {
				return "Failed " + commandTitle(event.Args)
			}
			return "Ran " + commandTitle(event.Args)
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

func toolEventDetail(event runtimeevent.Event) string {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		return cleanTerminalText(event.Message)
	case runtimeevent.TypeToolCall:
		if event.Tool == "run_command" || isEditTool(event.Tool) || isExploreTool(event.Tool) {
			return ""
		}
		return compactJSON(event.Args, 200)
	case runtimeevent.TypeToolResult:
		if event.Result == nil {
			return ""
		}
		result := *event.Result
		switch {
		case event.Tool == "run_command":
			return summarizeResultOutput(result)
		case isExploreTool(event.Tool):
			if strings.TrimSpace(result.Output) == "" {
				return exploreDetail(event)
			}
			return exploreDetail(event) + "\n" + truncate(result.Output, 2000)
		case isEditTool(event.Tool):
			return fileChangeDetail(result)
		default:
			if strings.TrimSpace(result.Output) != "" {
				return result.Summary + "\n" + truncate(result.Output, 2000)
			}
			return result.Summary
		}
	case runtimeevent.TypeApprovalRequest:
		return approvalDetail(event)
	case runtimeevent.TypeApprovalDecision:
		return event.Reason
	case runtimeevent.TypeError:
		return event.Error
	default:
		return ""
	}
}

func (r *BlockRenderer) renderToolCall(event runtimeevent.Event) {
	if event.Tool == "run_command" {
		command := jsonArgString(event.Args, "command")
		if command == "" {
			command = compactJSON(event.Args, 200)
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
	r.block("Tool "+event.Tool, compactJSON(event.Args, 200))
}

func (r *BlockRenderer) renderToolResult(event runtimeevent.Event) {
	if event.Result == nil {
		return
	}
	result := *event.Result
	switch {
	case event.Tool == "run_command":
		title := "Ran " + commandTitle(event.Args)
		if result.Status == "error" {
			title = "Failed " + commandTitle(event.Args)
		}
		r.block(title, "")
		printIndented(r.output, "  └ ", summarizeResultOutput(result))
	case isExploreTool(event.Tool):
		r.block("Explored", exploreDetail(event))
		if strings.TrimSpace(result.Output) != "" {
			printIndented(r.output, "  └ ", truncate(result.Output, 2000))
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
			printIndented(r.output, "  └ ", truncate(result.Output, 2000))
		}
	}
}

func (r *BlockRenderer) renderTodos(items []runtimeevent.TodoItem) {
	if len(items) == 0 {
		return
	}
	r.block("Todo", "")
	for _, item := range items {
		fmt.Fprintf(r.output, "  └ %s\n", greenText(todoMarker(item.Status)+" "+item.Text))
	}
}

func todoMarker(status runtimeevent.TodoStatus) string {
	switch status {
	case runtimeevent.TodoCompleted:
		return "[✓]"
	case runtimeevent.TodoInProgress:
		return "[>]"
	default:
		return "[ ]"
	}
}

func greenText(text string) string {
	return "\x1b[32m" + text + "\x1b[0m"
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		count++
	}
	return count
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

func fileChangeDetail(result tools.Result) string {
	if len(result.Changes) == 0 {
		if strings.TrimSpace(result.Output) != "" {
			return truncate(result.Output, 2000)
		}
		return result.Summary
	}
	var out strings.Builder
	for _, change := range result.Changes {
		if len(result.Changes) > 1 {
			fmt.Fprintf(&out, "%s %s (+%d -%d)\n", changeVerb(change), change.Path, change.AddedLines, change.RemovedLines)
		}
		if strings.TrimSpace(change.Preview) != "" {
			out.WriteString(truncate(change.Preview, 800))
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
			printIndented(r.output, "  └ ", truncate(result.Output, 2000))
		}
		return
	}

	added, removed := changeTotals(result.Changes)
	if len(result.Changes) == 1 {
		change := result.Changes[0]
		title := changeVerb(change) + " " + change.Path + fmt.Sprintf(" (+%d -%d)", change.AddedLines, change.RemovedLines)
		r.block(title, "")
		if strings.TrimSpace(change.Preview) != "" {
			printIndented(r.output, "  └ ", truncate(change.Preview, 2000))
		}
		return
	}

	r.block(fmt.Sprintf("Edited %d files (+%d -%d)", len(result.Changes), added, removed), "")
	for _, change := range result.Changes {
		fmt.Fprintf(r.output, "  └ %s %s (+%d -%d)\n", changeVerb(change), change.Path, change.AddedLines, change.RemovedLines)
		if strings.TrimSpace(change.Preview) != "" {
			printIndented(r.output, "    ", truncate(change.Preview, 800))
		}
	}
}

func (r *BlockRenderer) renderFinal(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	fmt.Fprintln(r.output, separatorLine())
	rendered, err := renderMarkdown(r.markdownRenderer, message)
	if err != nil {
		fmt.Fprintln(r.output, message)
		return
	}
	fmt.Fprint(r.output, rendered)
	if !strings.HasSuffix(rendered, "\n") {
		fmt.Fprintln(r.output)
	}
}

func renderMarkdown(renderer *glamour.TermRenderer, message string) (string, error) {
	if renderer == nil {
		return glamour.Render(message, "dark")
	}
	return renderer.Render(message)
}

func newMarkdownRenderer() (*glamour.TermRenderer, error) {
	style := styles.DarkStyleConfig
	clearHeadingPrefixes(&style)
	softenCodeBlocks(&style)
	return glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(100),
	)
}

func clearHeadingPrefixes(style *ansi.StyleConfig) {
	for _, heading := range []*ansi.StyleBlock{
		&style.H1,
		&style.H2,
		&style.H3,
		&style.H4,
		&style.H5,
		&style.H6,
	} {
		heading.Prefix = ""
		heading.Suffix = ""
	}
}

func softenCodeBlocks(style *ansi.StyleConfig) {
	style.CodeBlock.Theme = ""
	style.CodeBlock.Chroma = nil
	style.CodeBlock.BackgroundColor = nil
	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.Indent = uintPtr(1)
}

func uintPtr(v uint) *uint {
	return &v
}

func (r *BlockRenderer) block(title string, detail string) {
	fmt.Fprintln(r.output, separatorLine())
	printIndented(r.output, "• ", strings.TrimSpace(title))
	if strings.TrimSpace(detail) != "" {
		printIndented(r.output, "  └ ", strings.TrimSpace(detail))
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

func exploreDetail(event runtimeevent.Event) string {
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
		return compactJSON(event.Args, 200)
	}
}

func commandTitle(args []byte) string {
	command := jsonArgString(args, "command")
	if command == "" {
		return compactJSON(args, 200)
	}
	return command
}

func summarizeResultOutput(result tools.Result) string {
	if strings.TrimSpace(result.Output) != "" {
		return truncate(result.Output, 4000)
	}
	if strings.TrimSpace(result.Summary) != "" {
		return result.Summary
	}
	return "(no output)"
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

func approvalDetail(event runtimeevent.Event) string {
	if event.Tool == "" {
		return event.Reason
	}
	detail := event.Tool + " [" + string(event.Category) + "]"
	if event.Reason != "" {
		detail += ": " + event.Reason
	}
	if args := approvalArgsDetail(event); args != "" {
		detail += "\n" + args
	}
	return detail
}

func approvalArgsDetail(event runtimeevent.Event) string {
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
			return fmt.Sprintf(
				"Path: %s\nReplace: %d bytes -> %d bytes",
				args.Path,
				len(args.OldText),
				len(args.NewText),
			)
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
		return "Args: " + compactJSON(event.Args, 300)
	}
	return ""
}

func textLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
