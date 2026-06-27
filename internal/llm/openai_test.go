package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAICompatibleClientSendsNativeToolSpecAndParsesToolCall(t *testing.T) {
	client := NewOpenAICompatibleClient("https://example.test/v1", "test-key", "test-model")
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "test-model" {
			t.Fatalf("model = %v", req["model"])
		}
		if req["tool_choice"] != "auto" {
			t.Fatalf("tool_choice = %v, want auto", req["tool_choice"])
		}
		if req["parallel_tool_calls"] != true {
			t.Fatalf("parallel_tool_calls = %v, want true", req["parallel_tool_calls"])
		}
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one tool", req["tools"])
		}
		spec := tools[0].(map[string]any)
		if spec["type"] != "function" {
			t.Fatalf("tool type = %v, want function", spec["type"])
		}
		fn := spec["function"].(map[string]any)
		if fn["name"] != "read_file" {
			t.Fatalf("function name = %v, want read_file", fn["name"])
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"choices": [{
				"message": {
					"content": "",
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "read_file",
							"arguments": "{\"path\":\"go.mod\"}"
						}
					}]
				}
			}],
			"usage": {"total_tokens": 7}
		}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "read"}}, []FunctionTool{
		{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool name = %q, want read_file", resp.ToolCalls[0].Function.Name)
	}
	if string(resp.ToolCalls[0].ArgumentsJSON()) != `{"path":"go.mod"}` {
		t.Fatalf("arguments = %s", resp.ToolCalls[0].ArgumentsJSON())
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want total 7", resp.Usage)
	}
}

func TestOpenAICompatibleClientCanDisableParallelToolCalls(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "test-model", OpenAICompatibleOptions{
		ParallelToolCalls: false,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["parallel_tool_calls"] != false {
			t.Fatalf("parallel_tool_calls = %v, want false", req["parallel_tool_calls"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
		}, nil
	})}

	if _, err := client.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "read"}}, []FunctionTool{
		{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}); err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
}

func TestOpenAICompatibleClientOmitsToolControlsWhenNoTools(t *testing.T) {
	client := NewOpenAICompatibleClient("https://example.test/v1", "test-key", "test-model")
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := req["tools"]; ok {
			t.Fatalf("tools present in no-tool request: %#v", req["tools"])
		}
		if _, ok := req["tool_choice"]; ok {
			t.Fatalf("tool_choice present in no-tool request: %#v", req["tool_choice"])
		}
		if _, ok := req["parallel_tool_calls"]; ok {
			t.Fatalf("parallel_tool_calls present in no-tool request: %#v", req["parallel_tool_calls"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"hello"}}]}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("content = %q, want hello", resp.Content)
	}
}

func TestOpenAICompatibleClientResponsesSendsFlatToolSpecAndParsesOutput(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "test-model", OpenAICompatibleOptions{
		WireAPI:           WireAPIResponses,
		ParallelToolCalls: true,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "test-model" {
			t.Fatalf("model = %v", req["model"])
		}
		if req["parallel_tool_calls"] != true {
			t.Fatalf("parallel_tool_calls = %v, want true", req["parallel_tool_calls"])
		}
		if req["instructions"] != "sys" {
			t.Fatalf("instructions = %v, want sys", req["instructions"])
		}
		if req["tool_choice"] != "auto" {
			t.Fatalf("tool_choice = %v, want auto", req["tool_choice"])
		}
		if req["store"] != false {
			t.Fatalf("store = %v, want false", req["store"])
		}
		if req["stream"] != false {
			t.Fatalf("stream = %v, want false", req["stream"])
		}
		if _, ok := req["prompt_cache_key"].(string); !ok {
			t.Fatalf("prompt_cache_key = %#v, want string", req["prompt_cache_key"])
		}
		if include, ok := req["include"].([]any); !ok || len(include) != 0 {
			t.Fatalf("include = %#v, want empty array", req["include"])
		}
		input, ok := req["input"].([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("input = %#v, want one user message", req["input"])
		}
		user := input[0].(map[string]any)
		if user["type"] != "message" || user["role"] != "user" {
			t.Fatalf("user input = %#v", user)
		}
		content, ok := user["content"].([]any)
		if !ok || len(content) != 1 {
			t.Fatalf("user content = %#v, want one typed part", user["content"])
		}
		part := content[0].(map[string]any)
		if part["type"] != "input_text" || part["text"] != "read" {
			t.Fatalf("user content part = %#v", part)
		}
		tools, ok := req["tools"].([]any)
		if !ok || len(tools) != 1 {
			t.Fatalf("tools = %#v, want one tool", req["tools"])
		}
		spec := tools[0].(map[string]any)
		if spec["type"] != "function" {
			t.Fatalf("tool type = %v, want function", spec["type"])
		}
		if spec["name"] != "read_file" {
			t.Fatalf("function name = %v, want read_file", spec["name"])
		}
		if spec["strict"] != false {
			t.Fatalf("strict = %v, want false", spec["strict"])
		}
		if _, ok := spec["function"]; ok {
			t.Fatalf("responses tool spec must be flat, got nested function: %#v", spec["function"])
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"output": [
				{
					"type": "message",
					"role": "assistant",
					"content": [{"type": "output_text", "text": "checking"}]
				},
				{
					"type": "function_call",
					"call_id": "call_1",
					"name": "read_file",
					"arguments": "{\"path\":\"go.mod\"}"
				}
			],
			"usage": {"input_tokens": 2, "output_tokens": 5, "total_tokens": 7}
		}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "read"},
	}, []FunctionTool{
		{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	})
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if resp.Content != "checking" {
		t.Fatalf("content = %q, want checking", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool call id = %q, want call_1", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool name = %q, want read_file", resp.ToolCalls[0].Function.Name)
	}
	if string(resp.ToolCalls[0].ArgumentsJSON()) != `{"path":"go.mod"}` {
		t.Fatalf("arguments = %s", resp.ToolCalls[0].ArgumentsJSON())
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 2 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want 2/5/7", resp.Usage)
	}
}

func TestOpenAICompatibleClientUsesChatCompletionsForDeepSeekModels(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "deepseek-v4-flash", OpenAICompatibleOptions{
		WireAPI:           WireAPIResponses,
		ParallelToolCalls: true,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "deepseek-v4-flash" {
			t.Fatalf("model = %v", req["model"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
}

func TestOpenAICompatibleClientStreamsDeepSeekModelsThroughChatCompletions(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "deepseek-v4-flash", OpenAICompatibleOptions{
		WireAPI:           WireAPIResponses,
		ParallelToolCalls: true,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["stream"] != true {
			t.Fatalf("stream = %v, want true", req["stream"])
		}
		body := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"ok"}}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	resp, err := client.ChatWithToolsStream(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithToolsStream() error = %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
}

func TestOpenAICompatibleClientUsesChatCompletionsForQwenModels(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "qwen3.7-plus", OpenAICompatibleOptions{
		WireAPI:           WireAPIResponses,
		ParallelToolCalls: true,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "qwen3.7-plus" {
			t.Fatalf("model = %v", req["model"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
}

func TestOpenAICompatibleClientResponsesSerializesPriorToolExchange(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "test-model", OpenAICompatibleOptions{
		WireAPI: WireAPIResponses,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		input, ok := req["input"].([]any)
		if !ok || len(input) != 3 {
			t.Fatalf("input = %#v, want user, function_call, function_call_output", req["input"])
		}
		call := input[1].(map[string]any)
		if call["type"] != "function_call" || call["call_id"] != "call_1" || call["name"] != "read_file" {
			t.Fatalf("function_call input = %#v", call)
		}
		output := input[2].(map[string]any)
		if output["type"] != "function_call_output" || output["call_id"] != "call_1" {
			t.Fatalf("function_call_output input = %#v", output)
		}
		if output["output"] != `{"output":"module local-agent"}` {
			t.Fatalf("function output = %v", output["output"])
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}`)),
		}, nil
	})}

	resp, err := client.ChatWithTools(context.Background(), []Message{
		{Role: "user", Content: "read"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolFunction{
						Name:      "read_file",
						Arguments: `{"path":"go.mod"}`,
					},
				},
			},
		},
		{Role: "tool", ToolCallID: "call_1", Content: `{"output":"module local-agent"}`},
	}, nil)
	if err != nil {
		t.Fatalf("ChatWithTools() error = %v", err)
	}
	if resp.Content != "done" {
		t.Fatalf("content = %q, want done", resp.Content)
	}
}

func TestOpenAICompatibleClientResponsesAddsReasoningForCodexModel(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "gpt-5.5", OpenAICompatibleOptions{
		WireAPI: WireAPIResponses,
	})

	req := client.responsesRequestBody([]Message{{Role: "user", Content: "hello"}}, nil, true)
	if req.Reasoning == nil {
		t.Fatalf("reasoning = nil, want Codex-style reasoning controls")
	}
	if req.Reasoning.Effort != "xhigh" || req.Reasoning.Summary != "auto" {
		t.Fatalf("reasoning = %#v, want xhigh/auto", req.Reasoning)
	}
	if len(req.Include) != 1 || req.Include[0] != "reasoning.encrypted_content" {
		t.Fatalf("include = %#v, want reasoning encrypted content", req.Include)
	}
}

func TestOpenAICompatibleClientResponsesStreamsContentAndToolCalls(t *testing.T) {
	client := NewOpenAICompatibleClientWithOptions("https://example.test/v1", "test-key", "test-model", OpenAICompatibleOptions{
		WireAPI:           WireAPIResponses,
		ParallelToolCalls: true,
	})
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want text/event-stream", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["stream"] != true {
			t.Fatalf("stream = %v, want true", req["stream"])
		}
		if req["tool_choice"] != "auto" {
			t.Fatalf("tool_choice = %v, want auto", req["tool_choice"])
		}
		body := strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"hello"}`,
			``,
			`data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}"}}`,
			``,
			`data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":2,"output_tokens":5,"total_tokens":7}}}`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	var deltas []StreamDelta
	resp, err := client.ChatWithToolsStream(context.Background(), []Message{{Role: "user", Content: "hello"}}, []FunctionTool{
		{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		},
	}, func(delta StreamDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatWithToolsStream() error = %v", err)
	}
	if resp.Content != "hello" {
		t.Fatalf("content = %q, want hello", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].ID != "call_1" || resp.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool call = %#v", resp.ToolCalls[0])
	}
	if string(resp.ToolCalls[0].ArgumentsJSON()) != `{"path":"go.mod"}` {
		t.Fatalf("arguments = %s", resp.ToolCalls[0].ArgumentsJSON())
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 2 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want 2/5/7", resp.Usage)
	}
	if len(deltas) != 2 {
		t.Fatalf("deltas = %#v, want content and done", deltas)
	}
	if deltas[0].Content != "hello" || !deltas[1].Done {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestOpenAICompatibleClientStreamsContent(t *testing.T) {
	client := NewOpenAICompatibleClient("https://example.test/v1", "test-key", "test-model")
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["stream"] != true {
			t.Fatalf("stream = %v, want true", req["stream"])
		}
		body := strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"hello "}}]}`,
			``,
			`data: {"choices":[{"delta":{"content":"world"}}],"usage":{"total_tokens":9}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	var deltas []string
	resp, err := client.ChatWithToolsStream(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil, func(delta StreamDelta) error {
		if delta.Content != "" {
			deltas = append(deltas, delta.Content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ChatWithToolsStream() error = %v", err)
	}
	if strings.Join(deltas, "") != "hello world" {
		t.Fatalf("deltas = %#v, want hello world", deltas)
	}
	if resp.Content != "hello world" {
		t.Fatalf("content = %q, want hello world", resp.Content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 9 {
		t.Fatalf("usage = %#v, want total 9", resp.Usage)
	}
}

func TestOpenAICompatibleClientStreamsToolCallsKeepFunctionType(t *testing.T) {
	client := NewOpenAICompatibleClient("https://example.test/v1", "test-key", "test-model")
	client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			`data: {"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\""}}]}}]}`,
			``,
			`data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"README.md\"}"}}]}}]}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}

	resp, err := client.ChatWithToolsStream(context.Background(), []Message{{Role: "user", Content: "read"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithToolsStream() error = %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Type != "function" {
		t.Fatalf("tool call type = %q, want function", resp.ToolCalls[0].Type)
	}
	if resp.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool name = %q, want read_file", resp.ToolCalls[0].Function.Name)
	}
	if resp.ToolCalls[0].Function.Arguments != `{"path":"README.md"}` {
		t.Fatalf("tool arguments = %q", resp.ToolCalls[0].Function.Arguments)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
