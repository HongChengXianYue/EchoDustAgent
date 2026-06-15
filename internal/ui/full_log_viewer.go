package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type fullLogViewer struct {
	input             *os.File
	output            io.Writer
	lines             []string
	textProvider      func(fullLogState) string
	mainExpanded      bool
	expandedSubagents map[int]bool
	width             int
	height            int
	offset            int
	pollInterval      time.Duration
}

type fullLogState struct {
	MainExpanded      bool
	ExpandedSubagents map[int]bool
}

func (s fullLogState) SubagentExpanded(index int) bool {
	return s.ExpandedSubagents[index]
}

func newFullLogViewer(input *os.File, output *os.File, text string, options Options) *fullLogViewer {
	return newLiveFullLogViewer(input, output, func() string { return text }, options)
}

func newLiveFullLogViewer(input *os.File, output *os.File, textProvider func() string, options Options) *fullLogViewer {
	return newStatefulLiveFullLogViewer(input, output, func(fullLogState) string { return textProvider() }, options)
}

func newStatefulLiveFullLogViewer(input *os.File, output *os.File, textProvider func(fullLogState) string, options Options) *fullLogViewer {
	options = normalizeOptions(options)
	width, height, err := term.GetSize(int(output.Fd()))
	if err != nil {
		width = options.FullLogDefaultWidth
		height = options.FullLogDefaultHeight
	}
	if width < options.FullLogMinWidth {
		width = options.FullLogMinWidth
	}
	if height < options.FullLogMinHeight {
		height = options.FullLogMinHeight
	}

	return &fullLogViewer{
		input:             input,
		output:            output,
		textProvider:      textProvider,
		mainExpanded:      true,
		expandedSubagents: map[int]bool{},
		width:             width,
		height:            height,
		pollInterval:      time.Duration(options.FullLogPollMilliseconds) * time.Millisecond,
	}
}

func (v *fullLogViewer) Run() {
	if v == nil || v.input == nil || v.output == nil {
		return
	}

	fmt.Fprint(v.output, "\x1b[?1049h\x1b[H\x1b[2J")
	defer fmt.Fprint(v.output, "\x1b[?1049l")

	v.refreshLines()
	v.render()
	var buf [32]byte
	for {
		n, err := syscall.Read(int(v.input.Fd()), buf[:])
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(v.pollInterval)
				if v.refreshLines() {
					v.render()
				}
				continue
			}
			return
		}
		if v.handleInput(buf[:n]) {
			return
		}
		v.render()
	}
}

func (v *fullLogViewer) refreshLines() bool {
	if v.textProvider == nil {
		return false
	}
	next := wrapFullLogLines(v.textProvider(v.state()), v.width)
	if equalStringSlices(v.lines, next) {
		return false
	}
	v.lines = next
	if v.offset > v.maxOffset() {
		v.offset = v.maxOffset()
	}
	return true
}

func (v *fullLogViewer) handleInput(input []byte) bool {
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case 'q', 'Q', 20:
			return true
		case 3:
			if process, err := os.FindProcess(os.Getpid()); err == nil {
				_ = process.Signal(os.Interrupt)
			}
			return true
		case 27:
			if i+2 < len(input) && input[i+1] == '[' {
				consumed := v.handleEscape(input[i+2:])
				if consumed > 0 {
					i += consumed + 1
					continue
				}
			}
			return true
		case 'j':
			v.scroll(1)
		case 'k':
			v.scroll(-1)
		case 'g':
			v.offset = 0
		case 'G':
			v.offset = v.maxOffset()
		case '0':
			v.toggleMain()
		case '1', '2', '3', '4', '5':
			v.toggleSubagent(int(input[i] - '0'))
		}
	}
	return false
}

func (v *fullLogViewer) handleEscape(input []byte) int {
	if len(input) == 0 {
		return 0
	}
	if consumed := v.handleCSIu(input); consumed > 0 {
		return consumed
	}
	switch input[0] {
	case 'A':
		v.scroll(-1)
		return 1
	case 'B':
		v.scroll(1)
		return 1
	case 'H':
		v.offset = 0
		return 1
	case 'F':
		v.offset = v.maxOffset()
		return 1
	case '5':
		if len(input) > 1 && input[1] == '~' {
			v.scroll(-v.pageSize())
			return 2
		}
	case '6':
		if len(input) > 1 && input[1] == '~' {
			v.scroll(v.pageSize())
			return 2
		}
	}
	return 0
}

func (v *fullLogViewer) handleCSIu(input []byte) int {
	end := -1
	for i, b := range input {
		if b == 'u' {
			end = i
			break
		}
	}
	if end < 0 {
		return 0
	}

	parts := strings.Split(string(input[:end]), ";")
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	modifierText := strings.SplitN(parts[1], ":", 2)[0]
	modifier, err := strconv.Atoi(modifierText)
	if err != nil {
		return 0
	}
	if modifier == 5 && code >= int('0') && code <= int('5') {
		index := code - int('0')
		if index == 0 {
			v.toggleMain()
		} else {
			v.toggleSubagent(index)
		}
		return end + 1
	}
	return 0
}

func (v *fullLogViewer) render() {
	fmt.Fprint(v.output, "\x1b[H\x1b[2J")
	pageSize := v.pageSize()
	for i := 0; i < pageSize; i++ {
		index := v.offset + i
		fmt.Fprint(v.output, "\x1b[2K")
		if index < len(v.lines) {
			fmt.Fprint(v.output, v.lines[index])
		}
		fmt.Fprint(v.output, "\r\n")
	}
	fmt.Fprint(v.output, "\x1b[2K\x1b[7m")
	fmt.Fprintf(
		v.output,
		" Full tool log | Ctrl+T/q/Esc close | Ctrl+0 main | Ctrl+1-5 subagents | ↑↓/jk scroll | PgUp/PgDn page | %d-%d/%d ",
		v.offset+1,
		min(v.offset+pageSize, len(v.lines)),
		len(v.lines),
	)
	fmt.Fprint(v.output, "\x1b[0m")
}

func (v *fullLogViewer) pageSize() int {
	if v.height <= 1 {
		return 1
	}
	return v.height - 1
}

func (v *fullLogViewer) scroll(delta int) {
	v.offset += delta
	if v.offset < 0 {
		v.offset = 0
	}
	if maxOffset := v.maxOffset(); v.offset > maxOffset {
		v.offset = maxOffset
	}
}

func (v *fullLogViewer) toggleMain() {
	v.mainExpanded = !v.mainExpanded
	if v.refreshLines() && v.offset > v.maxOffset() {
		v.offset = v.maxOffset()
	}
}

func (v *fullLogViewer) toggleSubagent(index int) {
	if index < 1 || index > 5 {
		return
	}
	v.expandedSubagents[index] = !v.expandedSubagents[index]
	if v.refreshLines() && v.offset > v.maxOffset() {
		v.offset = v.maxOffset()
	}
}

func (v *fullLogViewer) state() fullLogState {
	expanded := make(map[int]bool, len(v.expandedSubagents))
	for index, isExpanded := range v.expandedSubagents {
		expanded[index] = isExpanded
	}
	return fullLogState{MainExpanded: v.mainExpanded, ExpandedSubagents: expanded}
}

func (v *fullLogViewer) maxOffset() int {
	maxOffset := len(v.lines) - v.pageSize()
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func wrapFullLogLines(text string, width int) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{""}
	}

	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, wrapFullLogLine(line, width)...)
	}
	return out
}

func wrapFullLogLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}

	var lines []string
	var current strings.Builder
	currentWidth := 0
	activeANSI := ""
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			sequence, next := readFullLogANSISequence(line, i)
			if next > i {
				current.WriteString(sequence)
				activeANSI = updateFullLogActiveANSI(activeANSI, sequence)
				i = next
				continue
			}
		}
		r, size := decodeFullLogRune(line[i:])
		cellWidth := runeCellWidth(r)
		if currentWidth > 0 && currentWidth+cellWidth > width {
			lines = append(lines, resetFullLogSegment(current.String(), activeANSI))
			current.Reset()
			currentWidth = 0
			if activeANSI != "" {
				current.WriteString(activeANSI)
			}
		}
		current.WriteRune(r)
		currentWidth += cellWidth
		i += size
	}
	lines = append(lines, current.String())
	return lines
}

func readFullLogANSISequence(text string, start int) (string, int) {
	if start+1 >= len(text) || text[start+1] != '[' {
		return "", start
	}
	for i := start + 2; i < len(text); i++ {
		b := text[i]
		if b >= 0x40 && b <= 0x7e {
			return text[start : i+1], i + 1
		}
	}
	return "", start
}

func updateFullLogActiveANSI(active string, sequence string) string {
	if !strings.HasSuffix(sequence, "m") {
		return active
	}
	if sequence == "\x1b[0m" {
		return ""
	}
	return sequence
}

func resetFullLogSegment(segment string, activeANSI string) string {
	if activeANSI == "" || strings.HasSuffix(segment, "\x1b[0m") {
		return segment
	}
	return segment + "\x1b[0m"
}

func decodeFullLogRune(text string) (rune, int) {
	r, size := utf8.DecodeRuneInString(text)
	if size <= 0 {
		return rune(text[0]), 1
	}
	return r, size
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
