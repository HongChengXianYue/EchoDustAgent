package ui

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

func readKey(reader *bufio.Reader) (string, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	switch b {
	case '\r', '\n':
		return "enter", nil
	case 3:
		return "ctrl_c", nil
	case 8, 127:
		return "backspace", nil
	case 27:
		next, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		if next != '[' {
			return "", nil
		}
		arrow, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch arrow {
		case 'A':
			return "up", nil
		case 'B':
			return "down", nil
		case 'C':
			return "right", nil
		case 'D':
			return "left", nil
		}
	}

	if b < utf8.RuneSelf {
		return string(rune(b)), nil
	}

	needed := utf8SequenceLength(b)
	if needed == 0 {
		return "", nil
	}
	buf := []byte{b}
	for len(buf) < needed {
		next, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		buf = append(buf, next)
	}
	r, _ := utf8.DecodeRune(buf)
	if r == utf8.RuneError {
		return "", nil
	}
	return string(r), nil
}

func utf8SequenceLength(first byte) int {
	switch {
	case first&0xE0 == 0xC0:
		return 2
	case first&0xF0 == 0xE0:
		return 3
	case first&0xF8 == 0xF0:
		return 4
	default:
		return 0
	}
}

type rawModeState struct {
	restoreFunc func()
	enabled     bool
}

func enterRawMode(input io.Reader) *rawModeState {
	state := &rawModeState{}
	file, ok := input.(*os.File)
	if !ok || !isTerminal(file) {
		return state
	}
	restore, err := enableRawMode(file)
	if err != nil {
		return state
	}
	state.restoreFunc = restore
	state.enabled = true
	return state
}

func (s *rawModeState) restore() {
	if s == nil || !s.enabled {
		return
	}
	s.restoreFunc()
	s.enabled = false
}

func isTerminal(file *os.File) bool {
	stat, err := file.Stat()
	return err == nil && (stat.Mode()&os.ModeCharDevice) != 0
}

func enableRawMode(file *os.File) (func(), error) {
	stateCmd := exec.Command("stty", "-g")
	stateCmd.Stdin = file
	stateBytes, err := stateCmd.Output()
	if err != nil {
		return nil, err
	}

	rawCmd := exec.Command("stty", "raw", "-echo")
	rawCmd.Stdin = file
	if err := rawCmd.Run(); err != nil {
		return nil, err
	}

	state := strings.TrimSpace(string(stateBytes))
	return func() {
		restoreCmd := exec.Command("stty", state)
		restoreCmd.Stdin = file
		_ = restoreCmd.Run()
	}, nil
}
