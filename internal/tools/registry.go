package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

type Spec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type FileChange struct {
	Path         string `json:"path"`
	Action       string `json:"action,omitempty"`
	AddedLines   int    `json:"added_lines,omitempty"`
	RemovedLines int    `json:"removed_lines,omitempty"`
	Preview      string `json:"preview,omitempty"`
}

type Result struct {
	Status  string       `json:"status"`
	Summary string       `json:"summary,omitempty"`
	Output  string       `json:"output,omitempty"`
	Changes []FileChange `json:"changes,omitempty"`
}

func Success(summary, output string) Result {
	return Result{Status: "success", Summary: summary, Output: output}
}

func Error(summary string) Result {
	return Result{Status: "error", Summary: summary}
}

func (r Result) JSON() string {
	if r.Status == "" {
		r.Status = "success"
	}
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"status":"error","summary":%q}`, err.Error())
	}
	return string(data)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) All() []Tool {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, r.tools[name])
	}
	return out
}

func (r *Registry) Specs() []Spec {
	registered := r.All()
	specs := make([]Spec, 0, len(registered))
	for _, tool := range registered {
		specs = append(specs, Spec{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return specs
}
