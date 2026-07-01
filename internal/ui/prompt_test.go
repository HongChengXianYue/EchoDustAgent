package ui

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestRenderPromptLineShowsPlaceholderBox(t *testing.T) {
	var out bytes.Buffer
	rows, _ := renderPromptLine(&out, "›", nil, 0)
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}
	text := out.String()
	for _, want := range []string{
		promptBoxBG,
		promptBoxMutedFG,
		promptBoxAccentFG,
		"›",
		promptPlaceholder,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("prompt output missing %q: %q", want, text)
		}
	}
	if !strings.Contains(text, "\x1b[") || !strings.Contains(text, "D") {
		t.Fatalf("placeholder prompt should move cursor back to input start: %q", text)
	}
}

func TestRenderPromptLineShowsTypedTextBox(t *testing.T) {
	var out bytes.Buffer
	rows, _ := renderPromptLine(&out, "›", []rune("hello"), 5)
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}
	text := out.String()
	if !strings.Contains(text, promptBoxBG) || !strings.Contains(text, promptBoxFG) {
		t.Fatalf("prompt box colors missing: %q", text)
	}
	if !strings.Contains(text, promptBoxAccentFG) {
		t.Fatalf("prompt accent color missing: %q", text)
	}
	if !strings.Contains(text, "hello") {
		t.Fatalf("prompt text missing: %q", text)
	}
	if strings.Contains(text, promptPlaceholder) {
		t.Fatalf("prompt placeholder should not be shown when text exists: %q", text)
	}
}

func TestRenderPromptLineShowsPastedNewlinesAsRows(t *testing.T) {
	var out bytes.Buffer
	rows, cursorUp := renderPromptLine(&out, "›", []rune("hello\nworld"), len([]rune("hello\nworld")))
	if rows != 2 {
		t.Fatalf("rows = %d, want 2", rows)
	}
	if cursorUp != 0 {
		t.Fatalf("cursorUp = %d, want cursor on last row", cursorUp)
	}
	text := out.String()
	if !strings.Contains(text, "hello") || !strings.Contains(text, "world") {
		t.Fatalf("multiline prompt missing content: %q", text)
	}
	if !strings.Contains(text, "\n") {
		t.Fatalf("multiline prompt should render multiple rows: %q", text)
	}
}

func TestPromptClearsPreviousMultilineRows(t *testing.T) {
	var out bytes.Buffer
	prompt := &Prompt{output: &out}
	prompt.renderPromptLine("›", []rune("hello\nworld"), len([]rune("hello\nworld")))
	out.Reset()
	prompt.renderPromptLine("›", []rune("short"), len([]rune("short")))
	text := out.String()
	if !strings.Contains(text, "\x1b[1A") {
		t.Fatalf("expected clear to move back over previous multiline prompt: %q", text)
	}
	if strings.Contains(text, "world") {
		t.Fatalf("old multiline content leaked after rerender: %q", text)
	}
}

func TestPromptDisplayLinesWrapsLongInputIntoMultipleRows(t *testing.T) {
	// bytes.Buffer 不是 *os.File，promptLineInputWidth 回退到 80。
	// 用明显超过 80 的纯 ASCII 字符串验证自动折行。
	long := []rune(strings.Repeat("a", 200))
	var out bytes.Buffer
	rows, _ := renderPromptLine(&out, "›", long, len(long))
	if rows < 2 {
		t.Fatalf("rows = %d, want >= 2 for 200-char input at width 80", rows)
	}
}

func TestPromptDisplayLinesKeepsExplicitNewlinesAndWrapsEachLogicalLine(t *testing.T) {
	// "aaaa\n" + 200 个 b：逻辑行 1 长度 4（不 wrap），逻辑行 2 长度 200（wrap 成多行）。
	runes := []rune(strings.Repeat("a", 4) + "\n" + strings.Repeat("b", 200))
	var out bytes.Buffer
	rows, _ := renderPromptLine(&out, "›", runes, len(runes))
	if rows < 3 {
		t.Fatalf("rows = %d, want >= 3 (1 short logical + multiple wraps of long line)", rows)
	}
}

func TestPromptCursorStaysOnCorrectWrapRow(t *testing.T) {
	// 100 字符，宽度 80 → 两个 wrap 行：[0,80) 和 [80,100)。
	runes := []rune(strings.Repeat("a", 100))
	var out bytes.Buffer

	// 光标在第二 wrap 行内（90），cursorUp 应为 0（光标在最后一行）。
	rows, cursorUp := renderPromptLine(&out, "›", runes, 90)
	if rows != 2 {
		t.Fatalf("rows = %d, want 2", rows)
	}
	if cursorUp != 0 {
		t.Fatalf("cursorUp = %d, want 0 (cursor on last row)", cursorUp)
	}

	// 光标在第一 wrap 行内（10），cursorUp 应为 1（从最后一行拉回一行）。
	_, cursorUp = renderPromptLine(&out, "›", runes, 10)
	if cursorUp != 1 {
		t.Fatalf("cursorUp = %d, want 1 (cursor on first of two rows)", cursorUp)
	}
}

func TestReadKeyTreatsBracketedPasteAsText(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\x1b[200~hello\nworld\x1b[201~"))
	key, err := readKey(reader)
	if err != nil {
		t.Fatalf("readKey() error = %v", err)
	}
	if key != "paste:hello\nworld" {
		t.Fatalf("key = %q, want pasted text with newline", key)
	}
}

func TestLineStatePasteDoesNotSubmit(t *testing.T) {
	state := newLineState(nil)
	line, done, ok := state.applyKey("paste:hello\nworld")
	if done || !ok || line != "" {
		t.Fatalf("paste should edit without submitting: line=%q done=%v ok=%v", line, done, ok)
	}
	if got := string(state.runes); got != "hello\nworld" {
		t.Fatalf("state = %q, want pasted multiline text", got)
	}

	line, done, ok = state.applyKey("enter")
	if !done || !ok || line != "hello\nworld" {
		t.Fatalf("enter should submit pasted text: line=%q done=%v ok=%v", line, done, ok)
	}
}

func TestRenderCommandSuggestionsShowsMatchedCommands(t *testing.T) {
	var out bytes.Buffer
	p := &Prompt{output: &out, commands: []CommandSuggestion{
		{Name: "info", Desc: "show startup details"},
		{Name: "model", Desc: "show or switch model"},
	}}

	// 输入 "/" → 匹配所有命令。
	p.renderCommandSuggestions("/")
	text := out.String()
	if !strings.Contains(text, "/info") || !strings.Contains(text, "/model") {
		t.Fatalf("expected all commands for prefix '/': %q", text)
	}
	if p.suggestRows != 3 { // 1 空行 + 2 命令
		t.Fatalf("suggestRows = %d, want 3", p.suggestRows)
	}
}

func TestRenderCommandSuggestionsFiltersByPrefix(t *testing.T) {
	var out bytes.Buffer
	p := &Prompt{output: &out, commands: []CommandSuggestion{
		{Name: "info", Desc: "show details"},
		{Name: "model", Desc: "switch model"},
	}}

	// 输入 "/mo" → 只匹配 model。
	out.Reset()
	p.renderCommandSuggestions("/mo")
	text := out.String()
	if !strings.Contains(text, "/model") {
		t.Fatalf("expected /model for prefix '/mo': %q", text)
	}
	if strings.Contains(text, "/info") {
		t.Fatalf("should not contain /info for prefix '/mo': %q", text)
	}
}

func TestRenderCommandSuggestionsHidesWhenPrefixHasSpace(t *testing.T) {
	var out bytes.Buffer
	p := &Prompt{output: &out, commands: []CommandSuggestion{
		{Name: "model", Desc: "switch model"},
	}}

	// 输入 "/model qwen" → 前缀含空格，不显示建议。
	p.renderCommandSuggestions("/model qwen")
	if out.Len() != 0 {
		t.Fatalf("should not render suggestions when prefix has space: %q", out.String())
	}
	if p.suggestRows != 0 {
		t.Fatalf("suggestRows = %d, want 0", p.suggestRows)
	}
}

func TestRenderCommandSuggestionsHidesForNonSlashInput(t *testing.T) {
	var out bytes.Buffer
	p := &Prompt{output: &out, commands: []CommandSuggestion{
		{Name: "info", Desc: "show details"},
	}}

	p.renderCommandSuggestions("hello")
	if out.Len() != 0 {
		t.Fatalf("should not render suggestions for non-slash input: %q", out.String())
	}
}

func TestClearPromptClearsSuggestionRows(t *testing.T) {
	var out bytes.Buffer
	p := &Prompt{
		output:   &out,
		commands: []CommandSuggestion{{Name: "info", Desc: "show details"}},
	}

	// 先渲染一个输入行 + 建议列表。
	p.promptRows = 1
	p.suggestRows = 2
	// 模拟光标位置：在建议列表最后一行之后。
	out.Reset()
	p.clearPrompt()

	text := out.String()
	// 应该包含上移序列（清除建议列表）和清除序列。
	if !strings.Contains(text, "\x1b[2A") {
		t.Fatalf("expected cursor up for suggestRows=2: %q", text)
	}
	if p.suggestRows != 0 {
		t.Fatalf("suggestRows should be reset to 0 after clear: %d", p.suggestRows)
	}
}

func TestApplyTabCompletionCompletesFirstMatch(t *testing.T) {
	p := &Prompt{commands: []CommandSuggestion{
		{Name: "info", Desc: "show details"},
		{Name: "model", Desc: "switch model"},
	}}

	// 输入 "/mo" → 补全成 "/model"。
	state := newLineState(nil)
	state.runes = []rune("/mo")
	state.cursor = len(state.runes)
	p.applyTabCompletion(state)
	if got := string(state.runes); got != "/model" {
		t.Fatalf("runes = %q, want /model", got)
	}
	if state.cursor != len(state.runes) {
		t.Fatalf("cursor = %d, want %d", state.cursor, len(state.runes))
	}
}

func TestApplyTabCompletionNoMatch(t *testing.T) {
	p := &Prompt{commands: []CommandSuggestion{
		{Name: "info", Desc: "show details"},
	}}

	state := newLineState(nil)
	state.runes = []rune("/xyz")
	state.cursor = len(state.runes)
	p.applyTabCompletion(state)
	if got := string(state.runes); got != "/xyz" {
		t.Fatalf("runes should be unchanged when no match: %q", got)
	}
}

func TestApplyTabCompletionIgnoresNonSlashInput(t *testing.T) {
	p := &Prompt{commands: []CommandSuggestion{
		{Name: "info", Desc: "show details"},
	}}

	state := newLineState(nil)
	state.runes = []rune("hello")
	state.cursor = len(state.runes)
	p.applyTabCompletion(state)
	if got := string(state.runes); got != "hello" {
		t.Fatalf("non-slash input should not be completed: %q", got)
	}
}

func TestApplyTabCompletionIgnoresPrefixWithSpace(t *testing.T) {
	p := &Prompt{commands: []CommandSuggestion{
		{Name: "model", Desc: "switch model"},
	}}

	state := newLineState(nil)
	state.runes = []rune("/model qwen")
	state.cursor = len(state.runes)
	p.applyTabCompletion(state)
	if got := string(state.runes); got != "/model qwen" {
		t.Fatalf("prefix with space should not be completed: %q", got)
	}
}
