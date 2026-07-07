package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/skill"
	"local-agent/internal/tools"
)

type invokeSkillTool struct {
	agent *Agent
}

type invokeSkillArgs struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

func (t *invokeSkillTool) Name() string {
	return tools.InvokeSkillToolName
}

func (t *invokeSkillTool) Description() string {
	return "Lazily load and execute one optional skill matched for the current user request."
}

func (t *invokeSkillTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"name":{"type":"string"},
			"input":{"type":"object"}
		},
		"required":["name"],
		"additionalProperties":false
	}`)
}

func (t *invokeSkillTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	if t == nil || t.agent == nil {
		return tools.Error("skill runtime is not configured"), nil
	}
	return t.agent.runSkill(ctx, args), nil
}

func (a *Agent) skillFunctionTool() *llm.FunctionTool {
	if a.skillTool == nil || len(a.activeSkills) == 0 {
		return nil
	}
	names := make([]string, 0, len(a.activeSkills))
	lines := []string{
		"Lazily load and execute one optional skill matched for the current user request.",
		"Only call this when one of the available skills clearly fits the task. The skill body is loaded on demand and runs in an isolated internal agent with only its declared tools.",
		"Available skills:",
	}
	for _, candidate := range a.activeSkillList() {
		names = append(names, candidate.Skill.Name)
		lines = append(lines, fmt.Sprintf("- %s: %s | input: %s | tools: %s",
			candidate.Skill.Name,
			candidate.Skill.Summary,
			candidate.Skill.InputSchemaSummary(),
			candidate.Skill.PermissionSummary(),
		))
	}
	return &llm.FunctionTool{
		Name:        a.skillTool.Name(),
		Description: strings.Join(lines, "\n"),
		Parameters:  invokeSkillParameters(names),
	}
}

func invokeSkillParameters(names []string) json.RawMessage {
	nameEnum, err := json.Marshal(names)
	if err != nil {
		nameEnum = []byte(`[]`)
	}
	return json.RawMessage(fmt.Sprintf(`{
		"type":"object",
		"properties":{
			"name":{"type":"string","enum":%s},
			"input":{"type":"object","description":"Skill-specific input object. Follow the selected skill's input schema from the current system context."}
		},
		"required":["name"],
		"additionalProperties":false
	}`, string(nameEnum)))
}

func (a *Agent) activeSkillList() []skill.Candidate {
	if len(a.activeSkills) == 0 {
		return nil
	}
	out := make([]skill.Candidate, 0, len(a.activeSkills))
	for _, candidate := range a.activeSkills {
		out = append(out, candidate)
	}
	sortSkillCandidates(out)
	return out
}

func sortSkillCandidates(candidates []skill.Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return strings.ToLower(candidates[i].Skill.Name) < strings.ToLower(candidates[j].Skill.Name)
	})
}

func (a *Agent) runSkill(ctx context.Context, args json.RawMessage) tools.Result {
	params, err := parseInvokeSkillArgs(args)
	if err != nil {
		return tools.Error(err.Error())
	}
	candidate, ok := a.activeSkills[strings.ToLower(params.Name)]
	if !ok {
		return tools.Error(fmt.Sprintf("skill %q is not available for the current request", params.Name))
	}
	if err := skill.ValidateInput(candidate.Skill.InputSchema, params.Input); err != nil {
		return tools.Error(fmt.Sprintf("invalid input for skill %q: %v", candidate.Skill.Name, err))
	}
	registry, err := a.skillRegistryFor(candidate.Skill)
	if err != nil {
		return tools.Error(err.Error())
	}
	body, err := candidate.Skill.ReadBody()
	if err != nil {
		return tools.Error(fmt.Sprintf("read skill %q body: %v", candidate.Skill.Name, err))
	}
	options := a.options
	options.Subagents.Enabled = false
	options.Skills.Enabled = false
	prompt := skillSystemPrompt(a.workspace, options.MaxParallelToolCalls, candidate.Skill, body)
	if suffix := strings.TrimSpace(options.SystemPromptSuffix); suffix != "" {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + suffix
	}
	skillAgent := newAgent(a.client, registry, a.options.Subagents.MaxSteps, a.workspace, prompt, options)
	skillAgent.SetApprover(a.approver)
	answer, err := skillAgent.Run(ctx, skillUserInput(candidate.Skill, params.Input))
	if err != nil {
		return tools.Error("skill failed: " + err.Error())
	}
	answer = truncateUTF8Bytes(strings.TrimSpace(answer), a.options.Subagents.ResultMaxBytes)
	if answer == "" {
		answer = "(skill returned no final answer)"
	}
	return tools.Success("skill completed", answer)
}

func parseInvokeSkillArgs(args json.RawMessage) (invokeSkillArgs, error) {
	var params invokeSkillArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return invokeSkillArgs{}, fmt.Errorf("invalid invoke_skill arguments: %w", err)
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return invokeSkillArgs{}, fmt.Errorf("name must be non-empty")
	}
	if len(strings.TrimSpace(string(params.Input))) == 0 {
		params.Input = json.RawMessage(`{}`)
	}
	return params, nil
}

func (a *Agent) skillRegistryFor(selected skill.Skill) (*tools.Registry, error) {
	registry := tools.NewRegistry()
	for _, name := range selected.Permissions.Tools {
		tool, ok := a.registry.Get(name)
		if !ok {
			return nil, fmt.Errorf("skill %q requires unavailable tool %q", selected.Name, name)
		}
		registry.Register(tool)
	}
	return registry, nil
}

func skillSystemPrompt(workspace string, maxParallelToolCalls int, selected skill.Skill, body string) string {
	if maxParallelToolCalls <= 0 {
		maxParallelToolCalls = DefaultOptions().MaxParallelToolCalls
	}
	lines := []string{
		"# Role",
		fmt.Sprintf("You are executing the on-demand skill %q.", selected.Name),
		"",
		"# Skill Contract",
		"Follow the provided skill instructions exactly for this run.",
		"Use only the tools exposed to you. The runtime already restricted them to the skill's declared permissions.",
		"Do not call delegate_task or invoke_skill. They are intentionally unavailable inside a skill run.",
		fmt.Sprintf("Do not return more than %d non-planning tool calls in one assistant turn. update_todos and engineering_checklist are planning/guidance tools and do not count toward this limit. Multiple calls to the same non-planning tool with different arguments count separately.", maxParallelToolCalls),
		"",
		"# Skill Metadata",
		"Description: " + selected.Description,
		"Input schema: " + selected.InputSchemaSummary(),
		"Allowed tools: " + selected.PermissionSummary(),
		"Trigger scenarios: " + selected.TriggerSummary(),
		"",
		"# Skill Instructions",
		body,
		"",
		"# Final Answer",
		"Return only the final result of the skill execution. Keep it concise and task-focused.",
	}
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		lines = append(lines[:1], append([]string{"Current workspace: " + workspace}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func skillUserInput(selected skill.Skill, input json.RawMessage) string {
	if len(strings.TrimSpace(string(input))) == 0 || string(input) == "{}" {
		return "Execute skill " + selected.Name + "."
	}
	formatted := string(input)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, input, "", "  "); err == nil {
		formatted = pretty.String()
	}
	return "Skill input:\n" + formatted
}
