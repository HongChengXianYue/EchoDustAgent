package approval

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"
)

type TerminalApprover struct {
	input  io.Reader
	output io.Writer
	reader *bufio.Reader
}

func NewTerminalApprover(input io.Reader, output io.Writer) *TerminalApprover {
	return &TerminalApprover{
		input:  input,
		output: output,
		reader: bufio.NewReader(input),
	}
}

func (a *TerminalApprover) Approve(ctx context.Context, request Request) Decision {
	if !isInteractiveInput(a.input) {
		return DecisionDeny
	}
	return chooseApproval(a.reader, a.input, a.output, request)
}

func chooseApproval(reader *bufio.Reader, input io.Reader, output io.Writer, request Request) Decision {
	raw := enterRawMode(input)
	defer raw.restore()

	decisions := DecisionOptions(request)
	options := approvalOptionLabels(request, decisions)
	selected := 0
	renderApprovalOptions(output, options, selected, false)

	for {
		key, err := approvalReadKey(reader)
		if err != nil {
			raw.restore()
			fmt.Fprintln(output)
			return DecisionDeny
		}

		switch key {
		case "up", "left", "k", "K":
			selected = (selected - 1 + len(options)) % len(options)
			renderApprovalOptions(output, options, selected, true)
		case "down", "right", "j", "J", "\t":
			selected = (selected + 1) % len(options)
			renderApprovalOptions(output, options, selected, true)
		case "a", "A", "y", "Y":
			if optionIndex(decisions, DecisionAllow) >= 0 {
				raw.restore()
				fmt.Fprintln(output)
				return DecisionAllow
			}
			if optionIndex(decisions, DecisionAlways) >= 0 {
				raw.restore()
				fmt.Fprintln(output)
				return DecisionAlways
			}
		case "d", "D", "n", "N", "ctrl_c":
			raw.restore()
			fmt.Fprintln(output)
			return DecisionDeny
		case "enter":
			raw.restore()
			fmt.Fprintln(output)
			return decisions[selected]
		}
	}
}

func approvalOptionLabels(request Request, decisions []Decision) []string {
	labels := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		switch decision {
		case DecisionAllow:
			labels = append(labels, "Allow once")
		case DecisionAlways:
			if request.Scope == ScopeSession && request.Key == "workspace_write" {
				labels = append(labels, "Always allow workspace writes this session")
			} else if request.Scope == ScopeLoop {
				labels = append(labels, "Always allow this loop")
			} else {
				labels = append(labels, "Always allow exact call")
			}
		case DecisionDeny:
			labels = append(labels, "Deny")
		}
	}
	return labels
}

func optionIndex(decisions []Decision, want Decision) int {
	for i, decision := range decisions {
		if decision == want {
			return i
		}
	}
	return -1
}

func renderApprovalOptions(output io.Writer, options []string, selected int, redraw bool) {
	if redraw {
		fmt.Fprintf(output, "\x1b[%dA", len(options))
	}
	for i, option := range options {
		fmt.Fprint(output, "\x1b[2K\r")
		if i == selected {
			fmt.Fprintf(output, "  › %s\n", option)
		} else {
			fmt.Fprintf(output, "    %s\n", option)
		}
	}
}

func isInteractiveInput(input io.Reader) bool {
	file, ok := input.(*os.File)
	if !ok {
		return false
	}
	stat, err := file.Stat()
	return err == nil && (stat.Mode()&os.ModeCharDevice) != 0
}

func approvalReadKey(reader *bufio.Reader) (string, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	switch b {
	case '\r', '\n':
		return "enter", nil
	case 3:
		return "ctrl_c", nil
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

	needed := approvalUTF8SequenceLength(b)
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

func approvalUTF8SequenceLength(first byte) int {
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

type approvalRawModeState struct {
	restoreFunc func()
	enabled     bool
}

func enterRawMode(input io.Reader) *approvalRawModeState {
	state := &approvalRawModeState{}
	file, ok := input.(*os.File)
	if !ok || !isInteractiveInput(file) {
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

func (s *approvalRawModeState) restore() {
	if s == nil || !s.enabled {
		return
	}
	s.restoreFunc()
	s.enabled = false
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
