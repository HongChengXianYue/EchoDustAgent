package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Prompt struct {
	input   io.Reader
	output  io.Writer
	reader  *bufio.Reader
	history []string
}

func NewPrompt(input io.Reader, output io.Writer) *Prompt {
	return &Prompt{
		input:  input,
		output: output,
		reader: bufio.NewReader(input),
	}
}

func (p *Prompt) ReadLine(prompt string) (string, bool) {
	raw := enterRawMode(p.input)
	defer raw.restore()

	if !raw.enabled {
		fmt.Fprint(p.output, prompt)
		line, err := p.reader.ReadString('\n')
		if err != nil && line == "" {
			return "", false
		}
		line = strings.TrimRight(line, "\r\n")
		p.addHistory(line)
		return line, true
	}

	state := newLineState(p.history)
	fmt.Fprint(p.output, prompt)
	renderPromptLine(p.output, prompt, state.runes, state.cursor)

	for {
		key, err := readKey(p.reader)
		if err != nil {
			raw.restore()
			fmt.Fprintln(p.output)
			return "", false
		}
		if line, done, ok := state.applyKey(key); done {
			raw.restore()
			// The renderer echoes submitted prompts as part of the run transcript.
			// Clear the editable input row so live-frame redraws do not depend on
			// terminal line-editor leftovers.
			fmt.Fprint(p.output, "\r\x1b[2K")
			if ok {
				p.addHistory(line)
			}
			return line, ok
		}
		renderPromptLine(p.output, prompt, state.runes, state.cursor)
	}
}

func (p *Prompt) addHistory(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if len(p.history) > 0 && p.history[len(p.history)-1] == line {
		return
	}
	p.history = append(p.history, line)
}

type lineState struct {
	runes      []rune
	cursor     int
	history    []string
	historyPos int
	draft      []rune
}

func newLineState(history []string) *lineState {
	return &lineState{history: append([]string(nil), history...), historyPos: len(history)}
}

func (s *lineState) applyKey(key string) (string, bool, bool) {
	switch key {
	case "enter":
		return string(s.runes), true, true
	case "ctrl_c":
		return "", true, false
	case "ctrl_e", "ctrl_t":
		return "", false, true
	case "left":
		if s.cursor > 0 {
			s.cursor--
		}
	case "right":
		if s.cursor < len(s.runes) {
			s.cursor++
		}
	case "backspace":
		if s.cursor > 0 {
			s.runes = append(s.runes[:s.cursor-1], s.runes[s.cursor:]...)
			s.cursor--
		}
	case "up":
		s.historyUp()
	case "down":
		s.historyDown()
	case "":
		return "", false, true
	default:
		for _, r := range key {
			s.runes = append(s.runes[:s.cursor], append([]rune{r}, s.runes[s.cursor:]...)...)
			s.cursor++
		}
	}
	return "", false, true
}

func (s *lineState) historyUp() {
	if len(s.history) == 0 || s.historyPos == 0 {
		return
	}
	if s.historyPos == len(s.history) {
		s.draft = append([]rune(nil), s.runes...)
	}
	s.historyPos--
	s.runes = []rune(s.history[s.historyPos])
	s.cursor = len(s.runes)
}

func (s *lineState) historyDown() {
	if len(s.history) == 0 || s.historyPos >= len(s.history) {
		return
	}
	s.historyPos++
	if s.historyPos == len(s.history) {
		s.runes = append([]rune(nil), s.draft...)
	} else {
		s.runes = []rune(s.history[s.historyPos])
	}
	s.cursor = len(s.runes)
}

func renderPromptLine(output io.Writer, prompt string, runes []rune, cursor int) {
	line := string(runes)
	right := displayWidth(runes[cursor:])
	fmt.Fprintf(output, "\r\x1b[2K%s%s", prompt, line)
	if right > 0 {
		fmt.Fprintf(output, "\x1b[%dD", right)
	}
}

func displayWidth(runes []rune) int {
	width := 0
	for _, r := range runes {
		width += runeCellWidth(r)
	}
	return width
}

func runeCellWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case r >= 0x1100 && (r <= 0x115f ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6)):
		return 2
	default:
		return 1
	}
}
