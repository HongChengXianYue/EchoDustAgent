package agent

import (
	"local-agent/internal/llm"
	"local-agent/internal/tools"
)

func (a *Agent) functionTools() []llm.FunctionTool {
	specs := a.registry.Specs()
	out := make([]llm.FunctionTool, 0, len(specs)+2)
	out = append(out, functionToolFromTool(a.todoTool))
	if a.subagentTool != nil {
		out = append(out, functionToolFromTool(a.subagentTool))
	}
	for _, spec := range specs {
		out = append(out, llm.FunctionTool{
			Name:        spec.Name,
			Description: spec.Description,
			Parameters:  spec.Parameters,
		})
	}
	return out
}

func (a *Agent) lookupTool(name string) (tools.Tool, bool) {
	if tools.IsDelegateTaskTool(name) && a.subagentTool != nil {
		return a.subagentTool, true
	}
	return a.registry.Get(name)
}

func functionToolFromTool(tool tools.Tool) llm.FunctionTool {
	return llm.FunctionTool{
		Name:        tool.Name(),
		Description: tool.Description(),
		Parameters:  tool.Parameters(),
	}
}
