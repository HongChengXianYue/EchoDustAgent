package ui

type Options struct {
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

func DefaultOptions() Options {
	return Options{
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
	}
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.SeparatorWidth <= 0 {
		options.SeparatorWidth = defaults.SeparatorWidth
	}
	if options.LiveFrameMaxLines <= 0 {
		options.LiveFrameMaxLines = defaults.LiveFrameMaxLines
	}
	if options.LiveFrameMaxWidth <= 0 {
		options.LiveFrameMaxWidth = defaults.LiveFrameMaxWidth
	}
	if options.LiveFrameHeightMargin <= 0 {
		options.LiveFrameHeightMargin = defaults.LiveFrameHeightMargin
	}
	if options.MaxExpandedLiveToolEvents <= 0 {
		options.MaxExpandedLiveToolEvents = defaults.MaxExpandedLiveToolEvents
	}
	if options.FullLogDefaultWidth <= 0 {
		options.FullLogDefaultWidth = defaults.FullLogDefaultWidth
	}
	if options.FullLogDefaultHeight <= 0 {
		options.FullLogDefaultHeight = defaults.FullLogDefaultHeight
	}
	if options.FullLogMinWidth <= 0 {
		options.FullLogMinWidth = defaults.FullLogMinWidth
	}
	if options.FullLogMinHeight <= 0 {
		options.FullLogMinHeight = defaults.FullLogMinHeight
	}
	if options.FullLogPollMilliseconds <= 0 {
		options.FullLogPollMilliseconds = defaults.FullLogPollMilliseconds
	}
	if options.TogglePollMilliseconds <= 0 {
		options.TogglePollMilliseconds = defaults.TogglePollMilliseconds
	}
	if options.MarkdownWordWrap <= 0 {
		options.MarkdownWordWrap = defaults.MarkdownWordWrap
	}
	if options.ToolPreviewOutputChars <= 0 {
		options.ToolPreviewOutputChars = defaults.ToolPreviewOutputChars
	}
	if options.ToolPreviewLongOutputChars <= 0 {
		options.ToolPreviewLongOutputChars = defaults.ToolPreviewLongOutputChars
	}
	if options.FileChangePreviewChars <= 0 {
		options.FileChangePreviewChars = defaults.FileChangePreviewChars
	}
	if options.ApprovalArgsPreviewChars <= 0 {
		options.ApprovalArgsPreviewChars = defaults.ApprovalArgsPreviewChars
	}
	return options
}
