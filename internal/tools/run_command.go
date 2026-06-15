package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

type RunCommandTool struct {
	Workdir               string
	DefaultTimeoutSeconds int
	MaxTimeoutSeconds     int
	OutputMaxBytes        int
}

func (t *RunCommandTool) Name() string {
	return "run_command"
}

func (t *RunCommandTool) Description() string {
	return "Run a shell command in the workdir and return combined stdout/stderr."
}

func (t *RunCommandTool) Parameters() json.RawMessage {
	defaultTimeout := t.DefaultTimeoutSeconds
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultOptions().CommandDefaultTimeoutSeconds
	}
	maxTimeout := t.MaxTimeoutSeconds
	if maxTimeout <= 0 {
		maxTimeout = DefaultOptions().CommandMaxTimeoutSeconds
	}
	return schemaObject([]string{"command"}, map[string]any{
		"command": map[string]any{
			"type":        "string",
			"description": "Shell command to run.",
		},
		"timeout_seconds": map[string]any{
			"type":        "integer",
			"description": fmt.Sprintf("Optional timeout in seconds. Defaults to %d and is capped at %d.", defaultTimeout, maxTimeout),
		},
	})
}

func (t *RunCommandTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if params.Command == "" {
		return Error("command is required"), nil
	}
	timeout := params.TimeoutSeconds
	if timeout <= 0 {
		timeout = t.DefaultTimeoutSeconds
	}
	if timeout <= 0 {
		timeout = DefaultOptions().CommandDefaultTimeoutSeconds
	}
	maxTimeout := t.MaxTimeoutSeconds
	if maxTimeout <= 0 {
		maxTimeout = DefaultOptions().CommandMaxTimeoutSeconds
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	name, commandArgs := shellCommand(params.Command)
	cmd := exec.CommandContext(commandCtx, name, commandArgs...)
	cmd.Dir = t.Workdir
	output, err := cmd.CombinedOutput()
	maxOutput := t.OutputMaxBytes
	if maxOutput <= 0 {
		maxOutput = DefaultOptions().CommandOutputMaxBytes
	}
	text := capOutput(string(output), maxOutput)
	if commandCtx.Err() == context.DeadlineExceeded {
		return Error(fmt.Sprintf("command timed out after %d seconds", timeout)), nil
	}
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: text}, nil
	}
	return Success("command completed", text), nil
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "bash", []string{"-lc", command}
}
