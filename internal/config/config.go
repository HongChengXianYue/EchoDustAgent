package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultConfigPath = "config.yaml"
)

type Config struct {
	APIKey    string
	LLM       LLMConfig
	Agent     AgentConfig
	Subagents SubagentsConfig
	Skills    SkillsConfig
	Memory    MemoryConfig
	MCP       MCPConfig
	Session   SessionConfig
	Context   ContextConfig
	Tools     ToolsConfig
	UI        UIConfig
}

type LLMConfig struct {
	BaseURL               string
	Model                 string
	WireAPI               string
	RequestTimeoutSeconds int
	MaxRetries            int
	RetryBackoffMS        int
	ParallelToolCalls     bool
}

type AgentConfig struct {
	MaxSteps                int
	MaxParallelToolCalls    int
	StepTimingEnabled       bool
	AdaptiveMaxStepsEnabled bool
	MaxStepExtensions       int
	StepExtensionSize       int
	AbsoluteMaxSteps        int
}

type SubagentsConfig struct {
	Enabled                 bool
	MaxConcurrent           int
	MaxSteps                int
	AdaptiveMaxStepsEnabled bool
	MaxStepExtensions       int
	StepExtensionSize       int
	AbsoluteMaxSteps        int
	ResultMaxBytes          int
}

type SkillsConfig struct {
	Enabled    bool
	UserDir    string
	ProjectDir string
	TopK       int
	MinScore   int
}

type MemoryConfig struct {
	Enabled bool
	UserDir string
}

type MCPConfig struct {
	Enabled               bool
	Dir                   string
	StartTimeoutSeconds   int
	RequestTimeoutSeconds int
}

type SessionConfig struct {
	Enabled bool
	Dir     string
}

type ContextConfig struct {
	WindowTokens             int
	PruneToolResultMaxBytes  int
	PruneKeepRecentMessages  int
	CompactEnabled           bool
	CompactRatioPercent      int
	CompactForceRatioPercent int
	CompactTargetPercent     int
	CompactKeepTailTokens    int
	CompactMinMessages       int
}

type ToolsConfig struct {
	ListMaxEntries               int
	FindMaxMatches               int
	ReadFileMaxBytes             int
	SearchMaxMatches             int
	SearchMaxFileBytes           int
	CommandDefaultTimeoutSeconds int
	CommandMaxTimeoutSeconds     int
	CommandOutputMaxBytes        int
	ApplyPatchTimeoutSeconds     int
	ApplyPatchOutputMaxBytes     int
	FileChangePreviewLines       int
}

type UIConfig struct {
	SeparatorWidth             int
	LiveFrameMaxLines          int
	LiveFrameMaxWidth          int
	LiveFrameHeightMargin      int
	MaxExpandedLiveToolEvents  int
	FullLogDefaultWidth        int
	FullLogDefaultHeight       int
	FullLogMinWidth            int
	FullLogMinHeight           int
	FullLogPollMilliseconds    int
	TogglePollMilliseconds     int
	MarkdownWordWrap           int
	ToolPreviewOutputChars     int
	ToolPreviewLongOutputChars int
	FileChangePreviewChars     int
	ApprovalArgsPreviewChars   int
}

func LoadFromEnv() (Config, error) {
	cfg, err := LoadFile(defaultConfigPath)
	if err != nil {
		return Config{}, err
	}
	cfg.APIKey = strings.TrimSpace(os.Getenv("AGENT_API_KEY"))
	if raw := strings.TrimSpace(os.Getenv("AGENT_BASE_URL")); raw != "" {
		cfg.LLM.BaseURL = raw
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_MODEL")); raw != "" {
		cfg.LLM.Model = raw
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_WIRE_API")); raw != "" {
		cfg.LLM.WireAPI = raw
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_LLM_MAX_RETRIES")); raw != "" {
		if err := setNonNegativeInt("AGENT_LLM_MAX_RETRIES", raw, &cfg.LLM.MaxRetries); err != nil {
			return Config{}, err
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_LLM_RETRY_BACKOFF_MILLISECONDS")); raw != "" {
		if err := setPositiveInt("AGENT_LLM_RETRY_BACKOFF_MILLISECONDS", raw, &cfg.LLM.RetryBackoffMS); err != nil {
			return Config{}, err
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_MAX_STEPS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("AGENT_MAX_STEPS must be a positive integer")
		}
		cfg.Agent.MaxSteps = n
		if cfg.Agent.AbsoluteMaxSteps < n {
			cfg.Agent.AbsoluteMaxSteps = n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_STEP_TIMING_ENABLED")); raw != "" {
		if err := setBool("AGENT_STEP_TIMING_ENABLED", raw, &cfg.Agent.StepTimingEnabled); err != nil {
			return Config{}, err
		}
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("AGENT_API_KEY is required")
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			BaseURL:               "https://anyrouter.top/v1",
			Model:                 "gpt-5.5",
			WireAPI:               "responses",
			RequestTimeoutSeconds: 300,
			MaxRetries:            1,
			RetryBackoffMS:        2000,
			ParallelToolCalls:     true,
		},
		Agent: AgentConfig{
			MaxSteps:                30,
			MaxParallelToolCalls:    10,
			StepTimingEnabled:       false,
			AdaptiveMaxStepsEnabled: true,
			MaxStepExtensions:       5,
			StepExtensionSize:       10,
			AbsoluteMaxSteps:        80,
		},
		Subagents: SubagentsConfig{
			Enabled:                 true,
			MaxConcurrent:           5,
			MaxSteps:                30,
			AdaptiveMaxStepsEnabled: true,
			MaxStepExtensions:       2,
			StepExtensionSize:       5,
			AbsoluteMaxSteps:        45,
			ResultMaxBytes:          16888,
		},
		Skills: SkillsConfig{
			Enabled:    true,
			UserDir:    "~/.echo-dust-code/skills",
			ProjectDir: "skills",
			TopK:       3,
			MinScore:   20,
		},
		Memory: MemoryConfig{
			Enabled: true,
			UserDir: "~/.echo-dust-code",
		},
		MCP: MCPConfig{
			Enabled:               true,
			Dir:                   "~/.echo-dust-code/mcp",
			StartTimeoutSeconds:   10,
			RequestTimeoutSeconds: 60,
		},
		Session: SessionConfig{
			Enabled: true,
			Dir:     "~/.echo-dust-code/session",
		},
		Context: ContextConfig{
			WindowTokens:             256000,
			PruneToolResultMaxBytes:  8192,
			PruneKeepRecentMessages:  16,
			CompactEnabled:           true,
			CompactRatioPercent:      80,
			CompactForceRatioPercent: 90,
			CompactTargetPercent:     50,
			CompactKeepTailTokens:    16000,
			CompactMinMessages:       4,
		},
		Tools: ToolsConfig{
			ListMaxEntries:               200,
			FindMaxMatches:               50,
			ReadFileMaxBytes:             64 * 1024,
			SearchMaxMatches:             100,
			SearchMaxFileBytes:           1024 * 1024,
			CommandDefaultTimeoutSeconds: 30,
			CommandMaxTimeoutSeconds:     120,
			CommandOutputMaxBytes:        64 * 1024,
			ApplyPatchTimeoutSeconds:     30,
			ApplyPatchOutputMaxBytes:     64 * 1024,
			FileChangePreviewLines:       20,
		},
		UI: UIConfig{
			SeparatorWidth:             80,
			LiveFrameMaxLines:          24,
			LiveFrameMaxWidth:          100,
			LiveFrameHeightMargin:      6,
			MaxExpandedLiveToolEvents:  6,
			FullLogDefaultWidth:        100,
			FullLogDefaultHeight:       24,
			FullLogMinWidth:            20,
			FullLogMinHeight:           6,
			FullLogPollMilliseconds:    30,
			TogglePollMilliseconds:     40,
			MarkdownWordWrap:           100,
			ToolPreviewOutputChars:     2000,
			ToolPreviewLongOutputChars: 4000,
			FileChangePreviewChars:     800,
			ApprovalArgsPreviewChars:   300,
		},
	}
}

func LoadFile(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	values, err := parseSimpleYAML(string(data))
	if err != nil {
		return Config{}, err
	}
	for key, value := range values {
		if err := applyConfigValue(&cfg, key, value); err != nil {
			return Config{}, err
		}
	}
	return cfg, validate(cfg)
}

func validate(cfg Config) error {
	checkPositive := map[string]int{
		"llm.request_timeout_seconds":           cfg.LLM.RequestTimeoutSeconds,
		"llm.retry_backoff_milliseconds":        cfg.LLM.RetryBackoffMS,
		"agent.max_steps":                       cfg.Agent.MaxSteps,
		"agent.max_parallel_tool_calls":         cfg.Agent.MaxParallelToolCalls,
		"agent.step_extension_size":             cfg.Agent.StepExtensionSize,
		"agent.absolute_max_steps":              cfg.Agent.AbsoluteMaxSteps,
		"subagents.max_concurrent":              cfg.Subagents.MaxConcurrent,
		"subagents.max_steps":                   cfg.Subagents.MaxSteps,
		"subagents.step_extension_size":         cfg.Subagents.StepExtensionSize,
		"subagents.absolute_max_steps":          cfg.Subagents.AbsoluteMaxSteps,
		"subagents.result_max_bytes":            cfg.Subagents.ResultMaxBytes,
		"skills.top_k":                          cfg.Skills.TopK,
		"mcp.start_timeout_seconds":             cfg.MCP.StartTimeoutSeconds,
		"mcp.request_timeout_seconds":           cfg.MCP.RequestTimeoutSeconds,
		"context.window_tokens":                 cfg.Context.WindowTokens,
		"context.prune_tool_result_max_bytes":   cfg.Context.PruneToolResultMaxBytes,
		"context.prune_keep_recent_messages":    cfg.Context.PruneKeepRecentMessages,
		"context.compact_ratio_percent":         cfg.Context.CompactRatioPercent,
		"context.compact_force_ratio_percent":   cfg.Context.CompactForceRatioPercent,
		"context.compact_target_percent":        cfg.Context.CompactTargetPercent,
		"context.compact_keep_tail_tokens":      cfg.Context.CompactKeepTailTokens,
		"context.compact_min_messages":          cfg.Context.CompactMinMessages,
		"tools.list_max_entries":                cfg.Tools.ListMaxEntries,
		"tools.find_max_matches":                cfg.Tools.FindMaxMatches,
		"tools.read_file_max_bytes":             cfg.Tools.ReadFileMaxBytes,
		"tools.search_max_matches":              cfg.Tools.SearchMaxMatches,
		"tools.search_max_file_bytes":           cfg.Tools.SearchMaxFileBytes,
		"tools.command_default_timeout_seconds": cfg.Tools.CommandDefaultTimeoutSeconds,
		"tools.command_max_timeout_seconds":     cfg.Tools.CommandMaxTimeoutSeconds,
		"tools.command_output_max_bytes":        cfg.Tools.CommandOutputMaxBytes,
		"tools.apply_patch_timeout_seconds":     cfg.Tools.ApplyPatchTimeoutSeconds,
		"tools.apply_patch_output_max_bytes":    cfg.Tools.ApplyPatchOutputMaxBytes,
		"tools.file_change_preview_lines":       cfg.Tools.FileChangePreviewLines,
		"ui.live_frame_max_lines":               cfg.UI.LiveFrameMaxLines,
		"ui.live_frame_height_margin":           cfg.UI.LiveFrameHeightMargin,
		"ui.max_expanded_live_tool_events":      cfg.UI.MaxExpandedLiveToolEvents,
		"ui.full_log_default_width":             cfg.UI.FullLogDefaultWidth,
		"ui.full_log_default_height":            cfg.UI.FullLogDefaultHeight,
		"ui.full_log_min_width":                 cfg.UI.FullLogMinWidth,
		"ui.full_log_min_height":                cfg.UI.FullLogMinHeight,
		"ui.full_log_poll_milliseconds":         cfg.UI.FullLogPollMilliseconds,
		"ui.toggle_poll_milliseconds":           cfg.UI.TogglePollMilliseconds,
		"ui.tool_preview_output_chars":          cfg.UI.ToolPreviewOutputChars,
		"ui.tool_preview_long_output_chars":     cfg.UI.ToolPreviewLongOutputChars,
		"ui.file_change_preview_chars":          cfg.UI.FileChangePreviewChars,
		"ui.approval_args_preview_chars":        cfg.UI.ApprovalArgsPreviewChars,
	}
	// SeparatorWidth/LiveFrameMaxWidth/MarkdownWordWrap 允许 0（表示使用终端宽度）。
	for key, value := range map[string]int{
		"ui.separator_width":      cfg.UI.SeparatorWidth,
		"ui.live_frame_max_width": cfg.UI.LiveFrameMaxWidth,
		"ui.markdown_word_wrap":   cfg.UI.MarkdownWordWrap,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", key)
		}
	}
	for key, value := range checkPositive {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	for key, value := range map[string]int{
		"llm.max_retries":               cfg.LLM.MaxRetries,
		"agent.max_step_extensions":     cfg.Agent.MaxStepExtensions,
		"subagents.max_step_extensions": cfg.Subagents.MaxStepExtensions,
		"skills.min_score":              cfg.Skills.MinScore,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", key)
		}
	}
	if cfg.Agent.AbsoluteMaxSteps < cfg.Agent.MaxSteps {
		return fmt.Errorf("agent.absolute_max_steps must be >= agent.max_steps")
	}
	if cfg.Subagents.AbsoluteMaxSteps < cfg.Subagents.MaxSteps {
		return fmt.Errorf("subagents.absolute_max_steps must be >= subagents.max_steps")
	}
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return fmt.Errorf("llm.base_url is required")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return fmt.Errorf("llm.model is required")
	}
	if cfg.MCP.Enabled && strings.TrimSpace(cfg.MCP.Dir) == "" {
		return fmt.Errorf("mcp.dir is required when mcp.enabled is true")
	}
	if cfg.Skills.Enabled && strings.TrimSpace(cfg.Skills.UserDir) == "" && strings.TrimSpace(cfg.Skills.ProjectDir) == "" {
		return fmt.Errorf("one of skills.user_dir or skills.project_dir is required when skills.enabled is true")
	}
	if cfg.Session.Enabled && strings.TrimSpace(cfg.Session.Dir) == "" {
		return fmt.Errorf("session.dir is required when session.enabled is true")
	}
	switch strings.TrimSpace(cfg.LLM.WireAPI) {
	case "chat_completions", "responses":
	default:
		return fmt.Errorf("llm.wire_api must be one of: chat_completions, responses")
	}
	if cfg.Tools.CommandDefaultTimeoutSeconds > cfg.Tools.CommandMaxTimeoutSeconds {
		return fmt.Errorf("tools.command_default_timeout_seconds must be <= tools.command_max_timeout_seconds")
	}
	if cfg.Context.CompactRatioPercent > cfg.Context.CompactForceRatioPercent {
		return fmt.Errorf("context.compact_ratio_percent must be <= context.compact_force_ratio_percent")
	}
	if cfg.Context.CompactTargetPercent >= cfg.Context.CompactRatioPercent {
		return fmt.Errorf("context.compact_target_percent must be < context.compact_ratio_percent")
	}
	for key, value := range map[string]int{
		"context.compact_ratio_percent":       cfg.Context.CompactRatioPercent,
		"context.compact_force_ratio_percent": cfg.Context.CompactForceRatioPercent,
		"context.compact_target_percent":      cfg.Context.CompactTargetPercent,
	} {
		if value > 100 {
			return fmt.Errorf("%s must be <= 100", key)
		}
	}
	return nil
}
