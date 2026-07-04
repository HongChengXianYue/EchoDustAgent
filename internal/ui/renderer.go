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
	frameText          string
	frameWrapWidth     int
	pendingPromptLines int
	liveFrameMaxLines  int
	liveFrameMaxWidth  int
	userMessage        string
	assistantMessage   string
	todos              []runtimeevent.TodoItem
	toolEvents         []runtimeevent.Event
	keyWatcher         *toggleKeyWatcher

	// Token consumption tracking. mainTokenTotal accumulates the main agent's
	// cumulative total; subagentTokens maps SubagentIndex -> cumulative total.
	mainTokenTotal    int
	mainCachedTokens  int
	subagentTokens    map[int]int
	subagentCacheHits map[int]int
	subagentTaskMap   map[int]string // SubagentIndex -> task label for display
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

func (r *BlockRenderer) refreshLiveFrameBounds() {
	if r == nil || !r.rewriteFrame {
		return
	}
	outputFile, ok := r.output.(*os.File)
	if !ok || !isTerminal(outputFile) {
		return
	}
	r.liveFrameMaxLines, r.liveFrameMaxWidth = liveFrameBounds(outputFile, r.options)
}

// separatorWidth 返回分隔线宽度：options 指定时用 options，否则用终端宽度。
func (r *BlockRenderer) separatorWidth() int {
	if r.options.SeparatorWidth > 0 {
		return r.options.SeparatorWidth
	}
	if outputFile, ok := r.output.(*os.File); ok && isTerminal(outputFile) {
		width, _, err := term.GetSize(int(outputFile.Fd()))
		if err == nil && width > 0 {
			return width
		}
	}
	return 80 // 回退值
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
		if r.renderedFrame {
			r.clearLiveFrame()
		}
		r.beginRun()
	case runtimeevent.TypeUserMessage:
		if r.inRun && r.rewriteFrame {
			r.userMessage = cleanTerminalText(event.Message)
			r.renderFrame()
		}
	case runtimeevent.TypeAssistantDelta:
		if r.inRun {
			r.assistantMessage += event.Delta
			r.renderFrame()
			return
		}
	case runtimeevent.TypeAssistantMessage:
		if r.inRun {
			r.assistantMessage = ""
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
	case runtimeevent.TypeContextPruned, runtimeevent.TypeCompactionStart, runtimeevent.TypeCompactionDone, runtimeevent.TypeCompactionSkip, runtimeevent.TypeStepBudgetExtend, runtimeevent.TypeStepBudgetStop, runtimeevent.TypeStepTiming:
		if r.inRun {
			r.toolEvents = append(r.toolEvents, event)
			r.renderFrame()
			return
		}
		r.block(toolEventTitle(event, r.options.ApprovalArgsPreviewChars), cleanTerminalText(event.Message))
	case runtimeevent.TypeTokenUsage:
		if r.inRun {
			if event.Source == "subagent" {
				if r.subagentTokens == nil {
					r.subagentTokens = make(map[int]int)
				}
				r.subagentTokens[event.SubagentIndex] = event.CumulativeTotal
				if event.CachedTokens > 0 {
					if r.subagentCacheHits == nil {
						r.subagentCacheHits = make(map[int]int)
					}
					r.subagentCacheHits[event.SubagentIndex] += event.CachedTokens
				}
				if event.ParentTool != "" {
					if r.subagentTaskMap == nil {
						r.subagentTaskMap = make(map[int]string)
					}
					r.subagentTaskMap[event.SubagentIndex] = event.ParentTool
				}
			} else {
				r.mainTokenTotal = event.CumulativeTotal
				r.mainCachedTokens += event.CachedTokens
			}
			r.renderFrame()
			return
		}
	}
}

func (r *BlockRenderer) beginRun() {
	r.inRun = true
	r.expandedTools = false
	r.renderedFrame = false
	r.frameLines = 0
	r.frameText = ""
	r.frameWrapWidth = 0
	r.pendingPromptLines = 0
	r.userMessage = ""
	r.assistantMessage = ""
	r.todos = nil
	r.toolEvents = nil
	r.mainTokenTotal = 0
	r.mainCachedTokens = 0
	r.subagentTokens = nil
	r.subagentCacheHits = nil
	r.subagentTaskMap = nil
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
	r.assistantMessage = ""
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

// ReleaseTerminal stops any live key watcher so terminal raw mode is restored.
// Use this before interrupt-driven shutdown while a run is active.
func (r *BlockRenderer) ReleaseTerminal() {
	r.stopKeyWatcher()
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
