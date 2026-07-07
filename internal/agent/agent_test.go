package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"local-agent/internal/approval"
	contextmgr "local-agent/internal/context"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/skill"
	"local-agent/internal/tools"
)

type fakeClient struct {
	responses []*llm.ChatResponse
	errors    []error
	calls     int
	messages  [][]llm.Message
	tools     [][]llm.FunctionTool
	stream    bool
	deltas    [][]string
}

func (f *fakeClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	f.messages = append(f.messages, append([]llm.Message(nil), messages...))
	f.tools = append(f.tools, append([]llm.FunctionTool(nil), specs...))
	if f.calls < len(f.errors) && f.errors[f.calls] != nil {
		err := f.errors[f.calls]
		f.calls++
		return nil, err
	}
	if f.calls >= len(f.responses) {
		f.calls++
		return &llm.ChatResponse{Content: "done"}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

func (f *fakeClient) ChatWithToolsStream(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool, onDelta llm.StreamHandler) (*llm.ChatResponse, error) {
	f.messages = append(f.messages, append([]llm.Message(nil), messages...))
	f.tools = append(f.tools, append([]llm.FunctionTool(nil), specs...))
	if onDelta != nil {
		var deltas []string
		if f.calls < len(f.deltas) {
			deltas = f.deltas[f.calls]
		}
		for _, delta := range deltas {
			if err := onDelta(llm.StreamDelta{Content: delta}); err != nil {
				return nil, err
			}
		}
	}
	if f.calls < len(f.errors) && f.errors[f.calls] != nil {
		err := f.errors[f.calls]
		f.calls++
		return nil, err
	}
	if f.calls >= len(f.responses) {
		f.calls++
		return &llm.ChatResponse{Content: "done"}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

type echoTool struct {
	calls []json.RawMessage
}

type namedTool struct {
	name  string
	calls []json.RawMessage
}

type blockingTool struct {
	name      string
	delay     time.Duration
	mu        sync.Mutex
	active    int
	maxActive int
	calls     []json.RawMessage
}

type captureRenderer struct {
	events []runtimeevent.Event
}

type fakeApprover struct {
	decision approval.Decision
	calls    []approval.Request
}

type subagentConcurrencyClient struct {
	mu              sync.Mutex
	parentCalls     int
	childCalls      int
	activeChildren  int
	maxActiveChild  int
	childDelay      time.Duration
	childToolCounts []int
}

type indexedSubagentClient struct {
	mu          sync.Mutex
	parentCalls int
}

type delegateScriptClient struct {
	mu                               sync.Mutex
	delegateTask                     string
	parentFinal                      string
	parentFollowupToolCalls          []llm.ToolCall
	childToolCalls                   []llm.ToolCall
	childFinal                       string
	childDelay                       time.Duration
	parentCalls                      int
	childCalls                       int
	childFinalReady                  bool
	parentContinuedWhileChildRunning bool
	messages                         [][]llm.Message
	tools                            [][]llm.FunctionTool
}

type skillScriptClient struct {
	mu          sync.Mutex
	parentCalls int
	childCalls  int
	parentFinal string
	childFinal  string
	messages    [][]llm.Message
	tools       [][]llm.FunctionTool
}

type lockingCaptureRenderer struct {
	mu     sync.Mutex
	events []runtimeevent.Event
}

func (r *captureRenderer) HandleEvent(event runtimeevent.Event) {
	r.events = append(r.events, event)
}

func (r *lockingCaptureRenderer) HandleEvent(event runtimeevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *lockingCaptureRenderer) Events() []runtimeevent.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runtimeevent.Event(nil), r.events...)
}

func (c *delegateScriptClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	c.mu.Lock()
	c.messages = append(c.messages, append([]llm.Message(nil), messages...))
	c.tools = append(c.tools, append([]llm.FunctionTool(nil), specs...))
	c.mu.Unlock()

	if len(messages) > 0 && strings.Contains(messages[0].Content, "read-only research subagent") {
		c.mu.Lock()
		c.childCalls++
		call := c.childCalls
		delay := c.childDelay
		toolCalls := append([]llm.ToolCall(nil), c.childToolCalls...)
		childFinal := c.childFinal
		c.mu.Unlock()

		if delay > 0 && call == 1 {
			time.Sleep(delay)
		}
		if call == 1 && len(toolCalls) > 0 {
			return &llm.ChatResponse{ToolCalls: toolCalls}, nil
		}

		c.mu.Lock()
		c.childFinalReady = true
		c.mu.Unlock()
		return &llm.ChatResponse{Content: childFinal}, nil
	}

	c.mu.Lock()
	c.parentCalls++
	call := c.parentCalls
	backgroundReady := messageSnapshotContains(messages, "Background delegate_task result.")
	childFinalReady := c.childFinalReady
	parentFinal := c.parentFinal
	delegateTask := c.delegateTask
	followup := append([]llm.ToolCall(nil), c.parentFollowupToolCalls...)
	if !childFinalReady && call >= 2 {
		c.parentContinuedWhileChildRunning = true
	}
	c.mu.Unlock()

	if backgroundReady {
		return &llm.ChatResponse{Content: parentFinal}, nil
	}
	if call == 1 {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{
			todoToolCall("todo_parent", "Delegate "+delegateTask),
			delegateTaskToolCall("delegate_1", delegateTask),
		}}, nil
	}
	if call == 2 && len(followup) > 0 {
		return &llm.ChatResponse{ToolCalls: followup}, nil
	}
	return &llm.ChatResponse{Content: "waiting on delegated work"}, nil
}

func (c *skillScriptClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	c.mu.Lock()
	c.messages = append(c.messages, append([]llm.Message(nil), messages...))
	c.tools = append(c.tools, append([]llm.FunctionTool(nil), specs...))
	c.mu.Unlock()

	if len(messages) > 0 && strings.Contains(messages[0].Content, `You are executing the on-demand skill "reviewer".`) {
		c.mu.Lock()
		c.childCalls++
		childFinal := c.childFinal
		c.mu.Unlock()
		return &llm.ChatResponse{Content: childFinal}, nil
	}

	c.mu.Lock()
	c.parentCalls++
	call := c.parentCalls
	parentFinal := c.parentFinal
	c.mu.Unlock()

	if call == 1 {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{
			todoToolCall("todo_skill", "Use reviewer skill"),
			testToolCall("skill_1", tools.InvokeSkillToolName, `{"name":"reviewer","input":{"focus":"bugs"}}`),
		}}, nil
	}
	return &llm.ChatResponse{Content: parentFinal}, nil
}

func (t *echoTool) Name() string {
	return "echo"
}

func (t *echoTool) Description() string {
	return "Echo input text."
}

func (t *echoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}`)
}

func (t *echoTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	t.calls = append(t.calls, append(json.RawMessage(nil), args...))
	var params struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.Error(err.Error()), nil
	}
	return tools.Success("echoed", params.Text), nil
}

func (t *namedTool) Name() string {
	return t.name
}

func (t *namedTool) Description() string {
	return "Named test tool."
}

func (t *namedTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *namedTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	t.calls = append(t.calls, append(json.RawMessage(nil), args...))
	return tools.Success("called", ""), nil
}

func (t *blockingTool) Name() string {
	return t.name
}

func (t *blockingTool) Description() string {
	return "Blocking test tool."
}

func (t *blockingTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *blockingTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	t.mu.Lock()
	t.calls = append(t.calls, append(json.RawMessage(nil), args...))
	t.active++
	if t.active > t.maxActive {
		t.maxActive = t.active
	}
	t.mu.Unlock()

	time.Sleep(t.delay)

	t.mu.Lock()
	t.active--
	t.mu.Unlock()
	return tools.Success("called", ""), nil
}

func (t *blockingTool) MaxActive() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.maxActive
}

func (t *blockingTool) CallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

func (a *fakeApprover) Approve(ctx context.Context, request approval.Request) approval.Decision {
	a.calls = append(a.calls, request)
	if a.decision == "" {
		return approval.DecisionAllow
	}
	return a.decision
}

func (c *subagentConcurrencyClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	if len(messages) > 0 && strings.Contains(messages[0].Content, "read-only research subagent") {
		c.mu.Lock()
		c.childCalls++
		c.activeChildren++
		if c.activeChildren > c.maxActiveChild {
			c.maxActiveChild = c.activeChildren
		}
		c.childToolCounts = append(c.childToolCounts, len(specs))
		c.mu.Unlock()

		time.Sleep(c.childDelay)

		c.mu.Lock()
		c.activeChildren--
		c.mu.Unlock()
		return &llm.ChatResponse{Content: "subagent result"}, nil
	}

	c.mu.Lock()
	c.parentCalls++
	call := c.parentCalls
	c.mu.Unlock()
	if call == 1 {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{
			todoToolCall("todo_1", "Delegate research"),
			delegateTaskToolCall("delegate_1", "research one"),
			delegateTaskToolCall("delegate_2", "research two"),
			delegateTaskToolCall("delegate_3", "research three"),
		}}, nil
	}
	return &llm.ChatResponse{Content: "parent done"}, nil
}

func (c *indexedSubagentClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	if len(messages) > 0 && strings.Contains(messages[0].Content, "read-only research subagent") {
		for _, message := range messages {
			if message.Role == "tool" {
				return &llm.ChatResponse{Content: "child done"}, nil
			}
		}
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{
			testToolCall("child_read", "read_file", `{"path":"README.md"}`),
		}}, nil
	}

	c.mu.Lock()
	c.parentCalls++
	call := c.parentCalls
	c.mu.Unlock()
	if call == 1 {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{
			todoToolCall("todo_1", "Delegate research"),
			delegateTaskToolCall("delegate_1", "research one"),
			delegateTaskToolCall("delegate_2", "research two"),
		}}, nil
	}
	return &llm.ChatResponse{Content: "parent done"}, nil
}

func TestRunExecutesNativeToolCallThenFinal(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Echo hello"),
				{
					ID:   "call_1",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "echo",
						Arguments: `{"text":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	tool := &echoTool{}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := New(client, registry, 3)
	answer, err := agent.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "finished" {
		t.Fatalf("answer = %q, want finished", answer)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if len(tool.calls) != 1 || string(tool.calls[0]) != `{"text":"hello"}` {
		t.Fatalf("tool calls = %q, want native arguments", tool.calls)
	}

	messages := agent.Messages()
	foundToolMessage := false
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == "call_1" && strings.Contains(message.Content, `"output":"hello"`) {
			foundToolMessage = true
		}
	}
	if !foundToolMessage {
		t.Fatalf("missing tool result message: %#v", messages)
	}
}

func TestRunDoesNotParseAssistantJSONTextAsToolCall(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: `{"name":"echo","arguments":{"text":"hello"}}`},
	}}
	tool := &echoTool{}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := New(client, registry, 3)
	answer, err := agent.Run(context.Background(), "return json")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != `{"name":"echo","arguments":{"text":"hello"}}` {
		t.Fatalf("answer = %q, want raw assistant content", answer)
	}
	if len(tool.calls) != 0 {
		t.Fatalf("tool calls = %d, want 0", len(tool.calls))
	}
}

func TestRunUsesPromptGuidanceInsteadOfHidingToolsForGreeting(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "Hello! How can I help?"},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})

	agent := New(client, registry, 3)
	answer, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Hello! How can I help?" {
		t.Fatalf("answer = %q, want greeting", answer)
	}
	if len(client.tools) != 1 {
		t.Fatalf("client tool snapshots = %d, want 1", len(client.tools))
	}
	if len(client.tools[0]) == 0 {
		t.Fatalf("tools were hidden for greeting; behavior should be prompt-guided")
	}
	if len(client.messages) != 1 || len(client.messages[0]) == 0 || client.messages[0][0].Role != "system" {
		t.Fatalf("missing system prompt in messages: %#v", client.messages)
	}
	systemPrompt := client.messages[0][0].Content
	for _, want := range []string{
		"# Role",
		"# Tool Use",
		"# Engineering Discipline",
		"# Delegation",
		"# Workspace Navigation",
		"# Final Answers",
		"Do not inspect the workspace for greetings",
		"Only call tools when the user asks for a concrete workspace action",
		"For multi-step coding, debugging, editing, or cross-file work",
		"For non-trivial code changes, call engineering_checklist before editing",
		"Do not call engineering_checklist for simple reads",
		"Simple single-step reads, lookups, or one-off commands do not need a todo list",
		"multiple tool calls in one assistant turn",
		"more than 10 non-planning tool calls",
		"claiming a feature or behavior is missing",
		"stale docs, missing regression tests, partial migration, or unreachable code path",
		"Classify implementation problems precisely",
		"concrete trigger path end-to-end",
		"without a reachable user path is incomplete",
		"verify both entry and exit paths",
		"actual event loop, command router, or UI state transitions",
		"git status --short -uall",
		"git diff --check",
		"required implementation files are tracked by git",
		"scratch or backup artifacts such as .orig, .bak",
		"success path, conflict or already-exists path",
		"first try to disprove that hypothesis",
		"Prefer the smallest complete fix",
		"simpler configurable or directly wired solution",
		"Complete Change Discipline",
		"end-to-end workflow changes",
		"Use engineering_checklist as executable guidance",
		"state owner, entrypoint, decision logic, output path",
		"wire the complete lifecycle",
		"easiest path while another valid state, caller, or execution mode bypasses",
		"screen space, context budget, concurrency slots, file paths, config scope, or step budget",
		"real entrypoint, the externally visible result, the important state transitions, and the side effects",
		"broad codebase analysis",
		"delegate before personally inspecting many files",
		"split it into focused delegate_task calls",
		"delegating multiple tasks in parallel",
		"Use workspace-relative paths",
		"Do not cd into guessed absolute paths",
		"use find_files first",
		"Use list_files only when the user asks to inspect one specific directory level",
		"under the current directory or under the workspace as recursive",
		"install, add, configure, wire, or connect a skill or MCP server",
		"Do not stop after downloading, cloning, or unpacking files",
		"`AGENT_CONFIG_FILE`, then workspace `./config.yaml`, then `~/.echo-dust-code/config.yaml`, otherwise built-in defaults",
		"Before writing `servers.json` or `registry.json`, prefer inspecting an existing example",
		"Skills are loaded from both the configured user skill dir and the configured project skill dir",
		"`~/.echo-dust-code/skills` and `<workspace>/skills`",
		"Root-level `registry.json` is recommended for retrieval quality, but a bare `SKILL.md` is still loadable",
		"Canonical `skills/registry.json` shape",
		"MCP servers are loaded from exactly one configured directory",
		"`~/.echo-dust-code/mcp`",
		"A workspace-local `servers.json` does nothing unless the active config points `mcp.dir`",
		"`~/.echo-dust-code/mcp/servers.json`",
		"Canonical `mcp/servers.json` shape",
		"`mcp.dir`",
		"Example: project skill install",
		"Example: current-agent MCP install",
		"Example: wrong pattern to avoid",
		"Markdown is allowed when it improves readability",
		"avoid decorative emoji",
		"do not run a full diff unless the user asks",
		"Final answers must be self-contained",
		"以上分析",
		"Synthesize the concrete findings",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestNewWithWorkspaceAddsCurrentWorkspaceToSystemPrompt(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "done"},
	}}
	registry := tools.NewRegistry()
	agent := NewWithWorkspace(client, registry, 3, "/tmp/local-agent-work")

	if _, err := agent.Run(context.Background(), "what is this workspace?"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.messages) != 1 || len(client.messages[0]) == 0 {
		t.Fatalf("missing messages: %#v", client.messages)
	}
	systemPrompt := client.messages[0][0].Content
	for _, want := range []string{
		"Current workspace: /tmp/local-agent-work",
		"Use workspace-relative paths for file tools",
		"Run commands in the configured workspace",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}
}

func TestNewWithWorkspaceAndOptionsAppendsSystemPromptSuffix(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "done"},
	}}
	options := DefaultOptions()
	options.SystemPromptSuffix = "# Memory\n\nProject rule."
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 3, "/tmp/local-agent-work", options)

	if _, err := agent.Run(context.Background(), "what memory is loaded?"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.messages) != 1 || len(client.messages[0]) == 0 {
		t.Fatalf("missing captured messages: %#v", client.messages)
	}
	systemPrompt := client.messages[0][0].Content
	if !strings.Contains(systemPrompt, "You are a general-purpose local coding agent.") {
		t.Fatalf("system prompt lost base content:\n%s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "# Memory\n\nProject rule.") {
		t.Fatalf("system prompt missing suffix:\n%s", systemPrompt)
	}
}

func TestRunExposesToolsForWorkspaceTask(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "done"},
	}}
	registry := tools.NewRegistry()
	tools.RegisterBuiltins(registry, t.TempDir())
	registry.Register(&echoTool{})

	agent := New(client, registry, 3)
	if _, err := agent.Run(context.Background(), "read go.mod"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.tools) != 1 || len(client.tools[0]) == 0 {
		t.Fatalf("tools were not exposed for workspace task: %#v", client.tools)
	}
	foundTodoTool := false
	foundChecklistTool := false
	for _, tool := range client.tools[0] {
		if tool.Name == tools.UpdateTodosToolName {
			foundTodoTool = true
		}
		if tool.Name == tools.EngineeringChecklistToolName {
			foundChecklistTool = true
		}
	}
	if !foundTodoTool {
		t.Fatalf("update_todos was not exposed: %#v", client.tools[0])
	}
	if !foundChecklistTool {
		t.Fatalf("engineering_checklist was not exposed: %#v", client.tools[0])
	}
	for _, name := range []string{"read_file_range", "find_symbol", "find_references", "find_callers", "find_callees", "git_status", "git_diff", "git_log"} {
		if !toolListContains(client.tools[0], name) {
			t.Fatalf("%s was not exposed: %#v", name, client.tools[0])
		}
	}
}

func TestEngineeringChecklistToolIsTransient(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("check", tools.EngineeringChecklistToolName, `{"task":"add mode","change_type":"feature","expected_behavior":"mode toggles and affects runtime behavior"}`),
			},
		},
		{Content: "finished"},
	}}
	agent := New(client, tools.NewRegistry(), 3)
	if _, err := agent.Run(context.Background(), "implement a mode"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, message := range agent.ConversationMessages() {
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				if call.Function.Name == tools.EngineeringChecklistToolName {
					t.Fatalf("engineering_checklist tool call leaked into conversation: %#v", agent.ConversationMessages())
				}
			}
		}
		if message.Role == "tool" && message.ToolCallID == "check" {
			t.Fatalf("engineering_checklist tool result leaked into conversation: %#v", agent.ConversationMessages())
		}
	}
}

func TestRunSimpleWorkspaceToolDoesNotAutoInitializeTodo(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("call_read", "read_file", `{"path":"README.md"}`),
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)
	renderer := &captureRenderer{}

	agent := New(client, registry, 3)
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "read README"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tool.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(tool.calls))
	}
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeTodoUpdate {
			t.Fatalf("simple read should not auto-create todo event: %#v", renderer.events)
		}
	}
	if len(renderer.events) < 3 || renderer.events[2].Type != runtimeevent.TypeToolCall {
		t.Fatalf("events = %#v, want tool call without synthetic todo", renderer.events)
	}
}

func TestRunProcessesTodoBeforeOtherToolsInSameTurn(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("call_read", "read_file", `{"path":"README.md"}`),
				todoToolCall("todo_late", "Read README"),
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)
	renderer := &captureRenderer{}

	agent := New(client, registry, 3)
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "read README"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tool.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(tool.calls))
	}
	if len(renderer.events) < 5 || renderer.events[2].Type != runtimeevent.TypeTodoUpdate || renderer.events[3].Type != runtimeevent.TypeToolCall {
		t.Fatalf("events = %#v, want todo update before tool call", renderer.events)
	}

	messages := agent.Messages()
	foundReadResult := false
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == "call_read" {
			foundReadResult = true
		}
		if message.Role == "tool" && message.ToolCallID == "todo_late" {
			t.Fatalf("todo tool result should not persist after run: %#v", messages)
		}
	}
	if !foundReadResult {
		t.Fatalf("missing read tool result: %#v", messages)
	}
}

func TestRunResetsTodoListForEachUserInput(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Read once"),
				testToolCall("call_read_1", "read_file", `{"path":"a.txt"}`),
			},
		},
		{Content: "first finished"},
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("call_read_2", "read_file", `{"path":"b.txt"}`),
			},
		},
		{Content: "second finished"},
	}}
	tool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := New(client, registry, 6)
	if _, err := agent.Run(context.Background(), "read a"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if _, err := agent.Run(context.Background(), "read b"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(tool.calls) != 2 {
		t.Fatalf("tool calls = %d, want both runs to execute", len(tool.calls))
	}
	if len(client.messages) < 3 {
		t.Fatalf("client messages snapshots = %d, want at least 3", len(client.messages))
	}
	for _, message := range client.messages[2] {
		if strings.Contains(message.Content, "Read once") {
			t.Fatalf("previous run todo leaked into second request messages: %#v", client.messages[2])
		}
		for _, call := range message.ToolCalls {
			if tools.IsUpdateTodosTool(call.Function.Name) {
				t.Fatalf("previous update_todos tool call leaked into second request messages: %#v", client.messages[2])
			}
		}
	}
}

func TestRunUnknownToolAddsErrorResultAndContinues(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_missing",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "missing",
						Arguments: `{}`,
					},
				},
			},
		},
		{Content: "used fallback"},
	}}

	agent := New(client, tools.NewRegistry(), 3)
	answer, err := agent.Run(context.Background(), "call missing")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "used fallback" {
		t.Fatalf("answer = %q, want used fallback", answer)
	}

	messages := agent.Messages()
	if got := messages[len(messages)-2]; got.Role != "tool" || !strings.Contains(got.Content, "unknown tool") {
		t.Fatalf("unknown tool result message = %#v", got)
	}
}

func TestRunEmitsToolAndFinalEvents(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			Content: "I will echo first.",
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Echo hello"),
				{
					ID:   "call_1",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "echo",
						Arguments: `{"text":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}

	agent := New(client, registry, 3)
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "say hello"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := make([]runtimeevent.Type, 0, len(renderer.events))
	for _, event := range renderer.events {
		got = append(got, event.Type)
	}
	want := []runtimeevent.Type{
		runtimeevent.TypeRunStart,
		runtimeevent.TypeUserMessage,
		runtimeevent.TypeAssistantMessage,
		runtimeevent.TypeTodoUpdate,
		runtimeevent.TypeToolCall,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeRunEnd,
		runtimeevent.TypeFinal,
		runtimeevent.TypeRunTiming,
	}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
	if renderer.events[1].Message != "say hello" {
		t.Fatalf("user event = %#v", renderer.events[1])
	}
	if len(renderer.events[3].Todos) != 1 || renderer.events[3].Todos[0].Text != "Echo hello" {
		t.Fatalf("todo event = %#v", renderer.events[3])
	}
	if renderer.events[4].Tool != "echo" || string(renderer.events[4].Args) != `{"text":"hello"}` {
		t.Fatalf("tool call event = %#v", renderer.events[4])
	}
	if renderer.events[5].Result == nil || renderer.events[5].Result.Output != "hello" {
		t.Fatalf("tool result event = %#v", renderer.events[5])
	}
	// Run timing is emitted via defer after TypeFinal, so it's the last event.
	lastIdx := len(renderer.events) - 1
	if renderer.events[lastIdx].Type != runtimeevent.TypeRunTiming || renderer.events[lastIdx].DurationMS < 0 {
		t.Fatalf("run timing event = %#v", renderer.events[lastIdx])
	}
}

func TestRunEmitsStepTimingWhenEnabled(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Echo hello"),
				{
					ID:   "call_echo",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "echo",
						Arguments: `{"text":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}

	options := DefaultOptions()
	options.StepTimingEnabled = true
	agent := NewWithWorkspaceAndOptions(client, registry, 3, "/tmp/work", options)
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "say hello"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	stepTimingCount := 0
	runTimingCount := 0
	for _, event := range renderer.events {
		switch event.Type {
		case runtimeevent.TypeStepTiming:
			stepTimingCount++
			if event.DurationMS < 0 {
				t.Fatalf("step timing event = %#v", event)
			}
		case runtimeevent.TypeRunTiming:
			runTimingCount++
			if event.DurationMS < 0 {
				t.Fatalf("run timing event = %#v", event)
			}
		}
	}
	if stepTimingCount != 2 {
		t.Fatalf("step timing count = %d, want 2", stepTimingCount)
	}
	if runTimingCount != 1 {
		t.Fatalf("run timing count = %d, want 1", runTimingCount)
	}
}

func TestRunEmitsAssistantDeltaBeforeFinal(t *testing.T) {
	client := &fakeClient{
		responses: []*llm.ChatResponse{
			{Content: "hello world"},
		},
		deltas: [][]string{
			{"hello ", "world"},
		},
	}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}

	agent := New(client, registry, 2)
	agent.SetRenderer(renderer)
	answer, err := agent.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "hello world" {
		t.Fatalf("answer = %q, want hello world", answer)
	}

	foundDelta := false
	foundFinal := false
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeAssistantDelta && strings.TrimSpace(event.Delta) != "" {
			foundDelta = true
		}
		if event.Type == runtimeevent.TypeFinal && event.Message == "hello world" {
			foundFinal = true
		}
	}
	if !foundDelta {
		t.Fatalf("expected assistant delta event, got %#v", renderer.events)
	}
	if !foundFinal {
		t.Fatalf("expected final event, got %#v", renderer.events)
	}
}

func TestRunRetriesNonStreamingChatTimeout(t *testing.T) {
	client := &fakeClient{
		responses: []*llm.ChatResponse{
			nil,
			{Content: "retried answer"},
		},
		errors: []error{
			context.DeadlineExceeded,
		},
	}
	options := DefaultOptions()
	options.ChatRetry.MaxRetries = 1
	options.ChatRetry.Backoff = time.Millisecond

	renderer := &captureRenderer{}
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 2, "/tmp/project", options)
	agent.streamingDisabled = true
	agent.SetRenderer(renderer)

	answer, err := agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "retried answer" {
		t.Fatalf("answer = %q, want retried answer", answer)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	foundRetry := false
	for _, event := range renderer.events {
		if event.Type != runtimeevent.TypeChatRetry {
			continue
		}
		foundRetry = true
		if event.Count != 1 || event.After != 1 {
			t.Fatalf("retry event counts = %d/%d, want 1/1", event.Count, event.After)
		}
		if event.DurationMS != time.Millisecond.Milliseconds() {
			t.Fatalf("retry backoff = %dms, want %dms", event.DurationMS, time.Millisecond.Milliseconds())
		}
		if !strings.Contains(event.Message, "timed out") {
			t.Fatalf("retry message = %q, want timeout hint", event.Message)
		}
	}
	if !foundRetry {
		t.Fatalf("expected chat retry event, got %#v", renderer.events)
	}
}

func TestRunDoesNotRetryStreamingFailureAfterVisibleDelta(t *testing.T) {
	client := &fakeClient{
		responses: []*llm.ChatResponse{
			nil,
			{Content: "should not be reached"},
		},
		errors: []error{
			context.DeadlineExceeded,
		},
		deltas: [][]string{
			{"partial "},
		},
	}
	options := DefaultOptions()
	options.ChatRetry.MaxRetries = 1
	options.ChatRetry.Backoff = time.Millisecond

	renderer := &captureRenderer{}
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 2, "/tmp/project", options)
	agent.SetRenderer(renderer)

	if _, err := agent.Run(context.Background(), "hello"); err == nil {
		t.Fatal("Run() error = nil, want streaming failure")
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}

	foundDelta := false
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeAssistantDelta && strings.Contains(event.Delta, "partial") {
			foundDelta = true
			break
		}
	}
	if !foundDelta {
		t.Fatalf("expected assistant delta before failure, got %#v", renderer.events)
	}
}

func TestRunDoesNotAskApprovalForLowRiskTool(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Read README"),
				{
					ID:   "call_read",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "read_file",
						Arguments: `{"path":"README.md"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)
	approver := &fakeApprover{decision: approval.DecisionDeny}

	agent := New(client, registry, 3)
	agent.SetApprover(approver)
	if _, err := agent.Run(context.Background(), "read README"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(approver.calls) != 0 {
		t.Fatalf("approval calls = %d, want 0", len(approver.calls))
	}
	if len(tool.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(tool.calls))
	}
}

func TestRunDeniesHighRiskToolWithoutExecuting(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write file"),
				{
					ID:   "call_write",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "write_file",
						Arguments: `{"path":"hello.txt","content":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "write_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)
	renderer := &captureRenderer{}
	approver := &fakeApprover{decision: approval.DecisionDeny}

	agent := New(client, registry, 3)
	agent.SetRenderer(renderer)
	agent.SetApprover(approver)
	if _, err := agent.Run(context.Background(), "write file"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(approver.calls) != 1 || approver.calls[0].Category != approval.CategoryWorkspaceWrite {
		t.Fatalf("approval calls = %#v", approver.calls)
	}
	if len(tool.calls) != 0 {
		t.Fatalf("tool was executed after denial: %d", len(tool.calls))
	}

	got := make([]runtimeevent.Type, 0, len(renderer.events))
	for _, event := range renderer.events {
		got = append(got, event.Type)
	}
	want := []runtimeevent.Type{
		runtimeevent.TypeRunStart,
		runtimeevent.TypeUserMessage,
		runtimeevent.TypeTodoUpdate,
		runtimeevent.TypeApprovalRequest,
		runtimeevent.TypeApprovalDecision,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeRunEnd,
		runtimeevent.TypeFinal,
		runtimeevent.TypeRunTiming,
	}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

func TestRunAllowsAlwaysDecision(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write file"),
				{
					ID:   "call_write",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "write_file",
						Arguments: `{"path":"hello.txt","content":"hello"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "write_file"}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := New(client, registry, 3)
	agent.SetApprover(&fakeApprover{decision: approval.DecisionAlways})
	if _, err := agent.Run(context.Background(), "write file"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tool.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(tool.calls))
	}
}

func TestRunBlocksBlacklistedCommandBeforeApproval(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Run dangerous command"),
				{
					ID:   "call_danger",
					Type: "function",
					Function: llm.ToolFunction{
						Name:      "run_command",
						Arguments: `{"command":"sudo rm -rf /"}`,
					},
				},
			},
		},
		{Content: "finished"},
	}}
	tool := &namedTool{name: "run_command"}
	registry := tools.NewRegistry()
	registry.Register(tool)
	approver := &fakeApprover{decision: approval.DecisionAllow}

	agent := New(client, registry, 3)
	agent.SetApprover(approver)
	if _, err := agent.Run(context.Background(), "danger"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(approver.calls) != 0 {
		t.Fatalf("blacklisted command should not ask approval: %#v", approver.calls)
	}
	if len(tool.calls) != 0 {
		t.Fatalf("blacklisted command executed: %d", len(tool.calls))
	}
	messages := agent.Messages()
	if got := messages[len(messages)-2]; got.Role != "tool" || !strings.Contains(got.Content, "permanent safety policy") {
		t.Fatalf("blacklist result message = %#v", got)
	}
}

func TestRunExecutesReadOnlyToolCallsConcurrently(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Read files"),
				testToolCall("call_1", "read_file", `{"path":"a.txt"}`),
				testToolCall("call_2", "read_file", `{"path":"b.txt"}`),
			},
		},
		{Content: "finished"},
	}}
	tool := &blockingTool{name: "read_file", delay: 20 * time.Millisecond}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := New(client, registry, 3)
	if _, err := agent.Run(context.Background(), "read files"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if tool.CallCount() != 2 {
		t.Fatalf("tool calls = %d, want 2", tool.CallCount())
	}
	if tool.MaxActive() < 2 {
		t.Fatalf("max active = %d, want concurrent execution", tool.MaxActive())
	}
}

func TestRunRejectsToolCallsBeyondParallelLimit(t *testing.T) {
	toolCalls := []llm.ToolCall{todoToolCall("todo_1", "Read many files")}
	for i := 0; i < 12; i++ {
		toolCalls = append(toolCalls, testToolCall(
			fmt.Sprintf("call_%02d", i),
			"read_file",
			fmt.Sprintf(`{"path":"file-%02d.txt"}`, i),
		))
	}
	client := &fakeClient{responses: []*llm.ChatResponse{
		{ToolCalls: toolCalls},
		{Content: "finished"},
	}}
	tool := &blockingTool{name: "read_file", delay: 20 * time.Millisecond}
	registry := tools.NewRegistry()
	registry.Register(tool)
	options := DefaultOptions()
	options.MaxParallelToolCalls = 10

	agent := NewWithWorkspaceAndOptions(client, registry, 3, "/tmp/local-agent-work", options)
	if _, err := agent.Run(context.Background(), "read many files"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if tool.CallCount() != 10 {
		t.Fatalf("tool calls = %d, want 10", tool.CallCount())
	}
	if tool.MaxActive() > 10 {
		t.Fatalf("max active = %d, want <= 10", tool.MaxActive())
	}

	rejected := 0
	for _, message := range agent.Messages() {
		if message.Role == "tool" && strings.Contains(message.Content, "too many parallel tool calls") {
			rejected++
		}
	}
	if rejected != 2 {
		t.Fatalf("rejected overflow tool calls = %d, want 2; messages=%#v", rejected, agent.Messages())
	}
}

func TestRunExtendsStepBudgetWhenToolsStillProgress(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Echo values"),
				testToolCall("call_1", "echo", `{"text":"a"}`),
			},
		},
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("call_2", "echo", `{"text":"b"}`),
			},
		},
		{Content: "finished"},
	}}
	registry := tools.NewRegistry()
	echo := &echoTool{}
	registry.Register(echo)
	renderer := &captureRenderer{}
	options := DefaultOptions()
	options.StepBudget.AdaptiveEnabled = true
	options.StepBudget.MaxExtensions = 1
	options.StepBudget.ExtensionSize = 2
	options.StepBudget.AbsoluteMaxSteps = 4

	agent := NewWithWorkspaceAndOptions(client, registry, 1, "/tmp/workspace", options)
	agent.SetRenderer(renderer)
	answer, err := agent.Run(context.Background(), "echo twice")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "finished" {
		t.Fatalf("answer = %q, want finished", answer)
	}
	if len(echo.calls) != 2 {
		t.Fatalf("echo calls = %d, want 2", len(echo.calls))
	}

	foundExtension := false
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeStepBudgetExtend {
			foundExtension = true
			if event.Before != 1 || event.After != 3 || event.Count != 1 {
				t.Fatalf("step budget event = %#v", event)
			}
		}
	}
	if !foundExtension {
		t.Fatalf("missing step budget extension event: %#v", renderer.events)
	}
}

func TestRunExtendsStepBudgetWhenTodoPlanProgresses(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Plan workspace work"),
			},
		},
		{Content: "finished"},
	}}
	renderer := &captureRenderer{}
	options := DefaultOptions()
	options.StepBudget.AdaptiveEnabled = true
	options.StepBudget.MaxExtensions = 1
	options.StepBudget.ExtensionSize = 1
	options.StepBudget.AbsoluteMaxSteps = 2

	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 1, "/tmp/workspace", options)
	agent.SetRenderer(renderer)
	answer, err := agent.Run(context.Background(), "plan then answer")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "finished" {
		t.Fatalf("answer = %q, want finished", answer)
	}

	foundExtension := false
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeStepBudgetExtend && strings.Contains(event.Reason, "todo plan changed") {
			foundExtension = true
		}
	}
	if !foundExtension {
		t.Fatalf("missing todo step budget extension event: %#v", renderer.events)
	}
}

func TestRunSummarizesWhenStepBudgetStopsAtRepeatedToolLoop(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Echo repeated value"),
				testToolCall("call_1", "echo", `{"text":"same"}`),
			},
		},
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_2", "Echo repeated value"),
				testToolCall("call_2", "echo", `{"text":"same"}`),
			},
		},
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_3", "Echo repeated value"),
				testToolCall("call_3", "echo", `{"text":"same"}`),
			},
		},
		{Content: "Loop detected. I repeatedly called the same tool and did not make further progress."},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}
	options := DefaultOptions()
	options.StepBudget.AdaptiveEnabled = true
	options.StepBudget.MaxExtensions = 3
	options.StepBudget.ExtensionSize = 1
	options.StepBudget.AbsoluteMaxSteps = 6

	agent := NewWithWorkspaceAndOptions(client, registry, 1, "/tmp/workspace", options)
	agent.SetRenderer(renderer)
	answer, err := agent.Run(context.Background(), "loop")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Loop detected. I repeatedly called the same tool and did not make further progress." {
		t.Fatalf("answer = %q", answer)
	}
	if client.calls != 4 {
		t.Fatalf("client calls = %d, want 4", client.calls)
	}
	if len(client.tools) != 4 || len(client.tools[3]) != 0 {
		t.Fatalf("summary tools = %#v, want nil/empty", client.tools)
	}
	if len(client.messages) != 4 {
		t.Fatalf("client messages = %d, want 4", len(client.messages))
	}
	lastMessages := client.messages[3]
	if len(lastMessages) == 0 || lastMessages[len(lastMessages)-1].Role != "system" {
		t.Fatalf("summary messages = %#v", lastMessages)
	}
	lastPrompt := lastMessages[len(lastMessages)-1].Content
	for _, want := range []string{
		"# Final Summary Only",
		"The action budget for this run is exhausted.",
		"Do not call tools in this turn.",
		"Stop reason: repeated identical tool calls indicate a likely loop",
	} {
		if !strings.Contains(lastPrompt, want) {
			t.Fatalf("summary prompt missing %q:\n%s", want, lastPrompt)
		}
	}

	extensions := 0
	foundStop := false
	for _, event := range renderer.events {
		switch event.Type {
		case runtimeevent.TypeStepBudgetExtend:
			extensions++
		case runtimeevent.TypeStepBudgetStop:
			foundStop = true
			if !strings.Contains(event.Reason, "repeated identical tool calls") {
				t.Fatalf("stop event = %#v", event)
			}
		}
	}
	if extensions != 2 {
		t.Fatalf("extensions = %d, want 2", extensions)
	}
	if !foundStop {
		t.Fatalf("missing step budget stop event: %#v", renderer.events)
	}
}

func TestRunExecutesWorkspaceWritesToDifferentFilesConcurrentlyAfterSessionApproval(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write files"),
				testToolCall("call_a", "write_file", `{"path":"a.txt","content":"a"}`),
				testToolCall("call_b", "write_file", `{"path":"b.txt","content":"b"}`),
			},
		},
		{Content: "finished"},
	}}
	tool := &blockingTool{name: "write_file", delay: 20 * time.Millisecond}
	registry := tools.NewRegistry()
	registry.Register(tool)
	approver := &fakeApprover{decision: approval.DecisionAlways}

	agent := NewWithWorkspace(client, registry, 3, "/tmp/local-agent-work")
	agent.SetApprover(approval.NewMemoryApprover(approver))
	if _, err := agent.Run(context.Background(), "write files"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if tool.CallCount() != 2 {
		t.Fatalf("tool calls = %d, want 2", tool.CallCount())
	}
	if tool.MaxActive() < 2 {
		t.Fatalf("max active = %d, want different files to run concurrently", tool.MaxActive())
	}
	if len(approver.calls) != 1 {
		t.Fatalf("approval calls = %d, want 1 session approval", len(approver.calls))
	}
	request := approver.calls[0]
	if request.Scope != approval.ScopeSession || request.Key != approval.WorkspaceWriteApprovalKey() {
		t.Fatalf("approval request = %#v, want session workspace write approval", request)
	}
	if len(request.Options) != 2 || request.Options[0] != approval.DecisionAlways || request.Options[1] != approval.DecisionDeny {
		t.Fatalf("approval options = %#v, want always/deny", request.Options)
	}
}

func TestRunSkipsApprovalEventsWhenSessionApprovalIsReused(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write first file"),
				testToolCall("call_a", "write_file", `{"path":"a.txt","content":"a"}`),
			},
		},
		{Content: "finished first"},
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_2", "Write second file"),
				testToolCall("call_b", "write_file", `{"path":"b.txt","content":"b"}`),
			},
		},
		{Content: "finished second"},
	}}
	registry := tools.NewRegistry()
	registry.Register(&namedTool{name: "write_file"})
	approver := &fakeApprover{decision: approval.DecisionAlways}
	renderer := &captureRenderer{}

	agent := NewWithWorkspace(client, registry, 4, "/tmp/local-agent-work")
	agent.SetApprover(approval.NewMemoryApprover(approver))
	agent.SetRenderer(renderer)

	if _, err := agent.Run(context.Background(), "write first file"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if _, err := agent.Run(context.Background(), "write second file"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(approver.calls) != 1 {
		t.Fatalf("approval calls = %d, want 1 reused session approval", len(approver.calls))
	}
	requestEvents := 0
	decisionEvents := 0
	for _, event := range renderer.events {
		switch event.Type {
		case runtimeevent.TypeApprovalRequest:
			requestEvents++
		case runtimeevent.TypeApprovalDecision:
			decisionEvents++
		}
	}
	if requestEvents != 1 || decisionEvents != 1 {
		t.Fatalf("approval events = request:%d decision:%d, want exactly one visible approval pair", requestEvents, decisionEvents)
	}
}

func TestRunSerializesWorkspaceWritesToSameFile(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write same file"),
				testToolCall("call_a", "write_file", `{"path":"same.txt","content":"a"}`),
				testToolCall("call_b", "write_file", `{"path":"same.txt","content":"b"}`),
			},
		},
		{Content: "finished"},
	}}
	tool := &blockingTool{name: "write_file", delay: 20 * time.Millisecond}
	registry := tools.NewRegistry()
	registry.Register(tool)

	agent := NewWithWorkspace(client, registry, 3, "/tmp/local-agent-work")
	agent.SetApprover(approval.NewMemoryApprover(&fakeApprover{decision: approval.DecisionAlways}))
	if _, err := agent.Run(context.Background(), "write same file"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if tool.CallCount() != 2 {
		t.Fatalf("tool calls = %d, want 2", tool.CallCount())
	}
	if tool.MaxActive() != 1 {
		t.Fatalf("max active = %d, want same file writes serialized", tool.MaxActive())
	}
}

func TestRunUsesLoopScopedApprovalForExternalWrites(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
				todoToolCall("todo_1", "Write outside workspace"),
				testToolCall("call_a", "run_command", `{"command":"echo a > /tmp/out-a"}`),
				testToolCall("call_b", "run_command", `{"command":"echo b > /tmp/out-b"}`),
			},
		},
		{
			ToolCalls: []llm.ToolCall{
				testToolCall("call_c", "run_command", `{"command":"echo c > /tmp/out-c"}`),
			},
		},
		{Content: "finished"},
	}}
	tool := &blockingTool{name: "run_command", delay: time.Millisecond}
	registry := tools.NewRegistry()
	registry.Register(tool)
	approver := &fakeApprover{decision: approval.DecisionAlways}

	agent := NewWithWorkspace(client, registry, 4, "/home/project")
	agent.SetApprover(approval.NewMemoryApprover(approver))
	if _, err := agent.Run(context.Background(), "write outside workspace"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if tool.CallCount() != 3 {
		t.Fatalf("tool calls = %d, want 3", tool.CallCount())
	}
	if len(approver.calls) != 2 {
		t.Fatalf("approval calls = %d, want one per loop", len(approver.calls))
	}
	for _, request := range approver.calls {
		if request.Scope != approval.ScopeLoop || request.Key != approval.ExternalWriteApprovalKey() {
			t.Fatalf("approval request = %#v, want loop external write approval", request)
		}
	}
}

func TestDelegateTaskUsesIsolatedSubagentAndReturnsOnlyFinalText(t *testing.T) {
	client := &delegateScriptClient{
		delegateTask: "Find what README says about this project",
		parentFinal:  "parent finished",
		childToolCalls: []llm.ToolCall{
			todoToolCall("todo_child", "Read README"),
			testToolCall("child_read", "read_file", `{"path":"README.md"}`),
		},
		childFinal: "README says this is a local agent.",
	}
	readTool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(readTool)

	agent := NewWithWorkspace(client, registry, 5, "/tmp/workspace")
	answer, err := agent.Run(context.Background(), "delegate the README check")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "parent finished" {
		t.Fatalf("answer = %q, want parent finished", answer)
	}
	if len(readTool.calls) != 1 {
		t.Fatalf("child read calls = %d, want 1", len(readTool.calls))
	}
	if !toolListContains(client.tools[0], tools.DelegateTaskToolName) {
		t.Fatalf("parent tools did not expose delegate_task: %#v", client.tools[0])
	}

	childSnapshot := findMessageSnapshot(client.messages, "read-only research subagent")
	if childSnapshot == nil {
		t.Fatalf("missing isolated child message snapshot: %#v", client.messages)
	}
	childPrompt := childSnapshot[0].Content
	for _, want := range []string{"# Role", "# Scope", "# Tool Use", "# Final Answer", "Do not spawn another subagent"} {
		if !strings.Contains(childPrompt, want) {
			t.Fatalf("child system prompt missing %q:\n%s", want, childPrompt)
		}
	}
	childTools := findToolSnapshotForSystemText(client.messages, client.tools, "read-only research subagent")
	if len(childTools) == 0 {
		t.Fatalf("missing child tool snapshot: %#v", client.tools)
	}
	if !toolListContains(childTools, tools.UpdateTodosToolName) || !toolListContains(childTools, "read_file") {
		t.Fatalf("child tools missing todo/read tools: %#v", childTools)
	}
	if toolListContains(childTools, tools.DelegateTaskToolName) {
		t.Fatalf("child tools exposed delegate_task: %#v", childTools)
	}
	for _, message := range childSnapshot {
		if strings.Contains(message.Content, "delegate the README check") {
			t.Fatalf("parent user request leaked into child messages: %#v", childSnapshot)
		}
	}

	foundDelegateResult := false
	foundInjectedResult := false
	for _, message := range agent.Messages() {
		if message.Role == "tool" && message.ToolCallID == "delegate_1" {
			foundDelegateResult = true
			if !strings.Contains(message.Content, subagentStartedSummary) {
				t.Fatalf("delegate start result missing async summary: %#v", message)
			}
			if strings.Contains(message.Content, "README says this is a local agent") {
				t.Fatalf("delegate tool message should not inline child final answer anymore: %#v", message)
			}
		}
		if message.Role == "system" && strings.Contains(message.Content, "Background delegate_task result.") {
			foundInjectedResult = true
			if !strings.Contains(message.Content, "README says this is a local agent.") {
				t.Fatalf("missing injected child final answer: %#v", message)
			}
		}
	}
	if !foundDelegateResult {
		t.Fatalf("missing delegate tool result: %#v", agent.Messages())
	}
	if !foundInjectedResult {
		t.Fatalf("missing injected async subagent result: %#v", agent.Messages())
	}
}

func TestDelegateTaskAutoInitializesSubagentTodo(t *testing.T) {
	client := &delegateScriptClient{
		delegateTask: "Inspect README",
		parentFinal:  "parent final",
		childToolCalls: []llm.ToolCall{
			testToolCall("child_read", "read_file", `{"path":"README.md"}`),
		},
		childFinal: "child final",
	}
	readTool := &namedTool{name: "read_file"}
	registry := tools.NewRegistry()
	registry.Register(readTool)

	agent := NewWithWorkspace(client, registry, 5, "/tmp/workspace")
	answer, err := agent.Run(context.Background(), "delegate readme")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "parent final" {
		t.Fatalf("answer = %q, want parent final", answer)
	}
	if len(readTool.calls) != 1 {
		t.Fatalf("child read calls = %d, want 1", len(readTool.calls))
	}
	if messageSnapshotsContain(client.messages, "requires a todo list") {
		t.Fatalf("child should not be blocked by todo gate: %#v", client.messages)
	}
}

func TestDelegateTaskSubagentKeepsDangerousCommandOnSafetyPath(t *testing.T) {
	client := &delegateScriptClient{
		delegateTask: "Try the command only if safe",
		parentFinal:  "parent finished",
		childToolCalls: []llm.ToolCall{
			todoToolCall("todo_child", "Check dangerous command"),
			testToolCall("child_cmd", "run_command", `{"command":"sudo rm -rf /"}`),
		},
		childFinal: "The command was blocked by policy.",
	}
	runTool := &namedTool{name: "run_command"}
	registry := tools.NewRegistry()
	registry.Register(runTool)

	agent := NewWithWorkspace(client, registry, 5, "/tmp/workspace")
	if _, err := agent.Run(context.Background(), "delegate command safety check"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(runTool.calls) != 0 {
		t.Fatalf("dangerous child command executed: %d", len(runTool.calls))
	}
	if !messageSnapshotsContain(client.messages, "permanent safety policy") {
		t.Fatalf("child messages did not include safety rejection: %#v", client.messages)
	}
}

func TestDelegateTaskForwardsSubagentToolEventsToParentRenderer(t *testing.T) {
	client := &delegateScriptClient{
		delegateTask: "Inspect README",
		parentFinal:  "parent final",
		childToolCalls: []llm.ToolCall{
			todoToolCall("todo_child", "Read README"),
			testToolCall("child_read", "read_file", `{"path":"README.md"}`),
		},
		childFinal: "child final",
	}
	registry := tools.NewRegistry()
	registry.Register(&namedTool{name: "read_file"})
	renderer := &captureRenderer{}

	agent := NewWithWorkspace(client, registry, 5, "/tmp/workspace")
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "delegate readme"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	foundSubagentRead := false
	for _, event := range renderer.events {
		if event.Source == "subagent" && event.Tool == "read_file" && event.Type == runtimeevent.TypeToolCall {
			foundSubagentRead = true
			if event.ParentTool != "Inspect README" {
				t.Fatalf("subagent parent task = %q", event.ParentTool)
			}
			if event.SubagentIndex != 1 {
				t.Fatalf("subagent index = %d, want 1", event.SubagentIndex)
			}
		}
		if event.Source == "subagent" && (event.Type == runtimeevent.TypeRunStart || event.Type == runtimeevent.TypeRunEnd || event.Type == runtimeevent.TypeFinal || event.Type == runtimeevent.TypeTodoUpdate) {
			t.Fatalf("subagent frame/todo/final event should not be forwarded: %#v", event)
		}
	}
	if !foundSubagentRead {
		t.Fatalf("missing forwarded subagent read event: %#v", renderer.events)
	}
}

func TestDelegateTaskRunsAsyncWhileParentContinuesIndependentWork(t *testing.T) {
	client := &delegateScriptClient{
		delegateTask: "Inspect README",
		parentFinal:  "parent final",
		parentFollowupToolCalls: []llm.ToolCall{
			testToolCall("parent_git", "git_status", `{}`),
		},
		childToolCalls: []llm.ToolCall{
			todoToolCall("todo_child", "Read README"),
			testToolCall("child_read", "read_file", `{"path":"README.md"}`),
		},
		childFinal: "child final",
		childDelay: 20 * time.Millisecond,
	}
	readTool := &namedTool{name: "read_file"}
	gitStatusTool := &namedTool{name: "git_status"}
	registry := tools.NewRegistry()
	registry.Register(readTool)
	registry.Register(gitStatusTool)

	agent := NewWithWorkspace(client, registry, 6, "/tmp/workspace")
	answer, err := agent.Run(context.Background(), "delegate and keep working")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "parent final" {
		t.Fatalf("answer = %q, want parent final", answer)
	}
	if len(gitStatusTool.calls) != 1 {
		t.Fatalf("parent follow-up tool calls = %d, want 1", len(gitStatusTool.calls))
	}
	if !client.parentContinuedWhileChildRunning {
		t.Fatalf("parent did not continue while child was still running: parentCalls=%d childCalls=%d snapshots=%#v", client.parentCalls, client.childCalls, client.messages)
	}
}

func TestDelegateTaskAssignsStableSubagentIndexesByToolCallOrder(t *testing.T) {
	client := &indexedSubagentClient{}
	registry := tools.NewRegistry()
	registry.Register(&blockingTool{name: "read_file", delay: 5 * time.Millisecond})
	renderer := &lockingCaptureRenderer{}

	agent := NewWithWorkspace(client, registry, 5, "/tmp/workspace")
	agent.SetRenderer(renderer)
	if _, err := agent.Run(context.Background(), "delegate two reads"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	indexByTask := map[string]int{}
	for _, event := range renderer.Events() {
		if event.Source == "subagent" && event.Type == runtimeevent.TypeToolCall && event.Tool == "read_file" {
			indexByTask[event.ParentTool] = event.SubagentIndex
		}
	}
	if indexByTask["research one"] != 1 || indexByTask["research two"] != 2 {
		t.Fatalf("subagent indexes = %#v, want research one=1 and research two=2", indexByTask)
	}
}

func TestDelegateTaskLimitsConcurrentSubagents(t *testing.T) {
	client := &subagentConcurrencyClient{childDelay: 20 * time.Millisecond}
	options := DefaultOptions()
	options.Subagents.MaxConcurrent = 2
	options.Subagents.MaxSteps = 2

	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 4, "/tmp/workspace", options)
	if _, err := agent.Run(context.Background(), "delegate three research tasks"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.childCalls != 3 {
		t.Fatalf("child calls = %d, want 3", client.childCalls)
	}
	if client.maxActiveChild > 2 {
		t.Fatalf("max active child subagents = %d, want <= 2", client.maxActiveChild)
	}
	if client.maxActiveChild < 2 {
		t.Fatalf("max active child subagents = %d, want concurrency", client.maxActiveChild)
	}
}

func todoToolCall(id string, text string) llm.ToolCall {
	return testToolCall(id, tools.UpdateTodosToolName, fmt.Sprintf(`{"items":[{"text":%q,"status":"in_progress"}]}`, text))
}

func delegateTaskToolCall(id string, task string) llm.ToolCall {
	return testToolCall(id, tools.DelegateTaskToolName, fmt.Sprintf(`{"task":%q}`, task))
}

func testToolCall(id string, name string, arguments string) llm.ToolCall {
	return llm.ToolCall{
		ID:   id,
		Type: "function",
		Function: llm.ToolFunction{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func toolListContains(specs []llm.FunctionTool, name string) bool {
	for _, spec := range specs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func findMessageSnapshot(snapshots [][]llm.Message, systemText string) []llm.Message {
	for _, snapshot := range snapshots {
		if len(snapshot) > 0 && strings.Contains(snapshot[0].Content, systemText) {
			return snapshot
		}
	}
	return nil
}

func messageSnapshotsContain(snapshots [][]llm.Message, text string) bool {
	for _, snapshot := range snapshots {
		if messageSnapshotContains(snapshot, text) {
			return true
		}
	}
	return false
}

func findToolSnapshotForSystemText(messageSnapshots [][]llm.Message, toolSnapshots [][]llm.FunctionTool, systemText string) []llm.FunctionTool {
	for i, snapshot := range messageSnapshots {
		if len(snapshot) == 0 || !strings.Contains(snapshot[0].Content, systemText) {
			continue
		}
		if i < len(toolSnapshots) {
			return toolSnapshots[i]
		}
	}
	return nil
}

func messageSnapshotContains(snapshot []llm.Message, text string) bool {
	for _, message := range snapshot {
		if strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}

func TestRunInjectsSkillContextAndExposesInvokeSkill(t *testing.T) {
	workdir := t.TempDir()
	skillRegistry := createTestSkillRegistry(t, workdir, testSkillSpec{
		Name:        "reviewer",
		Description: "Review code changes for bugs and regressions.",
		Triggers:    []string{"code review", "review diff"},
		InputSchema: `{"type":"object","properties":{"focus":{"type":"string"}},"additionalProperties":false}`,
		Tools:       []string{"read_file"},
		Body:        "Inspect the requested files and report the biggest risks.",
	})
	client := &skillScriptClient{
		parentFinal: "done",
		childFinal:  "review completed",
	}
	registry := tools.NewRegistry()
	registry.Register(&namedTool{name: "read_file"})
	options := DefaultOptions()
	options.Skills.Registry = skillRegistry
	agent := NewWithWorkspaceAndOptions(client, registry, 4, workdir, options)

	if _, err := agent.Run(context.Background(), "please review this diff for bugs"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.messages) < 2 {
		t.Fatalf("message snapshots = %d, want >= 2", len(client.messages))
	}
	if !messageSnapshotContains(client.messages[0], "# Optional Skills") {
		t.Fatalf("parent messages missing skill context: %#v", client.messages[0])
	}
	if !messageSnapshotContains(client.messages[0], "reviewer") {
		t.Fatalf("parent messages missing reviewer metadata: %#v", client.messages[0])
	}
	if !toolSnapshotContains(client.tools[0], tools.InvokeSkillToolName) {
		t.Fatalf("parent tools missing invoke_skill: %#v", client.tools[0])
	}
	if !toolSnapshotContains(client.tools[1], "read_file") {
		t.Fatalf("skill child tools missing read_file: %#v", client.tools[1])
	}
	if toolSnapshotContains(client.tools[1], tools.InvokeSkillToolName) {
		t.Fatalf("skill child tools unexpectedly exposed invoke_skill: %#v", client.tools[1])
	}
	if !messageSnapshotContains(client.messages[1], "Inspect the requested files and report the biggest risks.") {
		t.Fatalf("skill body was not loaded into child prompt: %#v", client.messages[1])
	}
}

func TestRunResetsSkillContextBetweenRequests(t *testing.T) {
	workdir := t.TempDir()
	skillRegistry := createTestSkillRegistry(t, workdir, testSkillSpec{
		Name:        "reviewer",
		Description: "Review code changes for bugs and regressions.",
		Triggers:    []string{"code review"},
		InputSchema: `{"type":"object","additionalProperties":true}`,
		Body:        "Review things.",
	})
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "done"},
		{Content: "done"},
	}}
	options := DefaultOptions()
	options.Skills.Registry = skillRegistry
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 2, workdir, options)

	if _, err := agent.Run(context.Background(), "please do a code review"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if _, err := agent.Run(context.Background(), "say hello"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if !messageSnapshotContains(client.messages[0], "# Optional Skills") {
		t.Fatalf("first request missing skill context: %#v", client.messages[0])
	}
	if messageSnapshotContains(client.messages[1], "# Optional Skills") {
		t.Fatalf("second request unexpectedly retained skill context: %#v", client.messages[1])
	}
}

func TestRunSkillRejectsUnavailablePermissionTool(t *testing.T) {
	workdir := t.TempDir()
	skillRegistry := createTestSkillRegistry(t, workdir, testSkillSpec{
		Name:        "reviewer",
		Description: "Review code changes for bugs and regressions.",
		Triggers:    []string{"code review"},
		InputSchema: `{"type":"object","additionalProperties":true}`,
		Tools:       []string{"read_file"},
		Body:        "Review things.",
	})
	options := DefaultOptions()
	options.Skills.Registry = skillRegistry
	agent := NewWithWorkspaceAndOptions(&fakeClient{}, tools.NewRegistry(), 2, workdir, options)
	agent.activateSkills("code review")

	result := agent.runSkill(context.Background(), json.RawMessage(`{"name":"reviewer","input":{}}`))
	if result.Status != "error" || !strings.Contains(result.Summary, `requires unavailable tool "read_file"`) {
		t.Fatalf("runSkill result = %#v, want missing tool error", result)
	}
}

func TestRunEmitsTokenUsageEventsWithCumulativeTotal(t *testing.T) {
	// Three LLM rounds with distinct usage values.
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			Content:   "",
			ToolCalls: []llm.ToolCall{testToolCall("call_1", "echo", `{"text":"a"}`)},
			Usage:     &llm.TokenUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30},
		},
		{
			Content:   "",
			ToolCalls: []llm.ToolCall{testToolCall("call_2", "echo", `{"text":"b"}`)},
			Usage:     &llm.TokenUsage{PromptTokens: 200, CompletionTokens: 40, TotalTokens: 240, CachedTokens: 40},
		},
		{
			Content: "finished",
			Usage:   &llm.TokenUsage{PromptTokens: 300, CompletionTokens: 60, TotalTokens: 360, CachedTokens: 50},
		},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}
	agent := NewWithWorkspace(client, registry, 5, "")
	agent.SetRenderer(renderer)

	if _, err := agent.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify cumulative token usage snapshot on the agent.
	usage := agent.TokenUsage()
	if usage.PromptTokens != 600 {
		t.Errorf("PromptTokens = %d, want 600", usage.PromptTokens)
	}
	if usage.CompletionTokens != 120 {
		t.Errorf("CompletionTokens = %d, want 120", usage.CompletionTokens)
	}
	if usage.TotalTokens != 720 {
		t.Errorf("TotalTokens = %d, want 720", usage.TotalTokens)
	}
	if usage.CachedTokens != 120 {
		t.Errorf("CachedTokens = %d, want 120", usage.CachedTokens)
	}

	// Verify token usage events carry correct per-call and cumulative values.
	var tokenEvents []runtimeevent.Event
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeTokenUsage {
			tokenEvents = append(tokenEvents, event)
		}
	}
	if len(tokenEvents) != 3 {
		t.Fatalf("token events = %d, want 3", len(tokenEvents))
	}
	wantPerCall := []struct {
		prompt, completion, cumulative, cached int
	}{
		{100, 20, 120, 30},
		{200, 40, 360, 40}, // 120 + 240
		{300, 60, 720, 50}, // 360 + 360
	}
	for i, want := range wantPerCall {
		got := tokenEvents[i]
		if got.PromptTokens != want.prompt {
			t.Errorf("event[%d].PromptTokens = %d, want %d", i, got.PromptTokens, want.prompt)
		}
		if got.CompletionTokens != want.completion {
			t.Errorf("event[%d].CompletionTokens = %d, want %d", i, got.CompletionTokens, want.completion)
		}
		if got.CumulativeTotal != want.cumulative {
			t.Errorf("event[%d].CumulativeTotal = %d, want %d", i, got.CumulativeTotal, want.cumulative)
		}
		if got.CachedTokens != want.cached {
			t.Errorf("event[%d].CachedTokens = %d, want %d", i, got.CachedTokens, want.cached)
		}
	}
}

func TestRunFallsBackToNonStreamingWhenUsageOmitted(t *testing.T) {
	// First response: streaming returns content but no usage.
	// Second response: non-streaming path returns content with usage.
	client := &fakeClient{
		responses: []*llm.ChatResponse{
			{
				Content:   "",
				ToolCalls: []llm.ToolCall{testToolCall("call_1", "echo", `{"text":"a"}`)},
				// Usage is nil — simulates Bailian streaming behavior.
			},
			{
				Content: "finished",
				Usage:   &llm.TokenUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			},
		},
	}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})
	renderer := &captureRenderer{}
	agent := NewWithWorkspace(client, registry, 5, "")
	agent.SetRenderer(renderer)

	if _, err := agent.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// After first streaming call with nil usage, streaming should be disabled.
	if !agent.streamingDisabled {
		t.Fatal("streamingDisabled should be true after streaming returns nil usage")
	}

	// Verify token usage was captured from the second (non-streaming) call.
	usage := agent.TokenUsage()
	if usage.TotalTokens != 120 {
		t.Errorf("TotalTokens = %d, want 120", usage.TotalTokens)
	}

	// Verify token event was emitted.
	var tokenEvents []runtimeevent.Event
	for _, event := range renderer.events {
		if event.Type == runtimeevent.TypeTokenUsage {
			tokenEvents = append(tokenEvents, event)
		}
	}
	if len(tokenEvents) != 1 {
		t.Fatalf("token events = %d, want 1", len(tokenEvents))
	}
	if tokenEvents[0].CumulativeTotal != 120 {
		t.Errorf("cumulative total = %d, want 120", tokenEvents[0].CumulativeTotal)
	}
}

func TestAgentContextMaintenanceEmitsPruneEvent(t *testing.T) {
	agent := New(&fakeClient{}, tools.NewRegistry(), 3)
	agent.options.Context.PruneToolResultMaxBytes = 8
	agent.options.Context.PruneKeepRecentMessages = 1
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_read", "read_file", `{"path":"big.txt"}`)}},
		{Role: "tool", ToolCallID: "call_read", Content: tools.Success("read", strings.Repeat("x", 40)).JSON()},
		{Role: "user", Content: "newer"},
	}
	renderer := &captureRenderer{}
	agent.SetRenderer(renderer)

	stats := agent.pruneStaleToolResults()
	if stats.Results != 1 {
		t.Fatalf("pruned results = %d, want 1", stats.Results)
	}
	if !strings.Contains(agent.messages[2].Content, contextmgr.PrunedToolOutputPrefix) {
		t.Fatalf("tool result was not pruned: %s", agent.messages[2].Content)
	}
	if !containsEvent(renderer.events, runtimeevent.TypeContextPruned) {
		t.Fatalf("missing context pruned event: %#v", renderer.events)
	}
}

func TestAgentCompactUsesSummarizerAndReplacesMessages(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{{Content: "Earlier work was summarized."}}}
	agent := NewWithWorkspaceAndOptions(client, tools.NewRegistry(), 3, "/tmp/work", compactTestOptions())
	agent.messages = []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: strings.Repeat("old user ", 80)},
		{Role: "assistant", Content: strings.Repeat("old assistant ", 80)},
		{Role: "user", Content: strings.Repeat("old decision ", 80)},
		{Role: "assistant", Content: strings.Repeat("old result ", 80)},
		{Role: "user", Content: strings.Repeat("recent prelude ", 200)},
		{Role: "assistant", ToolCalls: []llm.ToolCall{testToolCall("call_recent", "read_file", `{"path":"fresh.txt"}`)}},
		{Role: "tool", ToolCallID: "call_recent", Content: tools.Success("read", "fresh output").JSON()},
	}

	stats, err := agent.compact(context.Background(), true)
	if err != nil {
		t.Fatalf("compact() error = %v", err)
	}
	if stats.Messages == 0 {
		t.Fatalf("compacted message count = 0")
	}
	if len(agent.messages) != 4 {
		t.Fatalf("message count after compact = %d, want 4: %#v", len(agent.messages), agent.messages)
	}
	if !strings.Contains(agent.messages[1].Content, contextmgr.CompactionSummaryOpen) || !strings.Contains(agent.messages[1].Content, "Earlier work was summarized.") {
		t.Fatalf("missing summary message:\n%s", agent.messages[1].Content)
	}
}

func compactTestOptions() Options {
	options := DefaultOptions()
	options.Context.CompactKeepTailTokens = 80
	options.Context.CompactTargetPercent = 50
	options.Context.WindowTokens = 1000
	options.Context.CompactMinMessages = 1
	return options
}

func containsEvent(events []runtimeevent.Event, eventType runtimeevent.Type) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func toolSnapshotContains(snapshot []llm.FunctionTool, name string) bool {
	for _, tool := range snapshot {
		if tool.Name == name {
			return true
		}
	}
	return false
}

type testSkillSpec struct {
	Name        string
	Description string
	InputSchema string
	Triggers    []string
	Tools       []string
	Body        string
}

func createTestSkillRegistry(t *testing.T, workdir string, specs ...testSkillSpec) *skill.Registry {
	t.Helper()
	root := filepath.Join(workdir, "skills")
	registryEntries := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		dir := filepath.Join(root, spec.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir skill dir: %v", err)
		}
		schema := spec.InputSchema
		if strings.TrimSpace(schema) == "" {
			schema = `{"type":"object","additionalProperties":true}`
		}
		registryEntries = append(registryEntries, map[string]any{
			"path":         spec.Name,
			"name":         spec.Name,
			"description":  spec.Description,
			"input_schema": json.RawMessage(schema),
			"permissions": map[string]any{
				"tools": spec.Tools,
			},
			"triggers": spec.Triggers,
		})
		body := spec.Body
		if body == "" {
			body = "# Skill\n\nBody"
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write skill body: %v", err)
		}
	}
	registryData, err := json.MarshalIndent(map[string]any{"skills": registryEntries}, "", "  ")
	if err != nil {
		t.Fatalf("marshal skill registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "registry.json"), registryData, 0o644); err != nil {
		t.Fatalf("write skill registry: %v", err)
	}
	registry, err := skill.LoadRegistry(skill.Options{CWD: workdir, ProjectDir: "skills"})
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}
	return registry
}
