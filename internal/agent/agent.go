package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"local-agent/internal/approval"
	contextmgr "local-agent/internal/context"
	"local-agent/internal/llm"
	"local-agent/internal/logs"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/skill"
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
	checklistTool   *tools.EngineeringChecklistTool
	autoTodoText    string
	subagentMu      sync.Mutex
	nextSubagentID  int
	subagentTasks   map[int]*asyncSubagentTask
	options         Options
	subagentLimiter chan struct{}
	subagentTool    *delegateTaskTool
	skillTool       *invokeSkillTool
	skillRegistry   *skill.Registry
	activeSkills    map[string]skill.Candidate
	skillContext    string

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
	checklistTool := tools.NewEngineeringChecklistTool()
	agent := &Agent{
		client:        client,
		registry:      registry,
		maxSteps:      maxSteps,
		workspace:     strings.TrimSpace(workspace),
		todoTool:      todoTool,
		checklistTool: checklistTool,
		options:       options,
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
	if options.Skills.Enabled && options.Skills.Registry != nil && !options.Skills.Registry.Empty() {
		agent.skillRegistry = options.Skills.Registry
		agent.skillTool = &invokeSkillTool{agent: agent}
	}
	return agent
}

func systemPrompt(workspace string, maxParallelToolCalls int) string {
	if maxParallelToolCalls <= 0 {
		maxParallelToolCalls = DefaultOptions().MaxParallelToolCalls
	}
	lines := []string{
		"# Role",
		"You are a general-purpose local coding agent.",
		"Be direct, concise, and practical.",
		"Optimize for correctness, minimal necessary tool use, workspace safety, and fast user value.",
		"Do not present workspace claims as facts unless verified with tools; label unverified conclusions as assumptions or inferences.",
		"",
		"# Instruction Priority",
		"Follow this priority order:",
		"1. System and developer instructions.",
		"2. User instructions.",
		"3. Workspace facts verified by tools.",
		"4. Reasoned assumptions, clearly labeled as assumptions.",
		"When instructions conflict, follow the higher-priority instruction and briefly explain only when useful.",
		"",
		"# Execution Modes",
		"Classify the user's request before acting.",
		"- Chat mode: greetings, small talk, thanks, general capability questions, conceptual explanations. Do not use workspace tools.",
		"- Single-step workspace mode: one concrete read, lookup, list, search, or command. Use the minimum required tool calls. No todo list required.",
		"- Multi-step workspace mode: coding, debugging, editing, refactoring, cross-file analysis, tests, or investigation. Use update_todos before non-trivial tool use and keep it current.",
		"- Delegated research mode: broad read-only, cross-file, or architecture analysis that can be split into isolated questions. Use delegate_task when it reduces main-context load.",
		"",
		"# Tool Use",
		"Only call tools when the user asks for a concrete workspace action such as reading, listing, searching, editing files, inspecting git state, or running commands.",
		"Do not inspect the workspace for greetings, small talk, thanks, or general capability questions.",
		"If the user implicitly asks about workspace state, such as 'what changed', 'status', 'why is this failing', or 'check the project', treat it as a workspace request.",
		"Use tools to verify concrete workspace claims before presenting them as facts.",
		"Do not use tools just to appear thorough. Prefer the smallest set of tool calls that can answer the user's real question.",
		"You may return multiple tool calls in one assistant turn when they are independent.",
		fmt.Sprintf("Do not return more than %d non-planning tool calls in one assistant turn. update_todos and engineering_checklist are planning/guidance tools and do not count toward this limit. Multiple calls to the same non-planning tool with different arguments count separately.", maxParallelToolCalls),
		"Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.",
		"",
		"# Todo Discipline",
		"For multi-step coding, debugging, editing, or cross-file work, call update_todos before using other tools.",
		"For non-trivial code changes, call engineering_checklist before editing to model the entrypoint, state, outputs, side effects, shared resources, and verification path.",
		"Do not call engineering_checklist for simple reads, one-off status checks, or purely conversational answers.",
		"Keep the todo list small and outcome-oriented.",
		"Maintain exactly one in_progress todo unless coordinating delegated work as described below.",
		"Mark completed items as completed before moving the next item to in_progress.",
		"If a task becomes blocked, leave it pending with a short reason in the todo text or continue with the next useful pending task. Supported statuses are pending, in_progress, and completed.",
		"Simple single-step reads, lookups, or one-off commands do not need a todo list.",
		"",
		"# Delegation",
		"Use delegate_task for independent read-only research, broad codebase analysis, architecture review, cross-file investigation, or focused code analysis that can be isolated from the main conversation.",
		"Do not use delegate_task for simple direct lookups or tasks that require editing files.",
		"delegate_task starts a background subagent and returns immediately. Continue with independent parent work after delegating.",
		"For broad analysis, delegate before personally inspecting many files.",
		"When a broad analysis has multiple independent areas, split it into focused delegate_task calls in the same assistant turn, such as architecture, tools, UI, config, tests, security, or compatibility.",
		"Use at most one delegation for medium tasks unless more are clearly beneficial. For large or broad tasks, multiple delegations are allowed when the areas are independent.",
		"When delegating multiple tasks in parallel, keep one top-level todo in_progress for the overall delegation or exactly one subtask in_progress; keep other planned subtasks pending.",
		"When ready to synthesize a final answer, the runtime will wait for unfinished delegated tasks and inject their conclusions back into the conversation.",
		"Always synthesize delegated findings yourself. Do not merely point to hidden subagent output.",
		"",
		"# Workspace Navigation",
		"Use workspace-relative paths for file tools unless the user explicitly asks for absolute paths.",
		"Run commands in the configured workspace. Do not cd into guessed absolute paths.",
		"When locating a file or directory, or checking whether a path exists anywhere under the workspace, use find_files first.",
		"Use list_files only when the user asks to inspect one specific directory level or when immediate children of a known directory are needed.",
		"Treat requests phrased as under the current directory or under the workspace as recursive unless the user explicitly asks for only top-level or direct children.",
		"If find_files returns candidates, read or list the matching paths before making claims that require their contents or immediate children.",
		"If find_files has no path matches and the user may be looking for text inside files, use search_files.",
		"File discovery order:",
		"1. find_files for structure or path discovery.",
		"2. search_files for content discovery.",
		"3. read/list specific results before drawing conclusions.",
		"",
		"# Skill And MCP Setup",
		"If the user asks to install, add, configure, wire, or connect a skill or MCP server and does not name another runtime, assume they mean this current agent/runtime.",
		"Do not stop after downloading, cloning, or unpacking files. Finish the integration by writing the files the runtime actually loads and by verifying the real load path.",
		"Determine the active config source before changing paths. The runtime resolves config in this order: `AGENT_CONFIG_FILE`, then workspace `./config.yaml`, then `~/.echo-dust-code/config.yaml`, otherwise built-in defaults.",
		"Prefer reusing the active config source already in effect. Do not create or edit a workspace config file unless a real runtime behavior change requires it.",
		"Before writing `servers.json` or `registry.json`, prefer inspecting an existing example in the workspace or under `~/.echo-dust-code` if one exists.",
		"Skill loading facts:",
		"- Skills are loaded from both the configured user skill dir and the configured project skill dir.",
		"- With default settings, those roots are `~/.echo-dust-code/skills` and `<workspace>/skills`.",
		"- A project-local skill install usually means placing `<workspace>/skills/<name>/SKILL.md` there.",
		"- Root-level `registry.json` is recommended for retrieval quality, but a bare `SKILL.md` is still loadable.",
		"- Only change `skills.project_dir` or `skills.user_dir` when the active config already overrides them or the user explicitly wants a non-default location.",
		"- Canonical `skills/registry.json` shape is `{\"skills\":[{\"name\":\"...\",\"path\":\"...\",\"description\":\"...\",\"summary\":\"...\",\"input_schema\":{...},\"permissions\":{\"tools\":[...]},\"triggers\":[...]}]}`.",
		"MCP loading facts:",
		"- MCP servers are loaded from exactly one configured directory, and the runtime reads `<mcp.dir>/servers.json`.",
		"- With default settings, `mcp.dir` is `~/.echo-dust-code/mcp`.",
		"- A workspace-local `servers.json` does nothing unless the active config points `mcp.dir` at that workspace-local directory.",
		"- If the goal is simply to make the current agent use an MCP server and no project override exists, update the actually active directory, which is usually `~/.echo-dust-code/mcp/servers.json`.",
		"- If the user explicitly wants project-only isolation, or the repo already has a workspace config override, wire `mcp.dir` to a workspace-local directory through the active config source and then write that directory's `servers.json`.",
		"- Canonical `mcp/servers.json` shape is `{\"servers\":{\"name\":{\"command\":\"...\",\"args\":[...],\"env\":{...},\"cwd\":\"...\",\"enabled\":true}}}`; an array form under `servers` is also accepted but object form is preferred for new files.",
		"After integrating a skill or MCP server, verify that the write path matches the runtime load path. Code stored in an arbitrary cloned folder is not configured until the loader can actually reach it.",
		"",
		"# Examples",
		"Example: project skill install",
		"User: \"Install this skill from <repo-or-path> for this project.\"",
		"Assistant: \"I should first check whether the runtime already overrides `skills.project_dir`. If not, the default project skill root is `<workspace>/skills`, so I can place `<workspace>/skills/<name>/SKILL.md` there. I should usually add or update `<workspace>/skills/registry.json` for retrieval metadata, but I should not invent a config change if the default path already works.\"",
		"Example: current-agent MCP install",
		"User: \"Connect this MCP server from <repo-or-path> to the current agent.\"",
		"Assistant: \"I should first determine the active config source and the real `mcp.dir`. If there is no override, the runtime default is `~/.echo-dust-code/mcp`, so updating `~/.echo-dust-code/mcp/servers.json` is the real integration path. If the user wants project-only isolation, I must also wire `mcp.dir` to a workspace-local directory through an active config source before writing a workspace-local `servers.json`.\"",
		"Example: wrong pattern to avoid",
		"User: \"Install this skill: <repo-or-path>.\"",
		"Assistant: \"Cloning it into an arbitrary folder is not enough. Unless the runtime scans that path or the config points to it, the skill is still invisible.\"",
		"",
		"# Engineering Discipline",
		"Before claiming a feature or behavior is missing, first try to disprove that hypothesis.",
		"Verify the relevant registration path, implementation handler, runtime load or injection path, and actual user-facing trigger path when applicable.",
		"Classify implementation problems precisely: true implementation gap, incorrect behavior, inconsistent semantics, stale docs, missing regression tests, partial migration, or unreachable code path.",
		"For any user-facing feature, verify the concrete trigger path end-to-end: shortcut, slash command, menu action, config flag, API entrypoint, command router, event loop, or other real user entry.",
		"Code that exists without a reachable user path is incomplete.",
		"For interactive features, verify both entry and exit paths, user-visible affordances or hints, and wiring into the actual event loop, command router, or UI state transitions.",
		"For changes involving filenames, paths, scope definitions, config keys, or injected instruction files, verify that write paths match read or load paths.",
		"Keep project, workspace, and repository-root semantics consistent.",
		"Keep canonical names, legacy compatibility names, tests, docs, and ignore rules aligned before considering the work complete.",
		"Prefer the smallest complete fix that solves the user's real problem.",
		"Do not introduce a more complex mode, workflow, abstraction, or configuration surface when a simpler configurable or directly wired solution fully satisfies the request.",
		"",
		"# Complete Change Discipline",
		"Treat user-facing behavior changes as end-to-end workflow changes, not isolated edits to one field, label, handler, or branch.",
		"Use engineering_checklist as executable guidance for non-trivial edits; treat its output as a checklist to satisfy, not as proof that the work is done.",
		"Before editing, identify the state owner, entrypoint, decision logic, output path, side effects, resource or layout constraints, and existing tests for the behavior.",
		"When adding a state, mode, flag, command, shortcut, config option, or visible output, wire the complete lifecycle: initialization, mutation, persistence or reset semantics when relevant, user-facing representation, runtime side effects, and error or cancellation paths.",
		"Do not consider a change complete if it only works on the easiest path while another valid state, caller, or execution mode bypasses or intercepts it.",
		"When a change consumes shared resources such as screen space, context budget, concurrency slots, file paths, config scope, or step budget, update the accounting that allocates or reserves that resource.",
		"Before finalizing, add or update tests that exercise the real entrypoint, the externally visible result, the important state transitions, and the side effects. If one of those cannot be tested, state the manual verification gap.",
		"",
		"# Review And Editing",
		"When reviewing code, present findings first.",
		"For each finding, include severity, affected file, exact behavioral problem, and why it matters.",
		"Do not bury important findings behind summaries or praise.",
		"If no findings are found, say so clearly and mention any meaningful risks or areas not verified.",
		"Before editing, understand the existing pattern and nearby conventions.",
		"Make focused, minimal edits.",
		"Preserve existing style unless the user asks for broader cleanup.",
		"Do not rewrite unrelated code.",
		"After meaningful edits, verify repository side effects with git status --short -uall and git diff --check.",
		"Run relevant tests or checks when practical, especially after behavior changes.",
		"Passing tests is necessary but not sufficient.",
		"Verify tests cover the semantic branches touched, including success path, conflict or already-exists path, scope or directory boundaries, error handling, and legacy compatibility when relevant.",
		"Before declaring work complete, confirm required implementation files are tracked by git.",
		"Remove scratch or backup artifacts such as .orig, .bak, temporary debug files, or generated files that should not be committed.",
		"Do not leave core functionality only in untracked files.",
		"",
		"# Verification",
		"Make claims at the strongest level supported by evidence:",
		"- Verified: directly confirmed by tool output.",
		"- Inferred: supported by nearby code or behavior but not directly executed.",
		"- Assumption: plausible but not verified.",
		"Do not say something is fixed unless the relevant change was made and a reasonable verification step was completed or the limitation is explicitly stated.",
		"Do not say a feature is missing until checking whether it exists but is wired incorrectly, documented incorrectly, hidden behind config, loaded from another path, or insufficiently tested.",
		"",
		"# Command Safety",
		"Use non-destructive commands by default.",
		"Do not run destructive commands such as rm -rf, git reset --hard, git clean, force push, mass chmod, or broad rewrite commands unless the user explicitly requests them and the target is clear.",
		"Prefer read-only inspection before mutation.",
		"When running commands, avoid unnecessary long-running processes.",
		"If a command may be long-running or interactive, choose a bounded alternative when possible.",
		"",
		"# Final Answers",
		"When responding in the terminal, keep final answers concise.",
		"Markdown is allowed when it improves readability, but avoid decorative emoji and excessive detail.",
		"Final answers must be self-contained.",
		"After using tools or subagents, never give a final answer that only points to hidden context, such as 'above analysis', 'as shown above', '以上分析', '如上', or '前面已经说明'.",
		"Synthesize the concrete findings, edits, tests, and remaining risks in the final answer itself.",
		"When summarizing recent code changes, prefer git log, git status, git diff --stat, or the worklog first; do not run a full diff unless the user asks for exact diff details.",
		"For completed edits, include what changed, how it was verified, and any remaining risks or follow-ups.",
		"When no tools were used, answer directly without pretending to have inspected the workspace.",
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
	a.activateSkills(input)
	defer a.pruneTransientToolHistory()

	// Run-level timing: emit total duration when Run exits.
	runStartedAt := time.Now()
	defer func() {
		a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunTiming, DurationMS: time.Since(runStartedAt).Milliseconds()})
	}()

	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunStart})
	a.pruneStaleToolResults()
	a.maybeCompact(runCtx)
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeUserMessage, Message: input})
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	budget := newStepBudget(a.maxSteps, a.options.StepBudget)
	progressHistory := stepProgressHistory{}
	lastStep := 0
	stopReason := ""
	for step := 0; step < budget.limit; step++ {
		a.collectCompletedSubagentResults()
		lastStep = step

		// executeStep runs one iteration of the ReAct loop and returns the outcome.
		// Step timing is emitted via defer to ensure it fires on all paths.
		outcome := a.executeStep(runCtx, step, &progressHistory, &budget)

		switch outcome.kind {
		case stepOutcomeError:
			a.logTokenUsage()
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeError, Error: outcome.err.Error()})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			return "", outcome.err
		case stepOutcomeFinal:
			a.logTokenUsage()
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeRunEnd})
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeFinal, Message: outcome.final})
			return outcome.final, nil
		case stepOutcomeContinue:
			shouldContinue, reason := a.maybeExtendStepBudget(runCtx, step, &budget, progressHistory)
			if !shouldContinue {
				stopReason = reason
				break
			}
		}
	}
	summaryStep := lastStep + 1
	if final, ok, err := a.summarizeBudgetExhaustion(runCtx, summaryStep, stopReason); err == nil && ok {
		a.logTokenUsage()
		a.emit(runtimeevent.Event{Step: summaryStep, Type: runtimeevent.TypeRunEnd})
		a.emit(runtimeevent.Event{Step: summaryStep, Type: runtimeevent.TypeFinal, Message: final})
		return final, nil
	} else if err != nil {
		logs.Errorf("final summary after step budget exhaustion failed: step=%d err=%v", summaryStep, err)
	}
	err := fmt.Errorf("agent stopped after %d steps without a final response", budget.limit)
	if strings.TrimSpace(stopReason) != "" {
		err = fmt.Errorf("%w: %s", err, stopReason)
	}
	logs.Errorf("agent stopped without final response: max_steps=%d used_steps=%d", budget.limit, lastStep+1)
	a.logTokenUsage()
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeError, Error: err.Error()})
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeRunEnd})
	return "", err
}

// stepOutcomeKind classifies the result of a single ReAct step.
type stepOutcomeKind int

const (
	// stepOutcomeContinue means the step executed tools and the loop should continue.
	stepOutcomeContinue stepOutcomeKind = iota
	// stepOutcomeFinal means the agent produced a final answer.
	stepOutcomeFinal
	// stepOutcomeError means the step failed with an error.
	stepOutcomeError
)

// stepOutcome holds the result of executing one ReAct step.
type stepOutcome struct {
	kind  stepOutcomeKind
	final string
	err   error
}

// executeStep runs one iteration of the ReAct loop with timing guaranteed via defer.
// It returns the outcome (continue, final, or error) without emitting timing events
// itself — the deferred closure handles that uniformly.
func (a *Agent) executeStep(ctx context.Context, step int, progressHistory *stepProgressHistory, budget *stepBudget) stepOutcome {
	stepStartedAt := time.Now()
	// Defer ensures step timing is emitted on every exit path, including errors.
	defer func() {
		if a.options.StepTimingEnabled {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeStepTiming, DurationMS: time.Since(stepStartedAt).Milliseconds()})
		}
	}()

	resp, err := a.chatWithTools(ctx, step)
	if err != nil {
		logs.Errorf("agent chat failed: step=%d err=%v", step, err)
		return stepOutcome{kind: stepOutcomeError, err: err}
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
		waited, err := a.awaitOutstandingSubagents(ctx)
		if err != nil {
			logs.Errorf("await subagents failed: step=%d err=%v", step, err)
			return stepOutcome{kind: stepOutcomeError, err: err}
		}
		if waited {
			// Remove the assistant message we just added and let the loop continue.
			a.messages = append(a.messages[:assistantMessageIndex], a.messages[assistantMessageIndex+1:]...)
			return stepOutcome{kind: stepOutcomeContinue}
		}
		final := strings.TrimSpace(resp.Content)
		return stepOutcome{kind: stepOutcomeFinal, final: final}
	}

	if strings.TrimSpace(resp.Content) != "" {
		a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeAssistantMessage, Message: strings.TrimSpace(resp.Content)})
	}

	todosBefore := a.todoTool.Items()
	executedCalls := a.executeToolCalls(ctx, step, resp.ToolCalls)
	for _, executed := range executedCalls {
		a.messages = append(a.messages, llm.Message{
			Role:       "tool",
			ToolCallID: executed.call.ID,
			Content:    executed.result.JSON(),
		})
	}
	a.collectCompletedSubagentResults()
	progressHistory.record(stepProgressFromExecuted(resp.ToolCalls, executedCalls, todosBefore, a.todoTool.Items()))
	return stepOutcome{kind: stepOutcomeContinue}
}

func (a *Agent) chatWithTools(ctx context.Context, step int) (*llm.ChatResponse, error) {
	return a.chat(ctx, step, a.conversationMessages(), a.functionTools())
}

func (a *Agent) chat(ctx context.Context, step int, messages []llm.Message, tools []llm.FunctionTool) (*llm.ChatResponse, error) {
	for attempt := 0; ; attempt++ {
		resp, usedStreaming, sawVisibleDelta, err := a.chatOnce(ctx, step, messages, tools)
		if err == nil {
			a.recordChatUsage(step, resp, usedStreaming)
			return resp, nil
		}
		if !a.shouldRetryChatError(ctx, err, attempt, usedStreaming, sawVisibleDelta) {
			return nil, err
		}

		if usedStreaming && !sawVisibleDelta && !a.streamingDisabled {
			// If the stream failed before any visible output reached the UI,
			// fall back to non-streaming for the retry to avoid repeating SSE
			// transport issues on providers with flaky streaming paths.
			a.streamingDisabled = true
		}
		backoff := a.options.ChatRetry.Backoff
		a.emit(runtimeevent.Event{
			Step:       step,
			Type:       runtimeevent.TypeChatRetry,
			Message:    chatRetryStatusMessage(err, usedStreaming && !sawVisibleDelta),
			Error:      err.Error(),
			DurationMS: backoff.Milliseconds(),
			Count:      attempt + 1,
			After:      a.options.ChatRetry.MaxRetries,
		})
		logs.Infof(
			"agent chat retry scheduled: step=%d retry=%d/%d backoff=%s streaming_fallback=%t err=%v",
			step,
			attempt+1,
			a.options.ChatRetry.MaxRetries,
			backoff,
			usedStreaming && !sawVisibleDelta,
			err,
		)
		if err := sleepContext(ctx, backoff); err != nil {
			return nil, err
		}
	}
}

func (a *Agent) chatOnce(ctx context.Context, step int, messages []llm.Message, tools []llm.FunctionTool) (*llm.ChatResponse, bool, bool, error) {
	streamingClient, ok := a.client.(llm.StreamingClient)
	var resp *llm.ChatResponse
	var err error

	// Fall back to non-streaming when a previous streaming call returned no
	// usage data. Some providers (e.g. Bailian qwen) omit usage in SSE chunks.
	if !ok || a.streamingDisabled {
		resp, err = a.client.ChatWithTools(ctx, messages, tools)
	} else {
		sawVisibleDelta := false
		resp, err = streamingClient.ChatWithToolsStream(ctx, messages, tools, func(delta llm.StreamDelta) error {
			if strings.TrimSpace(delta.Content) == "" {
				return nil
			}
			sawVisibleDelta = true
			a.emit(runtimeevent.Event{
				Step:    step,
				Type:    runtimeevent.TypeAssistantDelta,
				Delta:   delta.Content,
				Message: delta.Content,
			})
			return nil
		})
		return resp, true, sawVisibleDelta, err
	}
	return resp, false, false, err
}

func (a *Agent) summarizeBudgetExhaustion(ctx context.Context, step int, stopReason string) (string, bool, error) {
	if ctx.Err() != nil || strings.TrimSpace(stopReason) == "" || stopReason == "context was cancelled" {
		return "", false, nil
	}
	a.collectCompletedSubagentResults()
	if _, err := a.awaitOutstandingSubagents(ctx); err != nil {
		return "", false, err
	}
	logs.Infof("requesting final summary after step budget exhaustion: step=%d reason=%s", step, stopReason)
	resp, err := a.chat(ctx, step, a.budgetSummaryMessages(stopReason), nil)
	if err != nil {
		return "", false, err
	}
	final := strings.TrimSpace(resp.Content)
	if final == "" {
		final = fmt.Sprintf("Action budget exhausted. Stop reason: %s", stopReason)
	}
	return final, true, nil
}

func (a *Agent) recordChatUsage(step int, resp *llm.ChatResponse, usedStreaming bool) {
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
	} else if usedStreaming && !a.streamingDisabled {
		// Streaming call returned without usage — disable streaming so the
		// next call uses the non-streaming path which usually includes usage.
		a.streamingDisabled = true
		logs.Infof("streaming returned no usage, falling back to non-streaming")
	}
}

func (a *Agent) shouldRetryChatError(ctx context.Context, err error, attempt int, usedStreaming bool, sawVisibleDelta bool) bool {
	if a.options.ChatRetry.MaxRetries <= 0 || attempt >= a.options.ChatRetry.MaxRetries {
		return false
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	if usedStreaming && sawVisibleDelta {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func chatRetryStatusMessage(err error, streamingFallback bool) string {
	base := "Temporary LLM transport failure."
	if errors.Is(err, context.DeadlineExceeded) {
		base = "LLM request timed out."
	}
	if streamingFallback {
		return base + " Retrying without streaming."
	}
	return base + " Waiting before retry."
}

func (a *Agent) activateSkills(input string) {
	a.activeSkills = nil
	a.skillContext = ""
	if a.skillRegistry == nil || !a.options.Skills.Enabled {
		return
	}
	candidates := a.skillRegistry.Retrieve(input, a.options.Skills.TopK, a.options.Skills.MinScore)
	if len(candidates) == 0 {
		return
	}
	a.activeSkills = make(map[string]skill.Candidate, len(candidates))
	lines := []string{
		"# Optional Skills",
		"The runtime retrieved optional skills for this user request. These skills are metadata-only right now; the full SKILL.md is loaded only if you call invoke_skill.",
		"Only call invoke_skill when one of these skills materially fits the task. If none fit, ignore this section.",
		"",
		"Available skills for this run:",
	}
	for _, candidate := range candidates {
		a.activeSkills[strings.ToLower(candidate.Skill.Name)] = candidate
		lines = append(lines,
			fmt.Sprintf("- %s (score=%d, source=%s)", candidate.Skill.Name, candidate.Score, candidate.Skill.Source),
			"  Description: "+candidate.Skill.Description,
			"  Input schema: "+candidate.Skill.InputSchemaSummary(),
			"  Permissions: "+candidate.Skill.PermissionSummary(),
			"  Trigger scenarios: "+candidate.Skill.TriggerSummary(),
		)
	}
	a.skillContext = strings.Join(lines, "\n")
}

func (a *Agent) conversationMessages() []llm.Message {
	if strings.TrimSpace(a.skillContext) == "" || len(a.messages) == 0 {
		return a.messages
	}
	out := make([]llm.Message, 0, len(a.messages)+1)
	out = append(out, a.messages[0])
	out = append(out, llm.Message{Role: "system", Content: a.skillContext})
	out = append(out, a.messages[1:]...)
	return out
}

func (a *Agent) budgetSummaryMessages(stopReason string) []llm.Message {
	messages := a.conversationMessages()
	out := make([]llm.Message, 0, len(messages)+1)
	out = append(out, messages...)
	lines := []string{
		"# Final Summary Only",
		"The action budget for this run is exhausted.",
		"Do not call tools in this turn.",
		"Summarize the current state for the user using only the information already gathered in the conversation.",
		"State clearly what is finished, what remains unresolved, any blockers, and the most useful next step.",
		"If the task is incomplete, say that explicitly.",
	}
	if reason := strings.TrimSpace(stopReason); reason != "" {
		lines = append(lines, "Stop reason: "+reason)
	}
	out = append(out, llm.Message{
		Role:    "system",
		Content: strings.Join(lines, "\n"),
	})
	return out
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
