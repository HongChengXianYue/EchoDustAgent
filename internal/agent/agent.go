package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"local-agent/internal/approval"
	contextmgr "local-agent/internal/context"
	"local-agent/internal/llm"
	"local-agent/internal/logs"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

// tokenUsage tracks cumulative LLM token consumption for one agent instance.
type tokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
}

type Agent struct {
	client          llm.Client
	registry        *tools.Registry
	messages        []llm.Message
	maxSteps        int
	workspace       string
	renderer        runtimeevent.Handler
	approver        approval.Approver
	emitMu          sync.Mutex
	todoTool        *tools.UpdateTodosTool
	autoTodoText    string
	subagentMu      sync.Mutex
	nextSubagentID  int
	subagentTasks   map[int]*asyncSubagentTask
	options         Options
	subagentLimiter chan struct{}
	subagentTool    *delegateTaskTool

	// Token consumption tracking. Protected by tokenMu because streaming
	// callbacks may arrive on a different goroutine.
	tokenMu    sync.Mutex
	tokenUsage tokenUsage

	// streamingDisabled is set when a streaming call returns without usage
	// data. Subsequent calls fall back to non-streaming so token tracking
	// works for providers that omit usage in SSE chunks (e.g. Bailian qwen).
	streamingDisabled bool
}

func New(client llm.Client, registry *tools.Registry, maxSteps int) *Agent {
	return NewWithWorkspace(client, registry, maxSteps, "")
}

func NewWithWorkspace(client llm.Client, registry *tools.Registry, maxSteps int, workspace string) *Agent {
	return NewWithWorkspaceAndOptions(client, registry, maxSteps, workspace, DefaultOptions())
}

func NewWithWorkspaceAndOptions(client llm.Client, registry *tools.Registry, maxSteps int, workspace string, options Options) *Agent {
	options = normalizeOptions(options)
	prompt := systemPrompt(workspace, options.MaxParallelToolCalls)
	if suffix := strings.TrimSpace(options.SystemPromptSuffix); suffix != "" {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + suffix
	}
	return newAgent(client, registry, maxSteps, workspace, prompt, options)
}

func newAgent(client llm.Client, registry *tools.Registry, maxSteps int, workspace string, prompt string, options Options) *Agent {
	if maxSteps <= 0 {
		maxSteps = 10
	}
	options = normalizeOptions(options)
	todoTool := tools.NewUpdateTodosTool()
	agent := &Agent{
		client:    client,
		registry:  registry,
		maxSteps:  maxSteps,
		workspace: strings.TrimSpace(workspace),
		todoTool:  todoTool,
		options:   options,
		messages: []llm.Message{
			{
				Role:    "system",
				Content: prompt,
			},
		},
		subagentTasks: map[int]*asyncSubagentTask{},
	}
	if options.Subagents.Enabled {
		agent.subagentLimiter = make(chan struct{}, options.Subagents.MaxConcurrent)
		agent.subagentTool = &delegateTaskTool{agent: agent}
	}
	return agent
}

func systemPrompt(workspace string, maxParallelToolCalls int) string {
	if maxParallelToolCalls <= 0 {
		maxParallelToolCalls = DefaultOptions().MaxParallelToolCalls
	}
	lines := []string{
		"# Role",
		"You are a general-purpose local agent.",
		"Be direct, concise, and verify claims with tools only when the user asks for concrete workspace work.",
		"",
		"# Tool Use",
		"Only call tools when the user asks for a concrete workspace action such as reading, listing, searching, editing files, or running commands.",
		"Do not inspect the workspace for greetings, small talk, thanks, or general capability questions.",
		"For multi-step coding, debugging, code-editing, or cross-file work, call update_todos before using tools and keep the todo list current: mark one item in_progress, mark completed items as completed, then move the next item to in_progress.",
		"Simple single-step reads, lookups, or one-off commands do not need a todo list.",
		"You may return multiple tool calls in one assistant turn when the calls are independent.",
		fmt.Sprintf("Do not return more than %d non-update_todos tool calls in one assistant turn. Multiple calls to the same tool with different arguments count separately.", maxParallelToolCalls),
		"Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.",
		"",
		"# Delegation",
		"Use delegate_task for independent read-only research, cross-file investigation, or focused code analysis that can be isolated from the main conversation. Do not use delegate_task for simple direct lookups.",
		"delegate_task starts a background subagent and returns immediately. Continue with any independent parent work after delegating.",
		"For broad codebase analysis, architecture review, finding missing project capabilities, or tasks that would require reading many files, delegate one or more focused research tasks before doing your own synthesis.",
		"When a broad analysis has multiple independent areas, split it into multiple delegate_task calls in the same assistant turn, such as architecture, tools, UI, config, tests, or security.",
		"When delegating multiple tasks in parallel, keep one top-level todo in_progress for the overall delegation or exactly one subtask in_progress; keep the other planned subtasks pending.",
		"When you are ready to synthesize a final answer, the runtime will wait for any unfinished delegated tasks and inject their final conclusions back into the conversation.",
		"Do not personally inspect many files for broad analysis before deciding whether to delegate; use subagents to keep the main context small.",
		"",
		"# Workspace Navigation",
		"Use workspace-relative paths for file tools unless the user explicitly asks for an absolute path.",
		"Run commands in the configured workspace. Do not cd into guessed absolute paths.",
		"When locating a file or directory, or checking whether a path exists anywhere under the workspace, use find_files first. Use list_files only when the user asks to inspect one specific directory level.",
		"Treat requests phrased as under the current directory or under the workspace as recursive unless the user explicitly asks for only top-level or direct children.",
		"If find_files returns candidates, read or list the matching paths before making claims that require their contents or immediate children.",
		"If find_files has no path matches and the user may be looking for text inside files, then use search_files.",
		"",
		"# Final Answers",
		"When responding in the terminal, keep final answers concise. Markdown is allowed for final summaries when it improves readability, but avoid decorative emoji and excessive detail.",
		"When summarizing recent code changes, prefer git log/stat or the worklog first; do not run a full diff unless the user asks for exact diff details.",
		"Final answers must be self-contained. Do not refer to hidden tool logs or prior unseen analysis with phrases like 'the above analysis' unless that analysis is included in the final answer.",
		"After using tools or subagents, never give a final answer that only points to hidden context, such as 'above analysis', 'as shown above', '以上分析', '如上', or '前面已经说明'. Synthesize the concrete findings in the final answer itself.",
		"When you are done, answer directly and concisely.",
	}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		lines = append(lines[:1], append([]string{"Current workspace: " + workspace}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) SetRenderer(renderer runtimeevent.Handler) {
	a.renderer = renderer
}

func (a *Agent) SetApprover(approver approval.Approver) {
	a.approver = approver
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.pruneTransientToolHistory()
	a.todoTool.Reset()
	a.initializeAutoTodo()
	a.resetSubagentIndexes()
	defer a.pruneTransientToolHistory()

	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	a.pruneStaleToolResults()
	a.maybeCompact(runCtx)
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: input})
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	budget := newStepBudget(a.maxSteps, a.options.StepBudget)
	progressHistory := stepProgressHistory{}
	lastStep := 0
	for step := 0; step < budget.limit; step++ {
		a.collectCompletedSubagentResults()
		lastStep = step
		resp, err := a.chatWithTools(runCtx, step)
		if err != nil {
			logs.Errorf("agent chat failed: step=%d err=%v", step, err)
			a.logTokenUsage()
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeError, Error: err.Error()})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			return "", err
		}
		assistantMessage := llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.messages = append(a.messages, assistantMessage)
		assistantMessageIndex := len(a.messages) - 1

		if len(resp.ToolCalls) == 0 {
			// A no-tool assistant turn normally means the run is ready to finish.
			// For async delegate_task, join any unfinished subagents first so the
			// next turn can synthesize their conclusions into the final answer.
			waited, err := a.awaitOutstandingSubagents(runCtx)
			if err != nil {
				logs.Errorf("await subagents failed: step=%d err=%v", step, err)
				a.logTokenUsage()
				a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeError, Error: err.Error()})
				a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
				return "", err
			}
			if waited {
				a.messages = append(a.messages[:assistantMessageIndex], a.messages[assistantMessageIndex+1:]...)
				continue
			}
			final := strings.TrimSpace(resp.Content)
			a.logTokenUsage()
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeFinal, Message: final})
			return final, nil
		}
		if strings.TrimSpace(resp.Content) != "" {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeAssistantMessage, Message: strings.TrimSpace(resp.Content)})
		}
		todosBefore := a.todoTool.Items()
		executedCalls := a.executeToolCalls(runCtx, step, resp.ToolCalls)
		for _, executed := range executedCalls {
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: executed.call.ID,
				Content:    executed.result.JSON(),
			})
		}
		a.collectCompletedSubagentResults()
		progressHistory.record(stepProgressFromExecuted(resp.ToolCalls, executedCalls, todosBefore, a.todoTool.Items()))
		if !a.maybeExtendStepBudget(runCtx, step, &budget, progressHistory) {
			lastStep = step
			break
		}
	}
	err := fmt.Errorf("agent stopped after %d steps without a final response", budget.limit)
	logs.Errorf("agent stopped without final response: max_steps=%d used_steps=%d", budget.limit, lastStep+1)
	a.logTokenUsage()
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeError, Error: err.Error()})
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	return "", err
}

func (a *Agent) chatWithTools(ctx context.Context, step int) (*llm.ChatResponse, error) {
	tools := a.functionTools()
	streamingClient, ok := a.client.(llm.StreamingClient)
	var resp *llm.ChatResponse
	var err error

	// Fall back to non-streaming when a previous streaming call returned no
	// usage data. Some providers (e.g. Bailian qwen) omit usage in SSE chunks.
	if !ok || a.streamingDisabled {
		resp, err = a.client.ChatWithTools(ctx, a.messages, tools)
	} else {
		resp, err = streamingClient.ChatWithToolsStream(ctx, a.messages, tools, func(delta llm.StreamDelta) error {
			if strings.TrimSpace(delta.Content) == "" {
				return nil
			}
			a.emit(runtimeevent.Event{
				Step:    step,
				Type:    runtimeevent.TypeAssistantDelta,
				Delta:   delta.Content,
				Message: delta.Content,
			})
			return nil
		})
	}

	if resp != nil && resp.Usage != nil {
		cumulative := a.addTokenUsage(resp.Usage)
		a.emit(runtimeevent.Event{
			Step:             step,
			Type:             runtimeevent.TypeTokenUsage,
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			CachedTokens:     resp.Usage.CachedTokens,
			CumulativeTotal:  cumulative,
		})
	} else if ok && !a.streamingDisabled {
		// Streaming call returned without usage — disable streaming so the
		// next call uses the non-streaming path which usually includes usage.
		a.streamingDisabled = true
		logs.Infof("streaming returned no usage, falling back to non-streaming")
	}
	return resp, err
}

func (a *Agent) Messages() []llm.Message {
	out := make([]llm.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// ConversationMessages returns the persisted conversation without the current
// system prompt. Session restore always uses the prompt from the current
// process. Any non-prompt system messages are downgraded to user-role context
// so resumed sessions never reintroduce privileged system history mid-stream.
func (a *Agent) ConversationMessages() []llm.Message {
	if len(a.messages) <= 1 {
		return nil
	}
	return cloneRestorableConversation(a.messages[1:])
}

// RestoreConversation resets the in-memory chat history to the current system
// prompt plus a previously persisted conversation.
func (a *Agent) RestoreConversation(messages []llm.Message) error {
	if len(a.messages) == 0 || a.messages[0].Role != "system" {
		return fmt.Errorf("agent is missing its system prompt")
	}
	restored := make([]llm.Message, 0, len(messages)+1)
	restored = append(restored, a.messages[0])
	restored = append(restored, cloneRestorableConversation(messages)...)
	a.messages = restored
	a.pruneTransientToolHistory()
	a.todoTool.Reset()
	a.autoTodoText = ""
	a.resetSubagentIndexes()
	a.tokenMu.Lock()
	a.tokenUsage = tokenUsage{}
	a.tokenMu.Unlock()
	return nil
}

func cloneRestorableConversation(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]llm.Message, len(messages))
	for i, message := range messages {
		out[i] = cloneRestorableMessage(message)
	}
	return out
}

func cloneRestorableMessage(message llm.Message) llm.Message {
	out := message
	if len(message.ToolCalls) > 0 {
		out.ToolCalls = append([]llm.ToolCall(nil), message.ToolCalls...)
	}
	if out.Role == "system" {
		out.Role = "user"
	}
	return out
}

// addTokenUsage accumulates one LLM call's usage and returns the new cumulative total.
func (a *Agent) addTokenUsage(usage *llm.TokenUsage) int {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	a.tokenUsage.PromptTokens += usage.PromptTokens
	a.tokenUsage.CompletionTokens += usage.CompletionTokens
	a.tokenUsage.TotalTokens += usage.TotalTokens
	a.tokenUsage.CachedTokens += usage.CachedTokens
	return a.tokenUsage.TotalTokens
}

// TokenUsage returns the cumulative token consumption snapshot.
func (a *Agent) TokenUsage() tokenUsage {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	return a.tokenUsage
}

// logTokenUsage writes a final cumulative token usage summary to the log file.
func (a *Agent) logTokenUsage() {
	usage := a.TokenUsage()
	logs.Infof("token usage: prompt=%d completion=%d total=%d cached=%d",
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.CachedTokens)
}

func (a *Agent) emit(event runtimeevent.Event) {
	a.emitMu.Lock()
	defer a.emitMu.Unlock()
	if a.renderer != nil {
		a.renderer.HandleEvent(event)
	}
}

func (a *Agent) initializeAutoTodo() {
	text := strings.TrimSpace(a.autoTodoText)
	if text == "" {
		return
	}
	// Subagents are isolated read-only workers. Seed their internal todo state
	// so the workspace-tool safety gate does not depend on the model making a
	// bookkeeping call before its first read/search tool.
	_ = a.todoTool.SetItems([]tools.TodoItem{
		{Text: text, Status: tools.TodoInProgress},
	})
}

func (a *Agent) resetSubagentIndexes() {
	a.subagentMu.Lock()
	a.nextSubagentID = 0
	a.subagentTasks = map[int]*asyncSubagentTask{}
	a.subagentMu.Unlock()
}

func (a *Agent) nextSubagentIndex() int {
	a.subagentMu.Lock()
	defer a.subagentMu.Unlock()
	a.nextSubagentID++
	return a.nextSubagentID
}

func (a *Agent) pruneStaleToolResults() contextmgr.PruneStats {
	stats := contextmgr.PruneStaleToolResults(a.messages, a.options.Context)
	if stats.Results > 0 {
		a.emit(runtimeevent.Event{
			Type:    runtimeevent.TypeContextPruned,
			Message: fmt.Sprintf("pruned %d stale tool result(s), saved about %d bytes", stats.Results, stats.BytesBefore-stats.BytesAfter),
			Count:   stats.Results,
			Before:  stats.BytesBefore,
			After:   stats.BytesAfter,
		})
	}
	return stats
}

func (a *Agent) maybeCompact(ctx context.Context) {
	before, force, ok := contextmgr.CompactionTrigger(a.messages, a.options.Context)
	if !ok {
		return
	}
	a.emit(runtimeevent.Event{
		Type:    runtimeevent.TypeCompactionStart,
		Message: fmt.Sprintf("compacting context at ~%d tokens", before),
		Before:  before,
	})
	stats, err := a.compact(ctx, force)
	if err != nil {
		a.emit(runtimeevent.Event{
			Type:    runtimeevent.TypeCompactionSkip,
			Message: err.Error(),
			Before:  before,
		})
		return
	}
	a.emit(runtimeevent.Event{
		Type:    runtimeevent.TypeCompactionDone,
		Message: fmt.Sprintf("compacted %d message(s), ~%d -> ~%d tokens", stats.Messages, stats.TokensBefore, stats.TokensAfter),
		Count:   stats.Messages,
		Before:  stats.TokensBefore,
		After:   stats.TokensAfter,
	})
}

func (a *Agent) compact(ctx context.Context, force bool) (contextmgr.CompactionStats, error) {
	compacted, stats, err := contextmgr.Compact(ctx, a.messages, a.options.Context, a.summarizeMessages, force)
	if err != nil {
		return contextmgr.CompactionStats{}, err
	}
	a.messages = compacted
	return stats, nil
}

func (a *Agent) summarizeMessages(ctx context.Context, messages []llm.Message) (string, error) {
	input := contextmgr.FormatMessagesForSummary(messages)
	resp, err := a.client.ChatWithTools(ctx, []llm.Message{
		{Role: "system", Content: contextmgr.SummarySystemPrompt},
		{Role: "user", Content: input},
	}, nil)
	if err != nil {
		return "", err
	}
	if len(resp.ToolCalls) > 0 {
		return "", fmt.Errorf("summary model returned tool calls")
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("summary model returned empty content")
	}
	return summary, nil
}
