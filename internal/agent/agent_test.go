package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
		"Use workspace-relative paths",
		"Do not cd into guessed absolute paths",
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
		runtimeevent.TypeAssistantMessage,
		runtimeevent.TypeToolCall,
		runtimeevent.TypeToolResult,
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
	if renderer.events[1].Tool != "echo" || string(renderer.events[1].Args) != `{"text":"hello"}` {
		t.Fatalf("tool call event = %#v", renderer.events[1])
	}
	if renderer.events[2].Result == nil || renderer.events[2].Result.Output != "hello" {
		t.Fatalf("tool result event = %#v", renderer.events[2])
	}
}

func TestRunDoesNotAskApprovalForLowRiskTool(t *testing.T) {
	client := &fakeClient{responses: []*llm.ChatResponse{
		{
			ToolCalls: []llm.ToolCall{
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
		runtimeevent.TypeApprovalRequest,
		runtimeevent.TypeApprovalDecision,
		runtimeevent.TypeToolResult,
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
