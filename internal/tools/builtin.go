package tools

func RegisterBuiltins(registry *Registry, workdir string) {
	registry.Register(&ListFilesTool{Workdir: workdir})
	registry.Register(&FindFilesTool{Workdir: workdir})
	registry.Register(&ReadFileTool{Workdir: workdir})
	registry.Register(&SearchFilesTool{Workdir: workdir})
	registry.Register(&WriteFileTool{Workdir: workdir})
	registry.Register(&ReplaceInFileTool{Workdir: workdir})
	registry.Register(&RunCommandTool{Workdir: workdir})
	registry.Register(&ApplyPatchTool{Workdir: workdir})
}
