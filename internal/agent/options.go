package agent

type Options struct {
	Subagents SubagentOptions
}

type SubagentOptions struct {
	Enabled        bool
	MaxConcurrent  int
	MaxSteps       int
	ResultMaxBytes int
}

func DefaultOptions() Options {
	return Options{
		Subagents: SubagentOptions{
			Enabled:        true,
			MaxConcurrent:  2,
			MaxSteps:       8,
			ResultMaxBytes: 12 * 1024,
		},
	}
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.Subagents.MaxConcurrent <= 0 {
		options.Subagents.MaxConcurrent = defaults.Subagents.MaxConcurrent
	}
	if options.Subagents.MaxSteps <= 0 {
		options.Subagents.MaxSteps = defaults.Subagents.MaxSteps
	}
	if options.Subagents.ResultMaxBytes <= 0 {
		options.Subagents.ResultMaxBytes = defaults.Subagents.ResultMaxBytes
	}
	return options
}
