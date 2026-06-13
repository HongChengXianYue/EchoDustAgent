package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ApplyPatchTool struct {
	Workdir string
}

func (t *ApplyPatchTool) Name() string {
	return "apply_patch"
}

func (t *ApplyPatchTool) Description() string {
	return "Apply a unified diff patch to files inside the workdir."
}

func (t *ApplyPatchTool) Parameters() json.RawMessage {
	return schemaObject([]string{"patch"}, map[string]any{
		"patch": map[string]any{
			"type":        "string",
			"description": "Unified diff patch text.",
		},
	})
}

func (t *ApplyPatchTool) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	var params struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return Error("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(params.Patch) == "" {
		return Error("patch is required"), nil
	}
	strip, err := patchStripLevel(params.Patch)
	if err != nil {
		return Error(err.Error()), nil
	}

	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "patch", "--batch", "--forward", fmt.Sprintf("-p%d", strip))
	cmd.Dir = t.Workdir
	cmd.Stdin = strings.NewReader(params.Patch)
	output, err := cmd.CombinedOutput()
	text := capOutput(string(output), 64*1024)
	if commandCtx.Err() == context.DeadlineExceeded {
		return Error("patch timed out"), nil
	}
	if err != nil {
		return Result{Status: "error", Summary: err.Error(), Output: text}, nil
	}
	result := Success("patch applied", text)
	result.Changes = parseUnifiedDiffChanges(params.Patch)
	return result, nil
}

func patchStripLevel(patchText string) (int, error) {
	strip := 0
	for _, line := range strings.Split(patchText, "\n") {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			path := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(line, "--- "), "+++ "))
			if len(path) == 0 || path[0] == "/dev/null" {
				continue
			}
			if strings.HasPrefix(path[0], "/") || strings.Contains(path[0], "../") || path[0] == ".." {
				return 0, fmt.Errorf("patch contains unsafe path %q", path[0])
			}
			if strings.HasPrefix(path[0], "a/") || strings.HasPrefix(path[0], "b/") {
				strip = 1
			}
		}
	}
	return strip, nil
}
