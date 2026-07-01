package ui

import (
	"bytes"
	"strings"
	"testing"
)

// 启动时（非终端）只渲染标题，不再堆详情和命令提示。
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
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("startup output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, startupBannerLines[0]) {
		t.Fatalf("non-terminal output should not render wide banner:\n%s", text)
	}
	for _, unwanted := range []string{"workdir:", "model:", "log file:", "type /info"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("startup output should not include details or hint (got %q):\n%s", unwanted, text)
		}
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

func TestStartupDetailsIncludesAllFields(t *testing.T) {
	var out bytes.Buffer
	RenderStartupDetails(&out, StartupInfo{
		Workdir:    "/tmp/project",
		Model:      "qwen3.7-plus",
		WireAPI:    "responses",
		MCPEnabled: true,
		MCPTools:   9,
		LogFile:    "/tmp/log/agent.log",
	})

	text := out.String()
	for _, want := range []string{"workdir:", "/tmp/project", "model:", "qwen3.7-plus", "wire api:", "responses", "mcp tools:", "9", "log file:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("details missing %q:\n%s", want, text)
		}
	}
}

func TestStartupDetailsOmitsMCPToolsWhenDisabled(t *testing.T) {
	var out bytes.Buffer
	RenderStartupDetails(&out, StartupInfo{
		Workdir: "/tmp/project",
		Model:   "test-model",
		WireAPI: "responses",
		LogFile: "/tmp/log",
	})
	text := out.String()
	if strings.Contains(text, "mcp tools:") {
		t.Fatalf("MCP line should be hidden when MCP is disabled:\n%s", text)
	}
}

// 宽模式启动只渲染居中 banner，不渲染详情和命令提示。
func TestRenderWideStartupRendersBannerOnly(t *testing.T) {
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
	for _, want := range []string{startupBlue, startupLightBlue} {
		if !strings.Contains(text, want) {
			t.Fatalf("wide startup output missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"workdir:", "model:", "log file:", "type /info"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("wide startup should not include details or hint (got %q):\n%s", unwanted, text)
		}
	}
}
