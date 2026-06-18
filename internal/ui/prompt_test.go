package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderPromptLineShowsPlaceholderBox(t *testing.T) {
	var out bytes.Buffer
	renderPromptLine(&out, "›", nil, 0)
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
	renderPromptLine(&out, "›", []rune("hello"), 5)
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
