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
  wire_api: responses
  request_timeout_seconds: 7
  max_retries: 3
  retry_backoff_milliseconds: 1500
  parallel_tool_calls: false
agent:
  max_steps: 9
  max_parallel_tool_calls: 6
  step_timing_enabled: true
  adaptive_max_steps_enabled: false
  max_step_extensions: 4
  step_extension_size: 3
  absolute_max_steps: 21
subagents:
  enabled: false
  max_concurrent: 4
  max_steps: 5
  adaptive_max_steps_enabled: true
  max_step_extensions: 1
  step_extension_size: 2
  absolute_max_steps: 11
  result_max_bytes: 6789
skills:
  enabled: true
  user_dir: /tmp/echo-dust-code-skills-user
  project_dir: .project-skills
  top_k: 5
  min_score: 7
memory:
  enabled: false
  user_dir: /tmp/echo-dust-code-memory
mcp:
  enabled: true
  dir: /tmp/echo-dust-code-mcp
  start_timeout_seconds: 3
  request_timeout_seconds: 4
session:
  enabled: false
  dir: /tmp/echo-dust-code-session
context:
  window_tokens: 5000
  prune_tool_result_max_bytes: 64
  prune_keep_recent_messages: 3
  compact_enabled: false
  compact_ratio_percent: 75
  compact_force_ratio_percent: 90
  compact_target_percent: 45
  compact_keep_tail_tokens: 1200
  compact_min_messages: 2
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
	if cfg.LLM.WireAPI != "responses" {
		t.Fatalf("wire api = %q", cfg.LLM.WireAPI)
	}
	if cfg.LLM.RequestTimeoutSeconds != 7 {
		t.Fatalf("request timeout = %d", cfg.LLM.RequestTimeoutSeconds)
	}
	if cfg.LLM.MaxRetries != 3 {
		t.Fatalf("max retries = %d", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.RetryBackoffMS != 1500 {
		t.Fatalf("retry backoff = %d", cfg.LLM.RetryBackoffMS)
	}
	if cfg.LLM.ParallelToolCalls {
		t.Fatalf("parallel tool calls = true, want false")
	}
	if cfg.Agent.MaxSteps != 9 {
		t.Fatalf("max steps = %d", cfg.Agent.MaxSteps)
	}
	if cfg.Agent.MaxParallelToolCalls != 6 {
		t.Fatalf("max parallel tool calls = %d", cfg.Agent.MaxParallelToolCalls)
	}
	if !cfg.Agent.StepTimingEnabled {
		t.Fatalf("step timing enabled = false, want true")
	}
	if cfg.Agent.AdaptiveMaxStepsEnabled {
		t.Fatalf("agent adaptive max steps enabled = true, want false")
	}
	if cfg.Agent.MaxStepExtensions != 4 || cfg.Agent.StepExtensionSize != 3 || cfg.Agent.AbsoluteMaxSteps != 21 {
		t.Fatalf("agent step budget = %#v", cfg.Agent)
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
	if !cfg.Subagents.AdaptiveMaxStepsEnabled {
		t.Fatalf("subagents adaptive max steps enabled = false, want true")
	}
	if cfg.Subagents.MaxStepExtensions != 1 || cfg.Subagents.StepExtensionSize != 2 || cfg.Subagents.AbsoluteMaxSteps != 11 {
		t.Fatalf("subagent step budget = %#v", cfg.Subagents)
	}
	if cfg.Subagents.ResultMaxBytes != 6789 {
		t.Fatalf("subagents result max bytes = %d", cfg.Subagents.ResultMaxBytes)
	}
	if !cfg.Skills.Enabled {
		t.Fatalf("skills enabled = false, want true")
	}
	if cfg.Skills.UserDir != "/tmp/echo-dust-code-skills-user" {
		t.Fatalf("skills user dir = %q", cfg.Skills.UserDir)
	}
	if cfg.Skills.ProjectDir != ".project-skills" {
		t.Fatalf("skills project dir = %q", cfg.Skills.ProjectDir)
	}
	if cfg.Skills.TopK != 5 || cfg.Skills.MinScore != 7 {
		t.Fatalf("skills config = %#v", cfg.Skills)
	}
	if cfg.Memory.Enabled {
		t.Fatalf("memory enabled = true, want false")
	}
	if cfg.Memory.UserDir != "/tmp/echo-dust-code-memory" {
		t.Fatalf("memory user dir = %q", cfg.Memory.UserDir)
	}
	if !cfg.MCP.Enabled {
		t.Fatalf("mcp enabled = false, want true")
	}
	if cfg.MCP.Dir != "/tmp/echo-dust-code-mcp" {
		t.Fatalf("mcp dir = %q", cfg.MCP.Dir)
	}
	if cfg.MCP.StartTimeoutSeconds != 3 || cfg.MCP.RequestTimeoutSeconds != 4 {
		t.Fatalf("mcp timeouts = %#v", cfg.MCP)
	}
	if cfg.Session.Enabled {
		t.Fatalf("session enabled = true, want false")
	}
	if cfg.Session.Dir != "/tmp/echo-dust-code-session" {
		t.Fatalf("session dir = %q", cfg.Session.Dir)
	}
	if cfg.Context.WindowTokens != 5000 {
		t.Fatalf("context window tokens = %d", cfg.Context.WindowTokens)
	}
	if cfg.Context.PruneToolResultMaxBytes != 64 {
		t.Fatalf("context prune max bytes = %d", cfg.Context.PruneToolResultMaxBytes)
	}
	if cfg.Context.PruneKeepRecentMessages != 3 {
		t.Fatalf("context prune keep recent = %d", cfg.Context.PruneKeepRecentMessages)
	}
	if cfg.Context.CompactEnabled {
		t.Fatalf("context compact enabled = true, want false")
	}
	if cfg.Context.CompactRatioPercent != 75 || cfg.Context.CompactForceRatioPercent != 90 || cfg.Context.CompactTargetPercent != 45 {
		t.Fatalf("context compact percentages = %#v", cfg.Context)
	}
	if cfg.Context.CompactKeepTailTokens != 1200 {
		t.Fatalf("context compact tail tokens = %d", cfg.Context.CompactKeepTailTokens)
	}
	if cfg.Context.CompactMinMessages != 2 {
		t.Fatalf("context compact min messages = %d", cfg.Context.CompactMinMessages)
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

func TestLoadFileAllowsZeroStepExtensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("agent:\n  max_step_extensions: 0\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Agent.MaxStepExtensions != 0 {
		t.Fatalf("max step extensions = %d, want 0", cfg.Agent.MaxStepExtensions)
	}
}

func TestLoadFileRejectsAbsoluteMaxStepsBelowInitialBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("agent:\n  max_steps: 12\n  absolute_max_steps: 11\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatalf("LoadFile() error = nil, want absolute max validation error")
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

func TestLoadFileRejectsInvalidWireAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("llm:\n  wire_api: completions\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatalf("LoadFile() error = nil, want invalid wire api error")
	}
}

func TestLoadFileRejectsSkillsWithoutRootsWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("skills:\n  enabled: true\n  user_dir: \"\"\n  project_dir: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatalf("LoadFile() error = nil, want skills root validation error")
	}
}

func TestLoadFromEnvOverridesConfigDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	setHomeEnv(t, t.TempDir())
	t.Setenv("AGENT_API_KEY", "test-key")
	t.Setenv("AGENT_BASE_URL", "https://env.example/v1")
	t.Setenv("AGENT_MODEL", "env-model")
	t.Setenv("AGENT_WIRE_API", "responses")
	t.Setenv("AGENT_LLM_MAX_RETRIES", "2")
	t.Setenv("AGENT_LLM_RETRY_BACKOFF_MILLISECONDS", "250")
	t.Setenv("AGENT_MAX_STEPS", "12")
	t.Setenv("AGENT_STEP_TIMING_ENABLED", "true")

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
	if cfg.LLM.WireAPI != "responses" {
		t.Fatalf("wire api = %q", cfg.LLM.WireAPI)
	}
	if cfg.LLM.MaxRetries != 2 {
		t.Fatalf("max retries = %d", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.RetryBackoffMS != 250 {
		t.Fatalf("retry backoff = %d", cfg.LLM.RetryBackoffMS)
	}
	if cfg.Agent.MaxSteps != 12 {
		t.Fatalf("max steps = %d", cfg.Agent.MaxSteps)
	}
	if !cfg.Agent.StepTimingEnabled {
		t.Fatalf("step timing enabled = false, want true")
	}
}

func TestLoadFromEnvUsesExplicitConfigFile(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(configPath, []byte("llm:\n  model: explicit-model\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(workspace)
	t.Setenv("AGENT_API_KEY", "test-key")
	t.Setenv("AGENT_CONFIG_FILE", configPath)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.LLM.Model != "explicit-model" {
		t.Fatalf("model = %q, want explicit-model", cfg.LLM.Model)
	}
}

func TestLoadFromEnvFallsBackToUserConfig(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	configPath := filepath.Join(home, ".echo-dust-code", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("llm:\n  model: user-home-model\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(workspace)
	setHomeEnv(t, home)
	t.Setenv("AGENT_API_KEY", "test-key")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.LLM.Model != "user-home-model" {
		t.Fatalf("model = %q, want user-home-model", cfg.LLM.Model)
	}
}

func TestLoadFromEnvPrefersWorkspaceConfigOverUserConfig(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	workspaceConfig := filepath.Join(workspace, "config.yaml")
	if err := os.WriteFile(workspaceConfig, []byte("llm:\n  model: workspace-model\n"), 0644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	homeConfig := filepath.Join(home, ".echo-dust-code", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(homeConfig), 0755); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	if err := os.WriteFile(homeConfig, []byte("llm:\n  model: user-home-model\n"), 0644); err != nil {
		t.Fatalf("write home config: %v", err)
	}

	t.Chdir(workspace)
	setHomeEnv(t, home)
	t.Setenv("AGENT_API_KEY", "test-key")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if cfg.LLM.Model != "workspace-model" {
		t.Fatalf("model = %q, want workspace-model", cfg.LLM.Model)
	}
}

func TestLoadFromEnvRejectsMissingExplicitConfigFile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("AGENT_API_KEY", "test-key")
	t.Setenv("AGENT_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.yaml"))

	if _, err := LoadFromEnv(); err == nil {
		t.Fatalf("LoadFromEnv() error = nil, want missing explicit config error")
	}
}

func setHomeEnv(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}
