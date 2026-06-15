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
	"golang.org/x/term"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type BlockRenderer struct {
	output             io.Writer
	markdownRenderer   *glamour.TermRenderer
	options            Options
	mu                 sync.Mutex
	inRun              bool
	expandedTools      bool
	rewriteFrame       bool
	renderedFrame      bool
	frameLines         int
	pendingPromptLines int
	liveFrameMaxLines  int
	liveFrameMaxWidth  int
	todos              []runtimeevent.TodoItem
	toolEvents         []runtimeevent.Event
	keyWatcher         *toggleKeyWatcher
}

func NewBlockRenderer(output io.Writer) *BlockRenderer {
	return NewBlockRendererWithOptions(output, DefaultOptions())
}

func NewBlockRendererWithOptions(output io.Writer, options Options) *BlockRenderer {
	options = normalizeOptions(options)
	renderer, _ := newMarkdownRenderer(options.MarkdownWordWrap)
	return &BlockRenderer{output: output, markdownRenderer: renderer, options: options}
}

func NewInteractiveBlockRenderer(input io.Reader, output io.Writer) *BlockRenderer {
	return NewInteractiveBlockRendererWithOptions(input, output, DefaultOptions())
}

func NewInteractiveBlockRendererWithOptions(input io.Reader, output io.Writer, options Options) *BlockRenderer {
	options = normalizeOptions(options)
	renderer := NewBlockRendererWithOptions(output, options)
	if outputFile, ok := output.(*os.File); ok && isTerminal(outputFile) {
		renderer.rewriteFrame = true
		renderer.liveFrameMaxLines, renderer.liveFrameMaxWidth = liveFrameBounds(outputFile, options)
	}
	renderer.keyWatcher = newToggleKeyWatcher(input, output, renderer.ToggleTools, renderer.ShowFullToolLog, options.TogglePollMilliseconds)
	return renderer
}

func liveFrameBounds(output *os.File, options Options) (int, int) {
	width, height, err := term.GetSize(int(output.Fd()))
	if err != nil {
		return options.LiveFrameMaxLines, options.LiveFrameMaxWidth
	}

	maxLines := options.LiveFrameMaxLines
	if height > options.LiveFrameHeightMargin {
		maxLines = height - options.LiveFrameHeightMargin
	}
	if maxLines > options.LiveFrameMaxLines {
		maxLines = options.LiveFrameMaxLines
	}
	if maxLines < 4 {
		maxLines = 4
	}

	maxWidth := options.LiveFrameMaxWidth
	if width > 1 {
		maxWidth = width - 1
	}
	return maxLines, maxWidth
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
			r.pendingPromptLines = approvalPromptLineCount(event)
			return
		}
		r.block("Approval requested", approvalDetail(event, r.options.ApprovalArgsPreviewChars))
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
	r.pendingPromptLines = 0
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

func (r *BlockRenderer) ShowFullToolLog(input *os.File, output *os.File) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inRun {
		return
	}

	viewer := newFullLogViewer(input, output, r.fullToolLogText(), r.options)
	viewer.Run()
	if r.renderedFrame {
		r.renderFrame()
	}
}

func (r *BlockRenderer) renderFrame() {
	var buf bytes.Buffer
	r.writeTodoBlock(&buf)
	r.writeToolsBlock(&buf)
	text := buf.String()
	if strings.TrimSpace(text) == "" {
		return
	}
	if r.rewriteFrame {
		text = r.limitLiveFrameText(text)
	}
	if r.rewriteFrame && r.renderedFrame && r.frameLines > 0 {
		clearLines := r.frameLines + r.pendingPromptLines
		// Cursor-up keeps the current column on most terminals. Return to column
		// zero first so repeated live-frame redraws do not drift diagonally.
		fmt.Fprintf(r.output, "\r\x1b[%dA\x1b[J", clearLines)
	}
	r.pendingPromptLines = 0
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

func approvalPromptLineCount(event runtimeevent.Event) int {
	options := len(event.Decisions)
	if options == 0 {
		options = 3
	}
	// The selectable approval prompt renders one line per option and prints one
	// trailing newline after the user confirms a choice.
	return options + 1
}

func (r *BlockRenderer) limitLiveFrameText(text string) string {
	maxLines := r.liveFrameMaxLines
	if maxLines <= 0 {
		maxLines = r.options.LiveFrameMaxLines
	}
	maxWidth := r.liveFrameMaxWidth
	if maxWidth <= 0 {
		maxWidth = r.options.LiveFrameMaxWidth
	}
	return limitTerminalText(text, maxLines, maxWidth)
}

func (r *BlockRenderer) writeTodoBlock(output io.Writer) {
	fmt.Fprintln(output, separatorLine(r.options.SeparatorWidth))
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

func (r *BlockRenderer) renderToolCall(event runtimeevent.Event) {
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

func (r *BlockRenderer) renderFinal(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	fmt.Fprintln(r.output, separatorLine(r.options.SeparatorWidth))
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

func newMarkdownRenderer(wordWrap int) (*glamour.TermRenderer, error) {
	style := styles.DarkStyleConfig
	clearHeadingPrefixes(&style)
	softenCodeBlocks(&style)
	return glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(wordWrap),
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
	fmt.Fprintln(r.output, separatorLine(r.options.SeparatorWidth))
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
