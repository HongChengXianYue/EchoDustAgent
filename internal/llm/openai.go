package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleClient struct {
	BaseURL           string
	APIKey            string
	Model             string
	ParallelToolCalls bool
	Client            *http.Client
}

type OpenAICompatibleOptions struct {
	Timeout           time.Duration
	ParallelToolCalls bool
}

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
	return &OpenAICompatibleClient{
		BaseURL:           strings.TrimRight(baseURL, "/"),
		APIKey:            apiKey,
		Model:             model,
		ParallelToolCalls: options.ParallelToolCalls,
		Client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *OpenAICompatibleClient) ChatWithTools(ctx context.Context, messages []Message, tools []FunctionTool) (*ChatResponse, error) {
	reqBody := chatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
		Tools:    buildToolSpecs(tools),
	}
	if len(tools) > 0 {
		reqBody.ToolChoice = "auto"
		reqBody.ParallelToolCalls = boolPtr(c.ParallelToolCalls)
	}

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
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return parseChatCompletionResponse(respBody)
}

type chatCompletionRequest struct {
	Model             string     `json:"model"`
	Messages          []Message  `json:"messages"`
	Tools             []toolSpec `json:"tools,omitempty"`
	ToolChoice        string     `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool      `json:"parallel_tool_calls,omitempty"`
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

func parseChatCompletionResponse(body []byte) (*ChatResponse, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}
	message := parsed.Choices[0].Message
	return &ChatResponse{
		Content:   message.Content,
		ToolCalls: message.ToolCalls,
		Usage:     parsed.Usage,
	}, nil
}

func boolPtr(v bool) *bool {
	return &v
}
