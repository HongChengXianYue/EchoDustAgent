package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type GitStatusTool struct {
	Workdir string
}

func (t *GitStatusTool) Name() string {
	return "git_status"
}

func (t *GitStatusTool) Description() string {
	return "Show a concise git status for the current workspace."
}

func (t *GitStatusTool) Parameters() json.RawMessage {
	return schemaObject(nil, map[string]any{})
}

func (t *GitStatusTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	if _, err := parseEmptyArgs(args); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	output, err := runCommandOutput(ctx, t.Workdir, nil, "git", "status", "--short", "--branch")
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: output}, nil
	}
	if strings.TrimSpace(output) == "" {
		output = "(clean working tree)"
	}
	return Success("git status completed", output), nil
}

type GitDiffTool struct {
	Workdir        string
	OutputMaxBytes int
	PreviewLines   int
}

func (t *GitDiffTool) Name() string {
	return "git_diff"
}

func (t *GitDiffTool) Description() string {
	return "Show git diff for the workspace, optionally limited to one path or staged changes."
}

func (t *GitDiffTool) Parameters() json.RawMessage {
	return schemaObject(nil, map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Optional workdir-relative file path to diff.",
		},
		"staged": map[string]any{
			"type":        "boolean",
			"description": "Show staged diff when true.",
		},
	})
}

func (t *GitDiffTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Path   string `json:"path"`
		Staged bool   `json:"staged"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	commandArgs := []string{"diff", "--no-ext-diff"}
	if params.Staged {
		commandArgs = append(commandArgs, "--cached")
	}
	if strings.TrimSpace(params.Path) != "" {
		path, err := resolvePath(t.Workdir, params.Path)
		if err != nil {
			return Error(err.Error()), nil
		}
		commandArgs = append(commandArgs, "--", displayPath(t.Workdir, path))
	}
	output, err := runCommandOutput(ctx, t.Workdir, nil, "git", commandArgs...)
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: output}, nil
	}
	maxBytes := t.OutputMaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultOptions().CommandOutputMaxBytes
	}
	previewLines := t.PreviewLines
	if previewLines <= 0 {
		previewLines = DefaultOptions().FileChangePreviewLines
	}
	cappedOutput := capOutput(output, maxBytes)
	if strings.TrimSpace(cappedOutput) == "" {
		return Success("git diff completed", "(no diff)"), nil
	}
	summary := "git diff completed"
	if cappedOutput != output {
		summary = fmt.Sprintf("git diff completed (truncated to %d bytes)", maxBytes)
	}
	diffText := strings.TrimSuffix(cappedOutput, "\n[truncated]")
	if changes := parseUnifiedDiffChanges(diffText, previewLines); len(changes) > 0 {
		result := Success(summary, "")
		result.Changes = changes
		return result, nil
	}
	return Success(summary, cappedOutput), nil
}

type GitLogTool struct {
	Workdir string
}

func (t *GitLogTool) Name() string {
	return "git_log"
}

func (t *GitLogTool) Description() string {
	return "Show recent git commits in a concise one-line format."
}

func (t *GitLogTool) Parameters() json.RawMessage {
	return schemaObject(nil, map[string]any{
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum number of commits to show. Defaults to 10.",
		},
	})
}

func (t *GitLogTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	output, err := runCommandOutput(ctx, t.Workdir, nil, "git", "log", fmt.Sprintf("-n%d", limit), "--date=short", "--pretty=format:%h%x09%ad%x09%an%x09%s")
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: output}, nil
	}
	if strings.TrimSpace(output) == "" {
		output = "(no commits)"
	}
	return Success(fmt.Sprintf("listed %d recent commit(s)", limit), output), nil
}

func parseEmptyArgs(args json.RawMessage) (struct{}, error) {
	if len(strings.TrimSpace(string(args))) == 0 || strings.TrimSpace(string(args)) == "null" {
		return struct{}{}, nil
	}
	var params struct{}
	return params, json.Unmarshal(args, &params)
}
