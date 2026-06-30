package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderStartupBannerFallsBackToCompactForNonTerminal(t *testing.T) {
	var out bytes.Buffer
	RenderStartupBanner(&out, StartupInfo{
		Workdir: "/tmp/project",
		Model:   "test-model",
		WireAPI: "responses",
		LogFile: "/tmp/project/.local-agent/logs/agent.log",
	})

	text := out.String()
	for _, want := range []string{
		startupBlue,
		startupTitle,
		"workdir:",
		"/tmp/project",
		"model:",
		"test-model",
		"wire api:",
		"responses",
		"log file:",
		startupQuitNotice,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("startup output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, startupBannerLines[0]) {
		t.Fatalf("non-terminal output should not render wide banner:\n%s", text)
	}
	if strings.Contains(text, "mcp tools:") {
		t.Fatalf("MCP line should be hidden when MCP is disabled:\n%s", text)
	}
}

func TestStartupDetailsIncludesMCPToolsWhenEnabled(t *testing.T) {
	var out bytes.Buffer
	renderStartupDetails(&out, StartupInfo{
		Workdir:    "/tmp/project",
		Model:      "test-model",
		WireAPI:    "chat_completions",
		MCPEnabled: true,
		MCPTools:   7,
		LogFile:    "/tmp/log",
	}, "")

	text := out.String()
	if !strings.Contains(text, "mcp tools:") || !strings.Contains(text, "7") {
		t.Fatalf("MCP tools line missing:\n%s", text)
	}
}

func TestRenderWideStartupIncludesBannerAndDetails(t *testing.T) {
	var out bytes.Buffer
	renderWideStartup(&out, StartupInfo{
		Workdir: "/tmp/project",
		Model:   "test-model",
		WireAPI: "responses",
		LogFile: "/tmp/log",
	})

	text := out.String()
	if !strings.Contains(text, startupBannerLines[0]) {
		t.Fatalf("wide startup banner missing:\n%s", text)
	}
	for _, want := range []string{startupBlue, startupLightBlue, startupWhite, startupQuitNotice} {
		if !strings.Contains(text, want) {
			t.Fatalf("wide startup output missing %q:\n%s", want, text)
		}
	}
}
