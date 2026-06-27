package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"local-agent/internal/logs"
)

// Chat Completions protocol types and implementation.
//
// This file handles the /chat/completions endpoint, including request/response
// types, HTTP transport, and SSE streaming.

type chatCompletionRequest struct {
	Model             string     `json:"model"`
	Messages          []Message  `json:"messages"`
	Tools             []toolSpec `json:"tools,omitempty"`
	ToolChoice        string     `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool      `json:"parallel_tool_calls,omitempty"`
	Stream            bool       `json:"stream,omitempty"`
}

type toolSpec struct {
	Type     string       `json:"type"`
	Function FunctionTool `json:"function"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *TokenUsage `json:"usage,omitempty"`
}

type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *TokenUsage `json:"usage,omitempty"`
}

func buildToolSpecs(tools []FunctionTool) []toolSpec {
	if len(tools) == 0 {
		return nil
	}
	specs := make([]toolSpec, 0, len(tools))
	for _, tool := range tools {
		specs = append(specs, toolSpec{
			Type:     "function",
			Function: tool,
		})
	}
	return specs
}

func (c *OpenAICompatibleClient) requestBody(messages []Message, tools []FunctionTool, stream bool) chatCompletionRequest {
	reqBody := chatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    buildToolSpecs(tools),
		Stream:   stream,
	}
	if len(tools) > 0 {
		reqBody.ToolChoice = "auto"
		reqBody.ParallelToolCalls = boolPtr(c.ParallelToolCalls)
	}
	return reqBody
}

func (c *OpenAICompatibleClient) doChatRequest(ctx context.Context, reqBody chatCompletionRequest) (*ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		logs.Errorf("llm request transport failed: stream=false model=%s err=%v", c.Model, err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Errorf("llm response read failed: stream=false model=%s err=%v", c.Model, err)
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logs.Errorf("llm request failed: stream=false model=%s status=%d body=%s", c.Model, resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return parseChatCompletionResponse(respBody)
}

func (c *OpenAICompatibleClient) doChatStreamRequest(ctx context.Context, reqBody chatCompletionRequest, onDelta StreamHandler) (*ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		logs.Errorf("llm request transport failed: stream=true model=%s err=%v", c.Model, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logs.Errorf("llm request failed: stream=true model=%s status=%d body_read_err=%v", c.Model, resp.StatusCode, readErr)
			return nil, fmt.Errorf("llm request failed: status=%d", resp.StatusCode)
		}
		logs.Errorf("llm request failed: stream=true model=%s status=%d body=%s", c.Model, resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return parseChatCompletionStream(resp.Body, onDelta)
}

func parseChatCompletionResponse(body []byte) (*ChatResponse, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		logs.Errorf("llm response decode failed: stream=false err=%v body=%s", err, string(body))
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		logs.Errorf("llm response has no choices: stream=false body=%s", string(body))
		return nil, fmt.Errorf("llm response has no choices")
	}
	message := parsed.Choices[0].Message
	return &ChatResponse{
		Content:   message.Content,
		ToolCalls: message.ToolCalls,
		Usage:     parsed.Usage,
	}, nil
}

func parseChatCompletionStream(body io.Reader, onDelta StreamHandler) (*ChatResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var content strings.Builder
	toolCallsByID := map[string]*ToolCall{}
	toolCallOrder := []string{}
	lastToolCallID := ""
	var usage *TokenUsage

	flushDelta := func(delta StreamDelta) error {
		if onDelta == nil {
			return nil
		}
		return onDelta(delta)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" {
			if err := flushDelta(StreamDelta{Done: true, Usage: usage}); err != nil {
				return nil, err
			}
			break
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			logs.Errorf("llm stream chunk decode failed: err=%v payload=%s", err, payload)
			return nil, err
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		delta := StreamDelta{Usage: chunk.Usage}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				delta.Content += choice.Delta.Content
			}
			for _, partial := range choice.Delta.ToolCalls {
				id := partial.ID
				if id == "" {
					if lastToolCallID != "" {
						id = lastToolCallID
					} else {
						id = fmt.Sprintf("tool_%d", len(toolCallOrder))
					}
				}
				toolType := partial.Type
				if toolType == "" {
					toolType = "function"
				}
				existing, ok := toolCallsByID[id]
				if !ok {
					call := &ToolCall{ID: id, Type: toolType}
					call.Function.Name = partial.Function.Name
					call.Function.Arguments = partial.Function.Arguments
					toolCallsByID[id] = call
					toolCallOrder = append(toolCallOrder, id)
					existing = call
				} else {
					if existing.Type == "" {
						existing.Type = "function"
					}
					if partial.Type != "" {
						existing.Type = partial.Type
					}
					if partial.Function.Name != "" {
						existing.Function.Name = partial.Function.Name
					}
					if partial.Function.Arguments != "" {
						existing.Function.Arguments += partial.Function.Arguments
					}
				}
				lastToolCallID = id
			}
		}
		if delta.Content != "" || delta.Usage != nil {
			if err := flushDelta(delta); err != nil {
				logs.Errorf("llm stream delta handler failed: err=%v delta=%q", err, delta.Content)
				return nil, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		logs.Errorf("llm stream scanner failed: err=%v", err)
		return nil, err
	}

	toolCalls := make([]ToolCall, 0, len(toolCallOrder))
	for _, id := range toolCallOrder {
		toolCalls = append(toolCalls, *toolCallsByID[id])
	}
	return &ChatResponse{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}

func boolPtr(v bool) *bool {
	return &v
}
