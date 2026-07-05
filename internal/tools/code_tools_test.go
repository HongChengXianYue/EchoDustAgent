package tools

import "testing"

func TestResolveGoplsCommandUsesEnvOverride(t *testing.T) {
	t.Setenv("ECHODUST_CODE_GOPLS", "/tmp/custom-gopls")
	if got := resolveGoplsCommand(); got != "/tmp/custom-gopls" {
		t.Fatalf("resolveGoplsCommand() = %q, want env override", got)
	}
}
