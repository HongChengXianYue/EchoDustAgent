package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

func TestBlockRendererRendersExploreRunEditAndFinal(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "list_files",
		Args: json.RawMessage(`{"path":"."}`),
		Result: &tools.Result{
			Status:  "success",
			Summary: "listed",
			Output:  "go.mod\ncmd/",
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolCall,
		Tool: "run_command",
		Args: json.RawMessage(`{"command":"pwd"}`),
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "run_command",
		Args: json.RawMessage(`{"command":"pwd"}`),
		Result: &tools.Result{
			Status: "success",
			Output: "/tmp/work",
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeToolResult,
		Tool: "write_file",
		Result: &tools.Result{
			Status: "success",
			Changes: []tools.FileChange{
				{Path: "hello.txt", Action: "added", AddedLines: 2, Preview: "    1 +hello"},
			},
		},
	})
	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "done",
	})

	text := out.String()
	for _, want := range []string{
		"• Explored",
		"List .",
		"• Running pwd",
		"• Ran pwd",
		"• Added hello.txt (+2 -0)",
		"done",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererRendersMarkdownFinalText(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type: runtimeevent.TypeFinal,
		Message: `## 最近一次代码改动总结

**提交信息**: ` + "`Add Codex-style CLI UI`" + `

| 文件 | 作用 |
|------|------|
| renderer.go | **块渲染器** |

### 1. 🆕 新增模块

这是一个 **Go CLI Agent**。`,
	})

	text := out.String()
	if strings.Contains(text, "###") || strings.Contains(text, "## ") {
		t.Fatalf("rendered headings should not show markdown heading prefixes:\n%s", text)
	}
	for _, want := range []string{
		"最近一次代码改动总结",
		"提交信息",
		"Add Codex-style CLI UI",
		"文件",
		"作用",
		"renderer.go",
		"块渲染器",
		"新增模块",
		"Go CLI Agent",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBlockRendererRendersCodeBlocksWithoutHeavyChromaHighlighting(t *testing.T) {
	var out bytes.Buffer
	renderer := NewBlockRenderer(&out)

	renderer.HandleEvent(runtimeevent.Event{
		Type:    runtimeevent.TypeFinal,
		Message: "项目结构\n\n```text\ncmd/agent/\ninternal/ui/\n```\n",
	})

	text := out.String()
	for _, want := range []string{"项目结构", "cmd/agent/", "internal/ui/"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[38;5;203m") {
		t.Fatalf("code block should not use heavy red chroma highlighting:\n%q", text)
	}
}
