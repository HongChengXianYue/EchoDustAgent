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
		if req["parallel_tool_calls"] != false {
			t.Fatalf("parallel_tool_calls = %v, want false", req["parallel_tool_calls"])
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
