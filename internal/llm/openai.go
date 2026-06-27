package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
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
