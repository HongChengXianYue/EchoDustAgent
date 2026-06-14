package ui

import (
	"bufio"
	"strings"
	"testing"
)

func TestLineStateEditingAndHistory(t *testing.T) {
	state := newLineState([]string{"first", "second"})
	for _, key := range []string{"h", "e", "l", "l", "o"} {
		state.applyKey(key)
	}
	state.applyKey("left")
	state.applyKey("backspace")
	state.applyKey("y")
	if got := string(state.runes); got != "helyo" {
		t.Fatalf("line = %q, want helyo", got)
	}

	state.applyKey("up")
	if got := string(state.runes); got != "second" {
		t.Fatalf("history up line = %q, want second", got)
	}
	state.applyKey("up")
	if got := string(state.runes); got != "first" {
		t.Fatalf("history up line = %q, want first", got)
	}
	state.applyKey("down")
	if got := string(state.runes); got != "second" {
		t.Fatalf("history down line = %q, want second", got)
	}
	state.applyKey("down")
	if got := string(state.runes); got != "helyo" {
		t.Fatalf("history draft line = %q, want helyo", got)
	}
}

func TestReadKeyRecognizesCtrlE(t *testing.T) {
	key, err := readKey(bufio.NewReader(strings.NewReader("\x05")))
	if err != nil {
		t.Fatalf("readKey() error = %v", err)
	}
	if key != "ctrl_e" {
		t.Fatalf("key = %q, want ctrl_e", key)
	}
}

func TestLineStateIgnoresCtrlE(t *testing.T) {
	state := newLineState(nil)
	state.applyKey("a")
	state.applyKey("ctrl_e")
	if got := string(state.runes); got != "a" {
		t.Fatalf("line = %q, want ctrl_e ignored", got)
	}
}
