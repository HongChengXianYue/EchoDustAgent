package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-agent/internal/tools"
)

func TestLoadStartsServerAndRegistersTool(t *testing.T) {
	dir := t.TempDir()
	config := fmt.Sprintf(`{
  "servers": {
    "fake": {
      "command": %q,
      "args": ["-test.run=TestMCPHelperProcess"],
      "env": {"LOCAL_AGENT_MCP_HELPER": "1"}
    }
  }
}`, os.Args[0])
	if err := os.WriteFile(filepath.Join(dir, serverConfigFile), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	manager, err := Load(context.Background(), Options{
		Dir:            dir,
		StartTimeout:   5 * time.Second,
		RequestTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	defer manager.Close()

	registry := tools.NewRegistry()
	manager.Register(registry)
	tool, ok := registry.Get("mcp__fake__echo_text")
	if !ok {
		t.Fatalf("registered tools = %#v, want fake echo tool", manager.Tools())
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "success" || !strings.Contains(result.Output, "echo: hello") {
		t.Fatalf("result = %#v", result)
	}
}

func TestLoadConfigSupportsArrayAndDisabledServers(t *testing.T) {
	dir := t.TempDir()
	disabled := false
	config := fmt.Sprintf(`{"servers":[{"name":"one","command":"cmd"},{"name":"two","command":"cmd","enabled":%v}]}`, disabled)
	if err := os.WriteFile(filepath.Join(dir, serverConfigFile), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Servers) != 2 || !serverEnabled(cfg.Servers[0]) || serverEnabled(cfg.Servers[1]) {
		t.Fatalf("servers = %#v", cfg.Servers)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("LOCAL_AGENT_MCP_HELPER") != "1" {
		return
	}
	defer os.Exit(0)
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			continue
		}
		id, hasID := request["id"]
		method, _ := request["method"].(string)
		if !hasID {
			continue
		}
		switch method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"protocolVersion": protocolVersion,
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "fake", "version": "test"},
				},
			})
		case "tools/list":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo.text",
							"description": "Echo text",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"text": map[string]any{"type": "string"}},
							},
						},
					},
				},
			})
		case "tools/call":
			params, _ := request["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "echo: " + text}},
				},
			})
		}
	}
}
