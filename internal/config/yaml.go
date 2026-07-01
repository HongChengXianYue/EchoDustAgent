package config

import (
	"fmt"
	"strconv"
	"strings"
)

func parseSimpleYAML(text string) (map[string]string, error) {
	values := map[string]string{}
	var section string
	for lineNo, raw := range strings.Split(text, "\n") {
		line := stripYAMLComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") {
			if indent != 0 {
				return nil, fmt.Errorf("config.yaml:%d nested sections are not supported", lineNo+1)
			}
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("config.yaml:%d expected key: value", lineNo+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("config.yaml:%d empty key", lineNo+1)
		}
		if section != "" {
			key = section + "." + key
		}
		values[key] = unquoteYAMLScalar(value)
	}
	return values, nil
}

func stripYAMLComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func unquoteYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
		if value[0] == '"' && value[len(value)-1] == '"' {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
		}
	}
	return value
}

func applyConfigValue(cfg *Config, key string, value string) error {
	switch key {
	case "llm.base_url":
		cfg.LLM.BaseURL = value
	case "llm.model":
		cfg.LLM.Model = value
	case "llm.wire_api":
		cfg.LLM.WireAPI = value
	case "llm.request_timeout_seconds":
		return setPositiveInt(key, value, &cfg.LLM.RequestTimeoutSeconds)
	case "llm.parallel_tool_calls":
		return setBool(key, value, &cfg.LLM.ParallelToolCalls)
	case "agent.max_steps":
		return setPositiveInt(key, value, &cfg.Agent.MaxSteps)
	case "agent.max_parallel_tool_calls":
		return setPositiveInt(key, value, &cfg.Agent.MaxParallelToolCalls)
	case "agent.adaptive_max_steps_enabled":
		return setBool(key, value, &cfg.Agent.AdaptiveMaxStepsEnabled)
	case "agent.max_step_extensions":
		return setNonNegativeInt(key, value, &cfg.Agent.MaxStepExtensions)
	case "agent.step_extension_size":
		return setPositiveInt(key, value, &cfg.Agent.StepExtensionSize)
	case "agent.absolute_max_steps":
		return setPositiveInt(key, value, &cfg.Agent.AbsoluteMaxSteps)
	case "subagents.enabled":
		return setBool(key, value, &cfg.Subagents.Enabled)
	case "subagents.max_concurrent":
		return setPositiveInt(key, value, &cfg.Subagents.MaxConcurrent)
	case "subagents.max_steps":
		return setPositiveInt(key, value, &cfg.Subagents.MaxSteps)
	case "subagents.adaptive_max_steps_enabled":
		return setBool(key, value, &cfg.Subagents.AdaptiveMaxStepsEnabled)
	case "subagents.max_step_extensions":
		return setNonNegativeInt(key, value, &cfg.Subagents.MaxStepExtensions)
	case "subagents.step_extension_size":
		return setPositiveInt(key, value, &cfg.Subagents.StepExtensionSize)
	case "subagents.absolute_max_steps":
		return setPositiveInt(key, value, &cfg.Subagents.AbsoluteMaxSteps)
	case "subagents.result_max_bytes":
		return setPositiveInt(key, value, &cfg.Subagents.ResultMaxBytes)
	case "memory.enabled":
		return setBool(key, value, &cfg.Memory.Enabled)
	case "memory.user_dir":
		cfg.Memory.UserDir = value
	case "mcp.enabled":
		return setBool(key, value, &cfg.MCP.Enabled)
	case "mcp.dir":
		cfg.MCP.Dir = value
	case "mcp.start_timeout_seconds":
		return setPositiveInt(key, value, &cfg.MCP.StartTimeoutSeconds)
	case "mcp.request_timeout_seconds":
		return setPositiveInt(key, value, &cfg.MCP.RequestTimeoutSeconds)
	case "context.window_tokens":
		return setPositiveInt(key, value, &cfg.Context.WindowTokens)
	case "context.prune_tool_result_max_bytes":
		return setPositiveInt(key, value, &cfg.Context.PruneToolResultMaxBytes)
	case "context.prune_keep_recent_messages":
		return setPositiveInt(key, value, &cfg.Context.PruneKeepRecentMessages)
	case "context.compact_enabled":
		return setBool(key, value, &cfg.Context.CompactEnabled)
	case "context.compact_ratio_percent":
		return setPositiveInt(key, value, &cfg.Context.CompactRatioPercent)
	case "context.compact_force_ratio_percent":
		return setPositiveInt(key, value, &cfg.Context.CompactForceRatioPercent)
	case "context.compact_target_percent":
		return setPositiveInt(key, value, &cfg.Context.CompactTargetPercent)
	case "context.compact_keep_tail_tokens":
		return setPositiveInt(key, value, &cfg.Context.CompactKeepTailTokens)
	case "context.compact_min_messages":
		return setPositiveInt(key, value, &cfg.Context.CompactMinMessages)
	case "tools.list_max_entries":
		return setPositiveInt(key, value, &cfg.Tools.ListMaxEntries)
	case "tools.find_max_matches":
		return setPositiveInt(key, value, &cfg.Tools.FindMaxMatches)
	case "tools.read_file_max_bytes":
		return setPositiveInt(key, value, &cfg.Tools.ReadFileMaxBytes)
	case "tools.search_max_matches":
		return setPositiveInt(key, value, &cfg.Tools.SearchMaxMatches)
	case "tools.search_max_file_bytes":
		return setPositiveInt(key, value, &cfg.Tools.SearchMaxFileBytes)
	case "tools.command_default_timeout_seconds":
		return setPositiveInt(key, value, &cfg.Tools.CommandDefaultTimeoutSeconds)
	case "tools.command_max_timeout_seconds":
		return setPositiveInt(key, value, &cfg.Tools.CommandMaxTimeoutSeconds)
	case "tools.command_output_max_bytes":
		return setPositiveInt(key, value, &cfg.Tools.CommandOutputMaxBytes)
	case "tools.apply_patch_timeout_seconds":
		return setPositiveInt(key, value, &cfg.Tools.ApplyPatchTimeoutSeconds)
	case "tools.apply_patch_output_max_bytes":
		return setPositiveInt(key, value, &cfg.Tools.ApplyPatchOutputMaxBytes)
	case "tools.file_change_preview_lines":
		return setPositiveInt(key, value, &cfg.Tools.FileChangePreviewLines)
	case "ui.separator_width":
		return setNonNegativeInt(key, value, &cfg.UI.SeparatorWidth)
	case "ui.live_frame_max_lines":
		return setPositiveInt(key, value, &cfg.UI.LiveFrameMaxLines)
	case "ui.live_frame_max_width":
		return setNonNegativeInt(key, value, &cfg.UI.LiveFrameMaxWidth)
	case "ui.live_frame_height_margin":
		return setPositiveInt(key, value, &cfg.UI.LiveFrameHeightMargin)
	case "ui.max_expanded_live_tool_events":
		return setPositiveInt(key, value, &cfg.UI.MaxExpandedLiveToolEvents)
	case "ui.full_log_default_width":
		return setPositiveInt(key, value, &cfg.UI.FullLogDefaultWidth)
	case "ui.full_log_default_height":
		return setPositiveInt(key, value, &cfg.UI.FullLogDefaultHeight)
	case "ui.full_log_min_width":
		return setPositiveInt(key, value, &cfg.UI.FullLogMinWidth)
	case "ui.full_log_min_height":
		return setPositiveInt(key, value, &cfg.UI.FullLogMinHeight)
	case "ui.full_log_poll_milliseconds":
		return setPositiveInt(key, value, &cfg.UI.FullLogPollMilliseconds)
	case "ui.toggle_poll_milliseconds":
		return setPositiveInt(key, value, &cfg.UI.TogglePollMilliseconds)
	case "ui.markdown_word_wrap":
		return setNonNegativeInt(key, value, &cfg.UI.MarkdownWordWrap)
	case "ui.tool_preview_output_chars":
		return setPositiveInt(key, value, &cfg.UI.ToolPreviewOutputChars)
	case "ui.tool_preview_long_output_chars":
		return setPositiveInt(key, value, &cfg.UI.ToolPreviewLongOutputChars)
	case "ui.file_change_preview_chars":
		return setPositiveInt(key, value, &cfg.UI.FileChangePreviewChars)
	case "ui.approval_args_preview_chars":
		return setPositiveInt(key, value, &cfg.UI.ApprovalArgsPreviewChars)
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func setPositiveInt(key string, value string, target *int) error {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	*target = n
	return nil
}

func setNonNegativeInt(key string, value string, target *int) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return fmt.Errorf("%s must be a non-negative integer", key)
	}
	*target = n
	return nil
}

func setBool(key string, value string, target *bool) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "on", "1":
		*target = true
	case "false", "no", "off", "0":
		*target = false
	default:
		return fmt.Errorf("%s must be a boolean", key)
	}
	return nil
}
