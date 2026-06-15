package tools

type Options struct {
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

func DefaultOptions() Options {
	return Options{
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
	}
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.ListMaxEntries <= 0 {
		options.ListMaxEntries = defaults.ListMaxEntries
	}
	if options.FindMaxMatches <= 0 {
		options.FindMaxMatches = defaults.FindMaxMatches
	}
	if options.ReadFileMaxBytes <= 0 {
		options.ReadFileMaxBytes = defaults.ReadFileMaxBytes
	}
	if options.SearchMaxMatches <= 0 {
		options.SearchMaxMatches = defaults.SearchMaxMatches
	}
	if options.SearchMaxFileBytes <= 0 {
		options.SearchMaxFileBytes = defaults.SearchMaxFileBytes
	}
	if options.CommandDefaultTimeoutSeconds <= 0 {
		options.CommandDefaultTimeoutSeconds = defaults.CommandDefaultTimeoutSeconds
	}
	if options.CommandMaxTimeoutSeconds <= 0 {
		options.CommandMaxTimeoutSeconds = defaults.CommandMaxTimeoutSeconds
	}
	if options.CommandOutputMaxBytes <= 0 {
		options.CommandOutputMaxBytes = defaults.CommandOutputMaxBytes
	}
	if options.ApplyPatchTimeoutSeconds <= 0 {
		options.ApplyPatchTimeoutSeconds = defaults.ApplyPatchTimeoutSeconds
	}
	if options.ApplyPatchOutputMaxBytes <= 0 {
		options.ApplyPatchOutputMaxBytes = defaults.ApplyPatchOutputMaxBytes
	}
	if options.FileChangePreviewLines <= 0 {
		options.FileChangePreviewLines = defaults.FileChangePreviewLines
	}
	if options.CommandDefaultTimeoutSeconds > options.CommandMaxTimeoutSeconds {
		options.CommandDefaultTimeoutSeconds = options.CommandMaxTimeoutSeconds
	}
	return options
}
