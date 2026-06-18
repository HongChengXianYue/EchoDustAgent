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
	Memory    MemoryConfig
	Tools     ToolsConfig
	UI        UIConfig
}

type LLMConfig struct {
	BaseURL               string
	Model                 string
	RequestTimeoutSeconds int
	ParallelToolCalls     bool
}

type AgentConfig struct {
	MaxSteps             int
	MaxParallelToolCalls int
}

type SubagentsConfig struct {
	Enabled        bool
	MaxConcurrent  int
	MaxSteps       int
	ResultMaxBytes int
}

type MemoryConfig struct {
	Enabled bool
	UserDir string
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
	if raw := strings.TrimSpace(os.Getenv("AGENT_MAX_STEPS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("AGENT_MAX_STEPS must be a positive integer")
		}
		cfg.Agent.MaxSteps = n
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
			BaseURL:               "https://api.openai.com/v1",
			Model:                 "gpt-4.1-mini",
			RequestTimeoutSeconds: 120,
			ParallelToolCalls:     true,
		},
		Agent: AgentConfig{
			MaxSteps:             20,
			MaxParallelToolCalls: 10,
		},
		Subagents: SubagentsConfig{
			Enabled:        true,
			MaxConcurrent:  2,
			MaxSteps:       8,
			ResultMaxBytes: 12 * 1024,
		},
		Memory: MemoryConfig{
			Enabled: true,
			UserDir: "~/.local-agent",
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
		"agent.max_steps":                       cfg.Agent.MaxSteps,
		"agent.max_parallel_tool_calls":         cfg.Agent.MaxParallelToolCalls,
		"subagents.max_concurrent":              cfg.Subagents.MaxConcurrent,
		"subagents.max_steps":                   cfg.Subagents.MaxSteps,
		"subagents.result_max_bytes":            cfg.Subagents.ResultMaxBytes,
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
		"ui.separator_width":                    cfg.UI.SeparatorWidth,
		"ui.live_frame_max_lines":               cfg.UI.LiveFrameMaxLines,
		"ui.live_frame_max_width":               cfg.UI.LiveFrameMaxWidth,
		"ui.live_frame_height_margin":           cfg.UI.LiveFrameHeightMargin,
		"ui.max_expanded_live_tool_events":      cfg.UI.MaxExpandedLiveToolEvents,
		"ui.full_log_default_width":             cfg.UI.FullLogDefaultWidth,
		"ui.full_log_default_height":            cfg.UI.FullLogDefaultHeight,
		"ui.full_log_min_width":                 cfg.UI.FullLogMinWidth,
		"ui.full_log_min_height":                cfg.UI.FullLogMinHeight,
		"ui.full_log_poll_milliseconds":         cfg.UI.FullLogPollMilliseconds,
		"ui.toggle_poll_milliseconds":           cfg.UI.TogglePollMilliseconds,
		"ui.markdown_word_wrap":                 cfg.UI.MarkdownWordWrap,
		"ui.tool_preview_output_chars":          cfg.UI.ToolPreviewOutputChars,
		"ui.tool_preview_long_output_chars":     cfg.UI.ToolPreviewLongOutputChars,
		"ui.file_change_preview_chars":          cfg.UI.FileChangePreviewChars,
		"ui.approval_args_preview_chars":        cfg.UI.ApprovalArgsPreviewChars,
	}
	for key, value := range checkPositive {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return fmt.Errorf("llm.base_url is required")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return fmt.Errorf("llm.model is required")
	}
	if cfg.Tools.CommandDefaultTimeoutSeconds > cfg.Tools.CommandMaxTimeoutSeconds {
		return fmt.Errorf("tools.command_default_timeout_seconds must be <= tools.command_max_timeout_seconds")
	}
	return nil
}
