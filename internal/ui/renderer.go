package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type BlockRenderer struct {
	output           io.Writer
	markdownRenderer *glamour.TermRenderer
}

func NewBlockRenderer(output io.Writer) *BlockRenderer {
	renderer, _ := newMarkdownRenderer()
	return &BlockRenderer{output: output, markdownRenderer: renderer}
}

func (r *BlockRenderer) HandleEvent(event runtimeevent.Event) {
	switch event.Type {
	case runtimeevent.TypeAssistantMessage:
		message := cleanTerminalText(event.Message)
		if message != "" {
			r.block(message, "")
		}
	case runtimeevent.TypeToolCall:
		r.renderToolCall(event)
	case runtimeevent.TypeToolResult:
		r.renderToolResult(event)
	case runtimeevent.TypeFinal:
		r.renderFinal(event.Message)
	case runtimeevent.TypeError:
		r.block("Error", event.Error)
	case runtimeevent.TypeApprovalRequest:
		r.block("Approval requested", approvalDetail(event))
	case runtimeevent.TypeApprovalDecision:
		r.block("Approval "+event.Decision, event.Reason)
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
	case "list_files", "read_file", "search_files":
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
