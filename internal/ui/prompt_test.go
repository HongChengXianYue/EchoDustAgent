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
