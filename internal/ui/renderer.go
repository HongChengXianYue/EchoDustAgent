package ui

import (
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
	"local-agent/internal/runtimeevent"
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
	switch event.Type {
	case runtimeevent.TypeRunEnd:
		r.handleRunEnd()
		return
	case runtimeevent.TypeApprovalRequest:
		r.handleApprovalRequest(event)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	switch event.Type {
	case runtimeevent.TypeRunStart:
		r.beginRun()
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

func (r *BlockRenderer) handleRunEnd() {
	r.mu.Lock()
	if r.inRun && r.renderedFrame {
		// The final answer prints after RunEnd. Collapse verbose tool output in
		// the last live frame so the answer is not pushed out of the viewport.
		r.expandedTools = false
		r.renderFrame()
	}
	r.inRun = false
	r.mu.Unlock()

	// Stop may wait for the Ctrl+T full-log viewer to close. The viewer redraws
	// through this renderer after closing, so waiting while holding r.mu can
	// deadlock the final-answer event behind the live log view.
	r.stopKeyWatcher()
}

func (r *BlockRenderer) handleApprovalRequest(event runtimeevent.Event) {
	r.mu.Lock()
	if !r.inRun {
		r.block("Approval requested", approvalDetail(event, r.options.ApprovalArgsPreviewChars))
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	// Approval prompts read from the same terminal, so pause the watcher first.
	// Keep the renderer lock free while Stop waits for an open full-log viewer.
	r.stopKeyWatcher()

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.inRun {
		return
	}
	r.toolEvents = append(r.toolEvents, event)
	r.renderFrame()
	r.pendingPromptLines = approvalPromptLineCount(event)
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
	if !r.inRun {
		r.mu.Unlock()
		return
	}
	options := r.options
	r.mu.Unlock()

	viewer := newStatefulLiveFullLogViewer(input, output, func(state fullLogState) string {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.fullToolLogTextWithState(state)
	}, options)
	viewer.Run()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.renderedFrame {
		r.renderFrame()
	}
}
