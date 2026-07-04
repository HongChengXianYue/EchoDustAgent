package tools

func RegisterBuiltins(registry *Registry, workdir string) {
	RegisterBuiltinsWithOptions(registry, workdir, DefaultOptions())
}

func RegisterBuiltinsWithOptions(registry *Registry, workdir string, options Options) {
	options = normalizeOptions(options)
	registry.Register(&ListFilesTool{Workdir: workdir, MaxEntries: options.ListMaxEntries})
	registry.Register(&FindFilesTool{Workdir: workdir, DefaultMaxMatches: options.FindMaxMatches})
	registry.Register(&ReadFileTool{Workdir: workdir, MaxBytes: options.ReadFileMaxBytes})
	registry.Register(&ReadFileRangeTool{Workdir: workdir, MaxBytes: options.ReadFileMaxBytes})
	registry.Register(&SearchFilesTool{Workdir: workdir, MaxMatches: options.SearchMaxMatches, MaxFileBytes: int64(options.SearchMaxFileBytes)})
	registry.Register(&FindSymbolTool{Workdir: workdir})
	registry.Register(&FindReferencesTool{Workdir: workdir})
	registry.Register(&FindCallersTool{Workdir: workdir})
	registry.Register(&FindCalleesTool{Workdir: workdir})
	registry.Register(&WriteFileTool{Workdir: workdir, PreviewLines: options.FileChangePreviewLines})
	registry.Register(&ReplaceInFileTool{Workdir: workdir, PreviewLines: options.FileChangePreviewLines})
	registry.Register(&RunCommandTool{
		Workdir:               workdir,
		DefaultTimeoutSeconds: options.CommandDefaultTimeoutSeconds,
		MaxTimeoutSeconds:     options.CommandMaxTimeoutSeconds,
		OutputMaxBytes:        options.CommandOutputMaxBytes,
	})
	registry.Register(&ApplyPatchTool{
		Workdir:        workdir,
		TimeoutSeconds: options.ApplyPatchTimeoutSeconds,
		OutputMaxBytes: options.ApplyPatchOutputMaxBytes,
		PreviewLines:   options.FileChangePreviewLines,
	})
	registry.Register(&GitStatusTool{Workdir: workdir})
	registry.Register(&GitDiffTool{
		Workdir:        workdir,
		OutputMaxBytes: options.CommandOutputMaxBytes,
		PreviewLines:   options.FileChangePreviewLines,
	})
	registry.Register(&GitLogTool{Workdir: workdir})
}
