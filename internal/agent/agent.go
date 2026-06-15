package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

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
	options         Options
	subagentLimiter chan struct{}
	subagentTool    *delegateTaskTool
}

func New(client llm.Client, registry *tools.Registry, maxSteps int) *Agent {
	return NewWithWorkspace(client, registry, maxSteps, "")
}

func NewWithWorkspace(client llm.Client, registry *tools.Registry, maxSteps int, workspace string) *Agent {
	return NewWithWorkspaceAndOptions(client, registry, maxSteps, workspace, DefaultOptions())
}

func NewWithWorkspaceAndOptions(client llm.Client, registry *tools.Registry, maxSteps int, workspace string, options Options) *Agent {
	options = normalizeOptions(options)
	return newAgent(client, registry, maxSteps, workspace, systemPrompt(workspace, options.MaxParallelToolCalls), options)
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
		"You are a local coding agent.",
		"Use the provided function tools when you need workspace information or need to modify files.",
		"For concrete workspace tasks, call update_todos before any workspace tool. Keep the todo list current: mark one item in_progress, mark completed items as completed, then move the next item to in_progress.",
		"You may return multiple tool calls in one assistant turn when the calls are independent.",
		fmt.Sprintf("Do not return more than %d non-update_todos tool calls in one assistant turn. Multiple calls to the same tool with different arguments count separately.", maxParallelToolCalls),
		"Use delegate_task for independent read-only research, cross-file investigation, or focused code analysis that can be isolated from the main conversation. Do not use delegate_task for simple direct lookups.",
		"For broad codebase analysis, architecture review, finding missing project capabilities, or tasks that would require reading many files, delegate one or more focused research tasks before doing your own synthesis.",
		"When a broad analysis has multiple independent areas, split it into multiple delegate_task calls in the same assistant turn, such as architecture, tools, UI, config, tests, or security.",
		"When delegating multiple tasks in parallel, keep one top-level todo in_progress for the overall delegation or exactly one subtask in_progress; keep the other planned subtasks pending.",
		"Do not personally inspect many files for broad analysis before deciding whether to delegate; use subagents to keep the main context small.",
		"Use workspace-relative paths for file tools unless the user explicitly asks for an absolute path.",
		"Run commands in the configured workspace. Do not cd into guessed absolute paths.",
		"When locating a file or directory, or checking whether a path exists anywhere under the workspace, use find_files first. Use list_files only when the user asks to inspect one specific directory level.",
		"Treat requests phrased as under the current directory or under the workspace as recursive unless the user explicitly asks for only top-level or direct children.",
		"If find_files returns candidates, read or list the matching paths before making claims that require their contents or immediate children.",
		"If find_files has no path matches and the user may be looking for text inside files, then use search_files.",
		"Do not inspect the workspace for greetings, small talk, thanks, or general capability questions.",
		"Only call tools when the user asks for a concrete workspace action such as reading, listing, searching, editing files, or running commands.",
		"Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.",
		"When responding in the terminal, keep final answers concise. Markdown is allowed for final summaries when it improves readability, but avoid decorative emoji and excessive detail.",
		"When summarizing recent code changes, prefer git log/stat or the worklog first; do not run a full diff unless the user asks for exact diff details.",
		"Final answers must be self-contained. Do not refer to hidden tool logs or prior unseen analysis with phrases like 'the above analysis' unless that analysis is included in the final answer.",
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
	a.pruneTransientToolHistory()
	a.todoTool.Reset()
	a.initializeAutoTodo()
	a.resetSubagentIndexes()
	defer a.pruneTransientToolHistory()

	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.client.ChatWithTools(ctx, a.messages, a.functionTools())
		if err != nil {
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

		if len(resp.ToolCalls) == 0 {
			final := strings.TrimSpace(resp.Content)
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeFinal, Message: final})
			return final, nil
		}
		if strings.TrimSpace(resp.Content) != "" {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeAssistantMessage, Message: strings.TrimSpace(resp.Content)})
		}
		for _, executed := range a.executeToolCalls(ctx, step, resp.ToolCalls) {
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: executed.call.ID,
				Content:    executed.result.JSON(),
			})
		}
	}
	err := fmt.Errorf("agent stopped after %d steps without a final response", a.maxSteps)
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeError, Error: err.Error()})
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	return "", err
}

func (a *Agent) Messages() []llm.Message {
	out := make([]llm.Message, len(a.messages))
	copy(out, a.messages)
	return out
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
	a.subagentMu.Unlock()
}

func (a *Agent) nextSubagentIndex() int {
	a.subagentMu.Lock()
	defer a.subagentMu.Unlock()
	a.nextSubagentID++
	return a.nextSubagentID
}
