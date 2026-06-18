package ui

import (
	"bytes"
	"fmt"
	"strings"

	"local-agent/internal/runtimeevent"
)

func (r *BlockRenderer) renderFrame() {
	var buf bytes.Buffer
	r.writeUserMessage(&buf)
	r.writeTodoBlock(&buf)
	r.writeToolsBlock(&buf)
	r.writeAssistantMessage(&buf)
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
