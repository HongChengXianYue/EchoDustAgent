package agent

import (
	"context"
	"fmt"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

type Agent struct {
	client   llm.Client
	registry *tools.Registry
	messages []llm.Message
	maxSteps int
}

func New(client llm.Client, registry *tools.Registry, maxSteps int) *Agent {
	if maxSteps <= 0 {
		maxSteps = 10
	}
	return &Agent{
		client:   client,
		registry: registry,
		maxSteps: maxSteps,
		messages: []llm.Message{
			{
				Role: "system",
				Content: strings.TrimSpace(`You are a local coding agent.
Use the provided function tools when you need workspace information or need to modify files.
Do not write JSON tool calls in assistant text. Tool calls must use native function calling only.
When you are done, answer directly and concisely.`),
			},
		},
	}
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	a.messages = append(a.messages, llm.Message{Role: "user", Content: input})

	for step := 0; step < a.maxSteps; step++ {
		resp, err := a.client.ChatWithTools(ctx, a.messages, a.functionTools())
		if err != nil {
			return "", err
		}
		assistantMessage := llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.messages = append(a.messages, assistantMessage)

		if len(resp.ToolCalls) == 0 {
			return strings.TrimSpace(resp.Content), nil
		}
		for _, call := range resp.ToolCalls {
			result := a.executeToolCall(ctx, call)
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.JSON(),
			})
		}
	}
	return "", fmt.Errorf("agent stopped after %d steps without a final response", a.maxSteps)
}

func (a *Agent) Messages() []llm.Message {
	out := make([]llm.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *Agent) executeToolCall(ctx context.Context, call llm.ToolCall) tools.Result {
	tool, ok := a.registry.Get(call.Function.Name)
	if !ok {
		return tools.Error(fmt.Sprintf("unknown tool %q", call.Function.Name))
	}
	result, err := tool.Execute(ctx, call.ArgumentsJSON())
	if err != nil {
		return tools.Error(err.Error())
	}
	if result.Status == "" {
		result.Status = "success"
	}
	return result
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
