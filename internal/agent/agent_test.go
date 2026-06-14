package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type fakeClient struct {
	responses []*llm.ChatResponse
	calls     int
	messages  [][]llm.Message
	tools     [][]llm.FunctionTool
}

func (f *fakeClient) ChatWithTools(ctx context.Context, messages []llm.Message, specs []llm.FunctionTool) (*llm.ChatResponse, error) {
	f.messages = append(f.messages, append([]llm.Message(nil), messages...))
	f.tools = append(f.tools, append([]llm.FunctionTool(nil), specs...))
	if f.calls >= len(f.responses) {
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

func (r *captureRenderer) HandleEvent(event runtimeevent.Event) {
	r.events = append(r.events, event)
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
		"Do not inspect the workspace for greetings",
		"Only call tools when the user asks for a concrete workspace action",
		"call update_todos before any workspace tool",
		"multiple tool calls in one assistant turn",
		"Use workspace-relative paths",
		"Do not cd into guessed absolute paths",
		"use find_files first",
		"Use list_files only when the user asks to inspect one specific directory level",
		"under the current directory or under the workspace as recursive",
		"Markdown is allowed for final summaries",
		"avoid decorative emoji",
		"do not run a full diff unless the user asks",
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

func TestRunExposesToolsForWorkspaceTask(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{Content: "done"},
	}}
	registry := tools.NewRegistry()
	registry.Register(&echoTool{})

	agent := New(client, registry, 3)
	if _, err := agent.Run(context.Background(), "read go.mod"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.tools) != 1 || len(client.tools[0]) == 0 {
		t.Fatalf("tools were not exposed for workspace task: %#v", client.tools)
	}
	foundTodoTool := false
	for _, tool := range client.tools[0] {
		if tool.Name == updateTodosToolName {
			foundTodoTool = true
		}
	}
	if !foundTodoTool {
		t.Fatalf("update_todos was not exposed: %#v", client.tools[0])
	}
}

func TestRunBlocksWorkspaceToolBeforeTodoList(t *testing.T) {
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

	agent := New(client, registry, 3)
	if _, err := agent.Run(context.Background(), "read README"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(tool.calls) != 0 {
		t.Fatalf("tool calls = %d, want 0 before todo list", len(tool.calls))
	}
	messages := agent.Messages()
	if got := messages[len(messages)-2]; got.Role != "tool" || !strings.Contains(got.Content, "requires a todo list") {
		t.Fatalf("missing todo-required tool result: %#v", got)
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
	if len(renderer.events) < 4 || renderer.events[1].Type != runtimeevent.TypeTodoUpdate || renderer.events[2].Type != runtimeevent.TypeToolCall {
		t.Fatalf("events = %#v, want todo update before tool call", renderer.events)
	}

	messages := agent.Messages()
	if messages[len(messages)-3].ToolCallID != "call_read" || messages[len(messages)-2].ToolCallID != "todo_late" {
		t.Fatalf("tool result message order = %#v", messages)
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
	if len(tool.calls) != 1 {
		t.Fatalf("tool calls = %d, want only first run to execute", len(tool.calls))
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
		runtimeevent.TypeAssistantMessage,
		runtimeevent.TypeTodoUpdate,
		runtimeevent.TypeToolCall,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeRunEnd,
		runtimeevent.TypeFinal,
	}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
	if len(renderer.events[2].Todos) != 1 || renderer.events[2].Todos[0].Text != "Echo hello" {
		t.Fatalf("todo event = %#v", renderer.events[2])
	}
	if renderer.events[3].Tool != "echo" || string(renderer.events[3].Args) != `{"text":"hello"}` {
		t.Fatalf("tool call event = %#v", renderer.events[3])
	}
	if renderer.events[4].Result == nil || renderer.events[4].Result.Output != "hello" {
		t.Fatalf("tool result event = %#v", renderer.events[4])
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
		runtimeevent.TypeTodoUpdate,
		runtimeevent.TypeApprovalRequest,
		runtimeevent.TypeApprovalDecision,
		runtimeevent.TypeToolResult,
		runtimeevent.TypeRunEnd,
		runtimeevent.TypeFinal,
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

func TestParseTodoItemsValidatesInput(t *testing.T) {
	tests := []struct {
		name    string
		args    json.RawMessage
		wantErr string
	}{
		{name: "empty", args: json.RawMessage(`{"items":[]}`), wantErr: "at least one"},
		{name: "blank text", args: json.RawMessage(`{"items":[{"text":" ","status":"pending"}]}`), wantErr: "non-empty"},
		{name: "invalid status", args: json.RawMessage(`{"items":[{"text":"Read","status":"started"}]}`), wantErr: "invalid todo status"},
		{name: "multiple in progress", args: json.RawMessage(`{"items":[{"text":"Read","status":"in_progress"},{"text":"Write","status":"in_progress"}]}`), wantErr: "only one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTodoItems(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseTodoItems() error = %v, want %q", err, tt.wantErr)
			}
		})
	}

	items, err := parseTodoItems(json.RawMessage(`{"items":[{"text":" Read ","status":"in_progress"},{"text":"Write","status":"pending"}]}`))
	if err != nil {
		t.Fatalf("parseTodoItems() valid error = %v", err)
	}
	if len(items) != 2 || items[0].Text != "Read" || items[0].Status != runtimeevent.TodoInProgress {
		t.Fatalf("items = %#v", items)
	}
}

func todoToolCall(id string, text string) llm.ToolCall {
	return testToolCall(id, updateTodosToolName, fmt.Sprintf(`{"items":[{"text":%q,"status":"in_progress"}]}`, text))
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
