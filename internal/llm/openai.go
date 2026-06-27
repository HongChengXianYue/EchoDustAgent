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
	"time"

	"local-agent/internal/logs"
)

type OpenAICompatibleClient struct {
	BaseURL           string
	APIKey            string
	Model             string
	WireAPI           string
	ParallelToolCalls bool
	Client            *http.Client
}

type OpenAICompatibleOptions struct {
	Timeout           time.Duration
	WireAPI           string
	ParallelToolCalls bool
}

const (
	WireAPIChatCompletions = "chat_completions"
	WireAPIResponses       = "responses"
)

func NewOpenAICompatibleClient(baseURL, apiKey, model string) *OpenAICompatibleClient {
	return NewOpenAICompatibleClientWithOptions(baseURL, apiKey, model, OpenAICompatibleOptions{
		Timeout:           120 * time.Second,
		ParallelToolCalls: true,
	})
}

func NewOpenAICompatibleClientWithTimeout(baseURL, apiKey, model string, timeout time.Duration) *OpenAICompatibleClient {
	return NewOpenAICompatibleClientWithOptions(baseURL, apiKey, model, OpenAICompatibleOptions{
		Timeout:           timeout,
		ParallelToolCalls: true,
	})
}

func NewOpenAICompatibleClientWithOptions(baseURL, apiKey, model string, options OpenAICompatibleOptions) *OpenAICompatibleClient {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	wireAPI := strings.TrimSpace(options.WireAPI)
	if wireAPI == "" {
		wireAPI = WireAPIChatCompletions
	}
	return &OpenAICompatibleClient{
		BaseURL:           strings.TrimRight(baseURL, "/"),
		APIKey:            apiKey,
		Model:             model,
		WireAPI:           wireAPI,
		ParallelToolCalls: options.ParallelToolCalls,
		Client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OpenAICompatibleClient) ChatWithTools(ctx context.Context, messages []Message, tools []FunctionTool) (*ChatResponse, error) {
	wireAPI := c.effectiveWireAPI()
	switch wireAPI {
	case WireAPIChatCompletions:
		reqBody := c.requestBody(messages, tools, false)
		return c.doChatRequest(ctx, reqBody)
	case WireAPIResponses:
		reqBody := c.responsesRequestBody(messages, tools, false)
		return c.doResponsesRequest(ctx, reqBody)
	default:
		return nil, fmt.Errorf("unsupported OpenAI wire API %q", wireAPI)
	}
}

func (c *OpenAICompatibleClient) ChatWithToolsStream(ctx context.Context, messages []Message, tools []FunctionTool, onDelta StreamHandler) (*ChatResponse, error) {
	wireAPI := c.effectiveWireAPI()
	if wireAPI == WireAPIResponses {
		reqBody := c.responsesRequestBody(messages, tools, true)
		return c.doResponsesStreamRequest(ctx, reqBody, onDelta)
	}
	if wireAPI != WireAPIChatCompletions {
		return nil, fmt.Errorf("unsupported OpenAI wire API %q", wireAPI)
	}
	reqBody := c.requestBody(messages, tools, false)
	reqBody.Stream = true
	return c.doChatStreamRequest(ctx, reqBody, onDelta)
}

func (c *OpenAICompatibleClient) effectiveWireAPI() string {
	wireAPI := strings.TrimSpace(c.WireAPI)
	if wireAPI == "" {
		return WireAPIChatCompletions
	}
	// DeepSeek-compatible routes commonly expose native tools through
	// /chat/completions only; sending these models to /responses returns 404.
	if wireAPI == WireAPIResponses && usesChatCompletionsOnly(c.Model) {
		return WireAPIChatCompletions
	}
	return wireAPI
}

func usesChatCompletionsOnly(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	// DeepSeek and Qwen-compatible routes commonly expose native tools through
	// /chat/completions only; sending these models to /responses returns 404.
	return strings.Contains(model, "deepseek") || strings.Contains(model, "qwen")
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
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
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
		PromptCacheKey:    "local-agent",
		ClientMetadata: map[string]string{
			"client": "local-agent",
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
	return &TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
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

func boolPtr(v bool) *bool {
	return &v
}
