package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
llm:
  base_url: "https://example.test/v1"
  model: test-model
  request_timeout_seconds: 7
  parallel_tool_calls: false
agent:
  max_steps: 9
subagents:
  enabled: false
  max_concurrent: 4
  max_steps: 5
  result_max_bytes: 6789
tools:
  list_max_entries: 11
  file_change_preview_lines: 3
ui:
  separator_width: 42
  approval_args_preview_chars: 123
`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.LLM.BaseURL != "https://example.test/v1" {
		t.Fatalf("base url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "test-model" {
		t.Fatalf("model = %q", cfg.LLM.Model)
	}
	if cfg.LLM.RequestTimeoutSeconds != 7 {
		t.Fatalf("request timeout = %d", cfg.LLM.RequestTimeoutSeconds)
	}
	if cfg.LLM.ParallelToolCalls {
		t.Fatalf("parallel tool calls = true, want false")
	}
	if cfg.Agent.MaxSteps != 9 {
		t.Fatalf("max steps = %d", cfg.Agent.MaxSteps)
	}
	if cfg.Subagents.Enabled {
		t.Fatalf("subagents enabled = true, want false")
	}
	if cfg.Subagents.MaxConcurrent != 4 {
		t.Fatalf("subagents max concurrent = %d", cfg.Subagents.MaxConcurrent)
	}
	if cfg.Subagents.MaxSteps != 5 {
		t.Fatalf("subagents max steps = %d", cfg.Subagents.MaxSteps)
	}
	if cfg.Subagents.ResultMaxBytes != 6789 {
		t.Fatalf("subagents result max bytes = %d", cfg.Subagents.ResultMaxBytes)
	}
	if cfg.Tools.ListMaxEntries != 11 {
		t.Fatalf("list max entries = %d", cfg.Tools.ListMaxEntries)
	}
	if cfg.Tools.FileChangePreviewLines != 3 {
		t.Fatalf("file change preview lines = %d", cfg.Tools.FileChangePreviewLines)
	}
	if cfg.Tools.ReadFileMaxBytes != Default().Tools.ReadFileMaxBytes {
		t.Fatalf("default read max bytes not preserved")
	}
	if cfg.UI.SeparatorWidth != 42 {
		t.Fatalf("separator width = %d", cfg.UI.SeparatorWidth)
	}
	if cfg.UI.ApprovalArgsPreviewChars != 123 {
		t.Fatalf("approval args preview chars = %d", cfg.UI.ApprovalArgsPreviewChars)
	}
}

func TestLoadFileRejectsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("tools:\n  missing_limit: 1\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatalf("LoadFile() error = nil, want unknown key error")
	}
}

func TestLoadFromEnvOverridesConfigDefaults(t *testing.T) {
	t.Setenv("AGENT_API_KEY", "test-key")
	t.Setenv("AGENT_BASE_URL", "https://env.example/v1")
	t.Setenv("AGENT_MODEL", "env-model")
	t.Setenv("AGENT_MAX_STEPS", "12")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("api key = %q", cfg.APIKey)
	}
	if cfg.LLM.BaseURL != "https://env.example/v1" {
		t.Fatalf("base url = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "env-model" {
		t.Fatalf("model = %q", cfg.LLM.Model)
	}
	if cfg.Agent.MaxSteps != 12 {
		t.Fatalf("max steps = %d", cfg.Agent.MaxSteps)
	}
}
