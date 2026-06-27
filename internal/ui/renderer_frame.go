package ui

import (
	"bytes"
	"fmt"
	"strings"

	"local-agent/internal/runtimeevent"
)

func (r *BlockRenderer) renderFrame() {
	r.refreshLiveFrameBounds()
	var buf bytes.Buffer
	r.writeUserMessage(&buf)
	r.writeTodoBlock(&buf)
	r.writeToolsBlock(&buf)
	r.writeAssistantMessage(&buf)
	r.writeTokenUsageBlock(&buf)
	text := buf.String()
	if strings.TrimSpace(text) == "" {
		return
	}
	clearFrameLines := r.currentFrameLinesForClear()
	if r.rewriteFrame {
		text = r.limitLiveFrameText(text)
	}
	if r.rewriteFrame && r.renderedFrame && clearFrameLines > 0 {
		clearLines := clearFrameLines + r.pendingPromptLines
		// Cursor-up keeps the current column on most terminals. Return to column
		// zero first so repeated live-frame redraws do not drift diagonally.
		fmt.Fprintf(r.output, "\r\x1b[%dA\x1b[J", clearLines)
	}
	r.pendingPromptLines = 0
	fmt.Fprint(r.output, r.frameOutputText(text))
	r.frameLines = countLines(text)
	r.frameText = text
	r.frameWrapWidth = r.liveFrameMaxWidth
	r.renderedFrame = true
}

func (r *BlockRenderer) writeUserMessage(output *bytes.Buffer) {
	message := strings.TrimSpace(r.userMessage)
	if message == "" {
		return
	}
	printIndented(output, "› ", message)
}

func (r *BlockRenderer) writeAssistantMessage(output *bytes.Buffer) {
	message := strings.TrimSpace(cleanTerminalText(r.assistantMessage))
	if message == "" {
		return
	}
	fmt.Fprintln(output, separatorLine(r.options.SeparatorWidth))
	fmt.Fprintln(output, "• Assistant (streaming)")
	printIndented(output, "  └ ", truncate(message, r.options.ToolPreviewOutputChars))
}

// writeTokenUsageBlock renders a compact token consumption summary.
// Shows main agent total and per-subagent breakdown when subagents have run.
func (r *BlockRenderer) writeTokenUsageBlock(output *bytes.Buffer) {
	if r.mainTokenTotal == 0 && len(r.subagentTokens) == 0 {
		return
	}
	fmt.Fprintln(output, separatorLine(r.options.SeparatorWidth))
	total := r.mainTokenTotal
	hasSubagents := len(r.subagentTokens) > 0
	if hasSubagents {
		fmt.Fprintf(output, "• Tokens: %s main", formatTokenCount(r.mainTokenTotal))
		for idx, count := range r.subagentTokens {
			label := fmt.Sprintf("sub#%d", idx)
			if task, ok := r.subagentTaskMap[idx]; ok && task != "" {
				label = truncate(task, 20)
			}
			fmt.Fprintf(output, " | %s %s", label, formatTokenCount(count))
			total += count
		}
		fmt.Fprintf(output, " | total %s\n", formatTokenCount(total))
	} else {
		fmt.Fprintf(output, "• Tokens: %s\n", formatTokenCount(r.mainTokenTotal))
	}
}

// formatTokenCount renders a token count with K-suffix for readability.
func formatTokenCount(count int) string {
	if count >= 1000 {
		return fmt.Sprintf("%.1fk", float64(count)/1000)
	}
	return fmt.Sprintf("%d", count)
}

// writeFinalTokenSummary prints a stable token usage summary after the final
// answer. Unlike the live-frame token block, this persists in the terminal
// scrollback so the user can actually see the consumption.
func (r *BlockRenderer) writeFinalTokenSummary() {
	fmt.Fprintln(r.output, separatorLine(r.options.SeparatorWidth))
	if r.mainTokenTotal == 0 && len(r.subagentTokens) == 0 {
		// Provider did not return usage data. Common reasons:
		// - Streaming mode: many providers (including Bailian qwen) omit usage
		//   in SSE chunks. Non-streaming requests usually include it.
		// - The provider simply doesn't support usage reporting.
		fmt.Fprintln(r.output, "• Tokens: N/A (streaming mode or provider omitted usage)")
		return
	}
	total := r.mainTokenTotal
	if len(r.subagentTokens) == 0 {
		fmt.Fprintf(r.output, "• Tokens: %s\n", formatTokenCount(r.mainTokenTotal))
		return
	}
	fmt.Fprintf(r.output, "• Tokens: %s main", formatTokenCount(r.mainTokenTotal))
	for idx, count := range r.subagentTokens {
		label := fmt.Sprintf("sub#%d", idx)
		if task, ok := r.subagentTaskMap[idx]; ok && task != "" {
			label = truncate(task, 20)
		}
		fmt.Fprintf(r.output, " | %s %s", label, formatTokenCount(count))
		total += count
	}
	fmt.Fprintf(r.output, " | total %s\n", formatTokenCount(total))
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

func (r *BlockRenderer) block(title string, detail string) {
	fmt.Fprintln(r.output, separatorLine(r.options.SeparatorWidth))
	printIndented(r.output, "• ", strings.TrimSpace(title))
	if strings.TrimSpace(detail) != "" {
		printIndented(r.output, "  └ ", strings.TrimSpace(detail))
	}
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

func (r *BlockRenderer) currentFrameLinesForClear() int {
	if !r.renderedFrame {
		return 0
	}
	if strings.TrimSpace(r.frameText) == "" {
		return r.frameLines
	}
	width := r.liveFrameMaxWidth
	if width <= 0 {
		width = r.frameWrapWidth
	}
	if width <= 0 {
		return r.frameLines
	}
	return countWrappedLines(r.frameText, width)
}

func countWrappedLines(text string, width int) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	total := 0
	for _, line := range lines {
		lineWidth := displayWidth([]rune(stripANSI(line)))
		wrapped := 1
		if width > 0 && lineWidth > width {
			wrapped = (lineWidth-1)/width + 1
		}
		total += wrapped
	}
	return total
}
