package agent

type Options struct {
	MaxParallelToolCalls int
	Subagents            SubagentOptions
	SystemPromptSuffix   string
	Context              ContextOptions
}

type SubagentOptions struct {
	Enabled        bool
	MaxConcurrent  int
	MaxSteps       int
	ResultMaxBytes int
}

type ContextOptions struct {
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

func DefaultOptions() Options {
	return Options{
		MaxParallelToolCalls: 10,
		Subagents: SubagentOptions{
			Enabled:        true,
			MaxConcurrent:  2,
			MaxSteps:       8,
			ResultMaxBytes: 12 * 1024,
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
	if options.Subagents.MaxConcurrent <= 0 {
		options.Subagents.MaxConcurrent = defaults.Subagents.MaxConcurrent
	}
	if options.Subagents.MaxSteps <= 0 {
		options.Subagents.MaxSteps = defaults.Subagents.MaxSteps
	}
	if options.Subagents.ResultMaxBytes <= 0 {
		options.Subagents.ResultMaxBytes = defaults.Subagents.ResultMaxBytes
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
