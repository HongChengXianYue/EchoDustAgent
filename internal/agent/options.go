package agent

import (
	contextmgr "local-agent/internal/context"
	"local-agent/internal/skill"
)

type Options struct {
	MaxParallelToolCalls int
	// StepTimingEnabled controls whether per-step timing events are emitted.
	// Total run timing remains enabled regardless of this flag.
	StepTimingEnabled    bool
	StepBudget           StepBudgetOptions
	Subagents            SubagentOptions
	Skills               SkillOptions
	SystemPromptSuffix   string
	Context              ContextOptions
}

type StepBudgetOptions struct {
	AdaptiveEnabled  bool
	MaxExtensions    int
	ExtensionSize    int
	AbsoluteMaxSteps int
}

type SubagentOptions struct {
	Enabled        bool
	MaxConcurrent  int
	MaxSteps       int
	StepBudget     StepBudgetOptions
	ResultMaxBytes int
}

type SkillOptions struct {
	Enabled  bool
	TopK     int
	MinScore int
	Registry *skill.Registry
}

type ContextOptions = contextmgr.Options

func DefaultOptions() Options {
	return Options{
		MaxParallelToolCalls: 10,
		StepBudget: StepBudgetOptions{
			AdaptiveEnabled:  true,
			MaxExtensions:    3,
			ExtensionSize:    10,
			AbsoluteMaxSteps: 80,
		},
		Subagents: SubagentOptions{
			Enabled:        true,
			MaxConcurrent:  2,
			MaxSteps:       8,
			ResultMaxBytes: 12 * 1024,
			StepBudget: StepBudgetOptions{
				AdaptiveEnabled:  true,
				MaxExtensions:    2,
				ExtensionSize:    5,
				AbsoluteMaxSteps: 45,
			},
		},
		Skills: SkillOptions{
			Enabled:  true,
			TopK:     3,
			MinScore: 20,
		},
		Context: ContextOptions{
			WindowTokens:             128000,
			PruneToolResultMaxBytes:  8192,
			PruneKeepRecentMessages:  16,
			CompactEnabled:           true,
			CompactRatioPercent:      80,
			CompactForceRatioPercent: 90,
			CompactTargetPercent:     50,
			CompactKeepTailTokens:    16000,
			CompactMinMessages:       4,
		},
	}
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.MaxParallelToolCalls <= 0 {
		options.MaxParallelToolCalls = defaults.MaxParallelToolCalls
	}
	options.StepBudget = normalizeStepBudgetOptions(options.StepBudget, defaults.StepBudget)
	if options.Subagents.MaxConcurrent <= 0 {
		options.Subagents.MaxConcurrent = defaults.Subagents.MaxConcurrent
	}
	if options.Subagents.MaxSteps <= 0 {
		options.Subagents.MaxSteps = defaults.Subagents.MaxSteps
	}
	options.Subagents.StepBudget = normalizeStepBudgetOptions(options.Subagents.StepBudget, defaults.Subagents.StepBudget)
	if options.Subagents.ResultMaxBytes <= 0 {
		options.Subagents.ResultMaxBytes = defaults.Subagents.ResultMaxBytes
	}
	if options.Skills.TopK <= 0 {
		options.Skills.TopK = defaults.Skills.TopK
	}
	if options.Skills.MinScore < 0 {
		options.Skills.MinScore = defaults.Skills.MinScore
	}
	if options.Context.WindowTokens <= 0 {
		options.Context.WindowTokens = defaults.Context.WindowTokens
	}
	if options.Context.PruneToolResultMaxBytes <= 0 {
		options.Context.PruneToolResultMaxBytes = defaults.Context.PruneToolResultMaxBytes
	}
	if options.Context.PruneKeepRecentMessages <= 0 {
		options.Context.PruneKeepRecentMessages = defaults.Context.PruneKeepRecentMessages
	}
	if options.Context.CompactRatioPercent <= 0 {
		options.Context.CompactRatioPercent = defaults.Context.CompactRatioPercent
	}
	if options.Context.CompactForceRatioPercent <= 0 {
		options.Context.CompactForceRatioPercent = defaults.Context.CompactForceRatioPercent
	}
	if options.Context.CompactTargetPercent <= 0 {
		options.Context.CompactTargetPercent = defaults.Context.CompactTargetPercent
	}
	if options.Context.CompactKeepTailTokens <= 0 {
		options.Context.CompactKeepTailTokens = defaults.Context.CompactKeepTailTokens
	}
	if options.Context.CompactMinMessages <= 0 {
		options.Context.CompactMinMessages = defaults.Context.CompactMinMessages
	}
	return options
}

func normalizeStepBudgetOptions(options StepBudgetOptions, defaults StepBudgetOptions) StepBudgetOptions {
	if options.MaxExtensions < 0 {
		options.MaxExtensions = defaults.MaxExtensions
	}
	if options.ExtensionSize <= 0 {
		options.ExtensionSize = defaults.ExtensionSize
	}
	if options.AbsoluteMaxSteps <= 0 {
		options.AbsoluteMaxSteps = defaults.AbsoluteMaxSteps
	}
	return options
}
