package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local-agent/internal/approval"
	"local-agent/internal/llm"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

type Agent struct {
	client   llm.Client
	registry *tools.Registry
	messages []llm.Message
	maxSteps int
	renderer runtimeevent.Handler
	approver approval.Approver
}

func New(client llm.Client, registry *tools.Registry, maxSteps int) *Agent {
	return NewWithWorkspace(client, registry, maxSteps, "")
}

func NewWithWorkspace(client llm.Client, registry *tools.Registry, maxSteps int, workspace string) *Agent {
	if maxSteps <= 0 {
		maxSteps = 10
	}
	return &Agent{
		client:   client,
		registry: registry,
		maxSteps: maxSteps,
		messages: []llm.Message{
			{
				Role:    "system",
				Content: systemPrompt(workspace),
			},
		},
	}
}

func systemPrompt(workspace string) string {
	lines := []string{
		"You are a local coding agent.",
		"Use the provided function tools when you need workspace information or need to modify files.",
		"Use workspace-relative paths for file tools unless the user explicitly asks for an absolute path.",
		"Run commands in the configured workspace. Do not cd into guessed absolute paths.",
		"Do not inspect the workspace for greetings, small talk, thanks, or general capability questions.",
		"Only call tools when the user asks for a concrete workspace action such as reading, listing, searching, editing files, or running commands.",
		"Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.",
		"When responding in the terminal, keep final answers concise. Markdown is allowed for final summaries when it improves readability, but avoid decorative emoji and excessive detail.",
		"When summarizing recent code changes, prefer git log/stat or the worklog first; do not run a full diff unless the user asks for exact diff details.",
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
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.client.ChatWithTools(ctx, a.messages, a.functionTools())
		if err != nil {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeError, Error: err.Error()})
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
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeFinal, Message: final})
			return final, nil
		}
		if strings.TrimSpace(resp.Content) != "" {
			a.emit(runtimeevent.Event{Step: step, Type: runtimeevent.TypeAssistantMessage, Message: strings.TrimSpace(resp.Content)})
		}
		for _, call := range resp.ToolCalls {
			result := a.executeToolCall(ctx, step, call)
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.JSON(),
			})
		}
	}
	err := fmt.Errorf("agent stopped after %d steps without a final response", a.maxSteps)
	a.emit(runtimeevent.Event{Type: runtimeevent.TypeError, Error: err.Error()})
	return "", err
}

func (a *Agent) Messages() []llm.Message {
	out := make([]llm.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *Agent) executeToolCall(ctx context.Context, step int, call llm.ToolCall) tools.Result {
	tool, ok := a.registry.Get(call.Function.Name)
	if !ok {
		result := tools.Error(fmt.Sprintf("unknown tool %q", call.Function.Name))
		a.emit(runtimeevent.Event{
			Step:   step,
			Type:   runtimeevent.TypeToolResult,
			Tool:   call.Function.Name,
			Args:   call.ArgumentsJSON(),
			Result: &result,
		})
		return result
	}
	args := call.ArgumentsJSON()
	category := approval.Classify(call.Function.Name, args)
	if reason, blocked := approval.BlockReason(call.Function.Name, args); blocked {
		result := tools.Error("command blocked by permanent safety policy: " + reason)
		a.emit(runtimeevent.Event{
			Step:     step,
			Type:     runtimeevent.TypeToolResult,
			Tool:     call.Function.Name,
			Category: category,
			Args:     args,
			Result:   &result,
		})
		return result
	}
	if !a.approveTool(ctx, step, call.Function.Name, category, args) {
		result := tools.Error("tool execution denied by approval policy")
		a.emit(runtimeevent.Event{
			Step:     step,
			Type:     runtimeevent.TypeToolResult,
			Tool:     call.Function.Name,
			Category: category,
			Args:     args,
			Result:   &result,
		})
		return result
	}

	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeToolCall,
		Tool:     call.Function.Name,
		Category: category,
		Args:     args,
	})

	startedAt := time.Now()
	result, err := tool.Execute(ctx, call.ArgumentsJSON())
	if err != nil {
		result = tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	a.emit(runtimeevent.Event{
		Step:       step,
		Type:       runtimeevent.TypeToolResult,
		Tool:       call.Function.Name,
		Category:   category,
		Args:       args,
		Result:     &result,
		DurationMS: time.Since(startedAt).Milliseconds(),
	})
	return result
}

func (a *Agent) approveTool(ctx context.Context, step int, tool string, category approval.Category, args json.RawMessage) bool {
	if !approval.RequiresApproval(category) || a.approver == nil {
		return true
	}
	request := approval.Request{
		Tool:     tool,
		Category: category,
		Args:     args,
		Reason:   "tool execution requested",
	}
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeApprovalRequest,
		Tool:     tool,
		Category: category,
		Args:     args,
		Reason:   request.Reason,
	})
	decision := a.approver.Approve(ctx, request)
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeApprovalDecision,
		Tool:     tool,
		Category: category,
		Args:     args,
		Decision: string(decision),
		Reason:   request.Reason,
	})
	return decision == approval.DecisionAllow || decision == approval.DecisionAlways
}

func (a *Agent) emit(event runtimeevent.Event) {
	if a.renderer != nil {
		a.renderer.HandleEvent(event)
	}
}

func (a *Agent) functionTools() []llm.FunctionTool {
	specs := a.registry.Specs()
	out := make([]llm.FunctionTool, 0, len(specs))
	for _, spec := range specs {
		out = append(out, llm.FunctionTool{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  spec.Parameters,
		})
	}
	return out
}
