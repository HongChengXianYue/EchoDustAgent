package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	fullLogDefaultWidth  = 100
	fullLogDefaultHeight = 24
)

type fullLogViewer struct {
	input  *os.File
	output io.Writer
	lines  []string
	width  int
	height int
	offset int
}

func newFullLogViewer(input *os.File, output *os.File, text string) *fullLogViewer {
	width, height, err := term.GetSize(int(output.Fd()))
	if err != nil {
		width = fullLogDefaultWidth
		height = fullLogDefaultHeight
	}
	if width < 20 {
		width = 20
	}
	if height < 6 {
		height = 6
	}

	return &fullLogViewer{
		input:  input,
		output: output,
		lines:  wrapFullLogLines(text, width),
		width:  width,
		height: height,
	}
}

func (v *fullLogViewer) Run() {
	if v == nil || v.input == nil || v.output == nil {
		return
	}

	fmt.Fprint(v.output, "\x1b[?1049h\x1b[H\x1b[2J")
	defer fmt.Fprint(v.output, "\x1b[?1049l")

	v.render()
	var buf [32]byte
	for {
		n, err := syscall.Read(int(v.input.Fd()), buf[:])
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(30 * time.Millisecond)
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
		}
	}
	return false
}

func (v *fullLogViewer) handleEscape(input []byte) int {
	if len(input) == 0 {
		return 0
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
		" Full tool log | Ctrl+T/q/Esc close | ↑↓/jk scroll | PgUp/PgDn page | %d-%d/%d ",
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
	for _, r := range line {
		cellWidth := runeCellWidth(r)
		if currentWidth > 0 && currentWidth+cellWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteRune(r)
		currentWidth += cellWidth
	}
	lines = append(lines, current.String())
	return lines
}
