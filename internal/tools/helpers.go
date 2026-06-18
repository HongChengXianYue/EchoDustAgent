package tools

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

func runCommandOutput(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = combineEnv(os.Environ(), env...)
	}
	output, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(output)), err
}

func combineEnv(base []string, extra ...string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	out = append(out, extra...)
	return out
}
