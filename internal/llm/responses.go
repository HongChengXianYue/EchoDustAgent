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

// Responses API protocol types and implementation.
//
// This file handles the /responses endpoint (OpenAI Responses API / Codex),
// including request/response types, HTTP transport, and SSE streaming.

type responsesRequest struct {
	Model             string              `json:"model"`
	Instructions      string              `json:"instructions"`
	Input             []responsesItem     `json:"input"`
	Tools             []responsesToolSpec `json:"tools"`
	ToolChoice        string              `json:"tool_choice"`
	ParallelToolCalls bool                `json:"parallel_tool_calls"`
	Reasoning         *responsesReasoning `json:"reasoning,omitempty"`
	Store             bool                `json:"store"`
	Stream            bool                `json:"stream"`
	Include           []string            `json:"include"`
	PromptCacheKey    string              `json:"prompt_cache_key,omitempty"`
	ClientMetadata    map[string]string   `json:"client_metadata,omitempty"`
}

type responsesItem struct {
	Type      string                 `json:"type,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Output    string                 `json:"output,omitempty"`
}

type responsesToolSpec struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Strict      bool            `json:"strict"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type responsesResponse struct {
	Output     []responsesOutputItem `json:"output"`
	OutputText string                `json:"output_text,omitempty"`
	Usage      *responsesUsage       `json:"usage,omitempty"`
}

type responsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Text      string                 `json:"text,omitempty"`
}

type responsesContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type responsesUsage struct {
	InputTokens        int                         `json:"input_tokens,omitempty"`
	OutputTokens       int                         `json:"output_tokens,omitempty"`
	PromptTokens       int                         `json:"prompt_tokens,omitempty"`
	CompletionTokens   int                         `json:"completion_tokens,omitempty"`
	TotalTokens        int                         `json:"total_tokens,omitempty"`
	CachedTokens       int                         `json:"cached_tokens,omitempty"`
	InputTokensDetails *responsesInputTokensDetail `json:"input_tokens_details,omitempty"`
	PromptTokensDetail *responsesInputTokensDetail `json:"prompt_tokens_details,omitempty"`
}

type responsesInputTokensDetail struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type responsesReasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary,omitempty"`
}

type responsesStreamEvent struct {
	Type     string              `json:"type"`
	Delta    string              `json:"delta,omitempty"`
	Item     responsesOutputItem `json:"item,omitempty"`
	Response responsesResponse   `json:"response,omitempty"`
	Error    *responsesError     `json:"error,omitempty"`
}

type responsesError struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
}

func buildResponsesToolSpecs(tools []FunctionTool) []responsesToolSpec {
	if len(tools) == 0 {
		return []responsesToolSpec{}
	}
	specs := make([]responsesToolSpec, 0, len(tools))
	for _, tool := range tools {
		specs = append(specs, responsesToolSpec{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Strict:      false,
			Parameters:  tool.Parameters,
		})
	}
	return specs
}

func (c *OpenAICompatibleClient) responsesRequestBody(messages []Message, tools []FunctionTool, stream bool) responsesRequest {
	instructions, input := responsesInput(messages)
	reasoning := responsesReasoningForModel(c.Model)
	include := []string{}
	if reasoning != nil {
		include = append(include, "reasoning.encrypted_content")
	}
	reqBody := responsesRequest{
		Model:             c.Model,
		Instructions:      instructions,
		Input:             input,
		Tools:             buildResponsesToolSpecs(tools),
		ToolChoice:        "auto",
		ParallelToolCalls: c.ParallelToolCalls,
		Reasoning:         reasoning,
		Store:             false,
		Stream:            stream,
		Include:           include,
		PromptCacheKey:    "echo-dust-code",
		ClientMetadata: map[string]string{
			"client": "echo-dust-code",
		},
	}
	return reqBody
}

func responsesReasoningForModel(model string) *responsesReasoning {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(normalized, "gpt-5") || strings.Contains(normalized, "codex") {
		return &responsesReasoning{
			Effort:  "xhigh",
			Summary: "auto",
		}
	}
	return nil
}

func responsesInput(messages []Message) (string, []responsesItem) {
	instructions := []string{}
	items := make([]responsesItem, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			if strings.TrimSpace(message.Content) != "" {
				instructions = append(instructions, message.Content)
			}
		case "tool":
			items = append(items, responsesItem{
				Type:   "function_call_output",
				CallID: message.ToolCallID,
				Output: message.Content,
			})
		case "assistant":
			if strings.TrimSpace(message.Content) != "" || len(message.ToolCalls) == 0 {
				items = append(items, responsesItem{
					Type: "message",
					Role: "assistant",
					Content: []responsesContentPart{{
						Type: "output_text",
						Text: message.Content,
					}},
				})
			}
			for _, call := range message.ToolCalls {
				callID := call.ID
				if callID == "" {
					callID = call.Function.Name
				}
				items = append(items, responsesItem{
					Type:      "function_call",
					CallID:    callID,
					Name:      call.Function.Name,
					Arguments: call.Function.Arguments,
				})
			}
		default:
			role := message.Role
			if role == "" {
				role = "user"
			}
			items = append(items, responsesItem{
				Type: "message",
				Role: role,
				Content: []responsesContentPart{{
					Type: "input_text",
					Text: message.Content,
				}},
			})
		}
	}
	return strings.Join(instructions, "\n\n"), items
}

func (c *OpenAICompatibleClient) doResponsesRequest(ctx context.Context, reqBody responsesRequest) (*ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(body))
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
		logs.Errorf("llm request transport failed: wire_api=responses stream=false model=%s err=%v", c.Model, err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Errorf("llm response read failed: wire_api=responses stream=false model=%s err=%v", c.Model, err)
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logs.Errorf("llm request failed: wire_api=responses stream=false model=%s status=%d body=%s", c.Model, resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return parseResponsesResponse(respBody)
}

func (c *OpenAICompatibleClient) doResponsesStreamRequest(ctx context.Context, reqBody responsesRequest, onDelta StreamHandler) (*ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.Client
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		logs.Errorf("llm request transport failed: wire_api=responses stream=true model=%s err=%v", c.Model, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			logs.Errorf("llm request failed: wire_api=responses stream=true model=%s status=%d body_read_err=%v", c.Model, resp.StatusCode, readErr)
			return nil, fmt.Errorf("llm request failed: status=%d", resp.StatusCode)
		}
		logs.Errorf("llm request failed: wire_api=responses stream=true model=%s status=%d body=%s", c.Model, resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return parseResponsesStream(resp.Body, onDelta)
}

func parseResponsesResponse(body []byte) (*ChatResponse, error) {
	var parsed responsesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		logs.Errorf("llm response decode failed: wire_api=responses err=%v body=%s", err, string(body))
		return nil, err
	}
	var content strings.Builder
	toolCalls := []ToolCall{}
	for _, item := range parsed.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if part.Text != "" {
					content.WriteString(part.Text)
				}
			}
		case "function_call":
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			toolCalls = append(toolCalls, ToolCall{
				ID:   callID,
				Type: "function",
				Function: ToolFunction{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		default:
			if item.Text != "" {
				content.WriteString(item.Text)
			}
		}
	}
	if content.Len() == 0 && parsed.OutputText != "" {
		content.WriteString(parsed.OutputText)
	}
	if len(parsed.Output) == 0 && parsed.OutputText == "" {
		logs.Errorf("llm response has no output: wire_api=responses body=%s", string(body))
		return nil, fmt.Errorf("llm response has no output")
	}
	return &ChatResponse{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Usage:     parsed.Usage.toTokenUsage(),
	}, nil
}

func (u *responsesUsage) toTokenUsage() *TokenUsage {
	if u == nil {
		return nil
	}
	promptTokens := u.PromptTokens
	if promptTokens == 0 {
		promptTokens = u.InputTokens
	}
	completionTokens := u.CompletionTokens
	if completionTokens == 0 {
		completionTokens = u.OutputTokens
	}
	totalTokens := u.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	cachedTokens := u.CachedTokens
	if cachedTokens == 0 && u.InputTokensDetails != nil {
		cachedTokens = u.InputTokensDetails.CachedTokens
	}
	if cachedTokens == 0 && u.PromptTokensDetail != nil {
		cachedTokens = u.PromptTokensDetail.CachedTokens
	}
	return &TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		CachedTokens:     cachedTokens,
	}
}

func parseResponsesStream(body io.Reader, onDelta StreamHandler) (*ChatResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var content strings.Builder
	toolCalls := []ToolCall{}
	var usage *TokenUsage
	doneSent := false
	sawTextDelta := false

	flushDelta := func(delta StreamDelta) error {
		if onDelta == nil {
			return nil
		}
		return onDelta(delta)
	}
	flushDone := func() error {
		if doneSent {
			return nil
		}
		doneSent = true
		return flushDelta(StreamDelta{Done: true, Usage: usage})
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" {
			if err := flushDone(); err != nil {
				return nil, err
			}
			break
		}

		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logs.Errorf("llm responses stream event decode failed: err=%v payload=%s", err, payload)
			return nil, err
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				sawTextDelta = true
				content.WriteString(event.Delta)
				if err := flushDelta(StreamDelta{Content: event.Delta, Usage: usage}); err != nil {
					logs.Errorf("llm responses stream delta handler failed: err=%v delta=%q", err, event.Delta)
					return nil, err
				}
			}
		case "response.output_item.done":
			switch event.Item.Type {
			case "function_call":
				callID := event.Item.CallID
				if callID == "" {
					callID = event.Item.ID
				}
				toolCalls = append(toolCalls, ToolCall{
					ID:   callID,
					Type: "function",
					Function: ToolFunction{
						Name:      event.Item.Name,
						Arguments: event.Item.Arguments,
					},
				})
			case "message":
				// Some Responses-compatible providers do not stream text deltas and
				// only send the completed message item.
				if sawTextDelta {
					break
				}
				for _, part := range event.Item.Content {
					if part.Text != "" {
						content.WriteString(part.Text)
						if err := flushDelta(StreamDelta{Content: part.Text, Usage: usage}); err != nil {
							return nil, err
						}
					}
				}
			}
		case "response.completed":
			usage = event.Response.Usage.toTokenUsage()
			if err := flushDone(); err != nil {
				return nil, err
			}
		case "response.failed", "response.incomplete", "error":
			if event.Error != nil && event.Error.Message != "" {
				return nil, fmt.Errorf("llm responses stream failed: %s", event.Error.Message)
			}
			return nil, fmt.Errorf("llm responses stream failed: %s", event.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		logs.Errorf("llm responses stream scanner failed: err=%v", err)
		return nil, err
	}
	if !doneSent {
		if err := flushDone(); err != nil {
			return nil, err
		}
	}
	return &ChatResponse{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}
