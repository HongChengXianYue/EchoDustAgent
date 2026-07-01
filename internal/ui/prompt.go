package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	promptBoxBG         = "\x1b[48;5;238m"
	promptBoxFG         = "\x1b[38;5;255m"
	promptBoxAccentFG   = "\x1b[38;5;252m"
	promptBoxMutedFG    = "\x1b[38;5;246m"
	promptBoxReset      = "\x1b[0m"
	promptBoxLeftPad    = 1
	promptBoxRightPad   = 2
	promptPlaceholder   = `Try "create a util logging.py that..."`
	promptPromptSpacing = 1

	// 命令建议列表的颜色和布局。
	suggestNameFG   = "\x1b[38;5;117m"
	suggestDescFG   = "\x1b[38;5;248m"
	suggestReset    = "\x1b[0m"
	suggestNameWidth = 14 // 命令名列固定宽度（含 / 和前导空格）
)

// CommandSuggestion 是一个 /命令 的名称和描述，用于输入框下方的建议列表。
type CommandSuggestion struct {
	Name string // 不含 /，如 "model"
	Desc string // 简短描述
}

type Prompt struct {
	input           io.Reader
	output          io.Writer
	reader          *bufio.Reader
	history         []string
	promptRows      int
	promptCursorUp  int
	commands        []CommandSuggestion  // /命令列表，为空时不显示建议
	suggestRows     int                  // 上次渲染的建议列表行数（用于清除）
	suggestMatched  []CommandSuggestion  // 当前匹配的列表（上下键选择用）
	suggestSelected int                  // 当前选中的命令索引
}

func NewPrompt(input io.Reader, output io.Writer) *Prompt {
	return &Prompt{
		input:  input,
		output: output,
		reader: bufio.NewReader(input),
	}
}

// SetCommands 设置 /命令 列表，供输入框在用户输入 / 时显示建议。
// 传 nil 或空切片会禁用建议列表。
func (p *Prompt) SetCommands(cmds []CommandSuggestion) {
	p.commands = cmds
}

// applyTabCompletion 在输入以 / 开头时，补全第一个前缀匹配的命令名。
// 如果前缀含空格（用户在输参数）或无匹配，不做任何事。
func (p *Prompt) applyTabCompletion(state *lineState) {
	input := string(state.runes)
	if !strings.HasPrefix(input, "/") {
		return
	}
	prefix := strings.TrimPrefix(input, "/")
	// 前缀含空格说明用户在输入参数（如 "/model qwen"），不补全命令名。
	if strings.Contains(prefix, " ") {
		return
	}
	for _, cmd := range p.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			state.runes = []rune("/" + cmd.Name)
			state.cursor = len(state.runes)
			return
		}
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
	p.renderFrame(prompt, state.runes, state.cursor)

	for {
		key, err := readKey(p.reader)
		if err != nil {
			raw.restore()
			fmt.Fprintln(p.output)
			return "", false
		}
		// 建议列表可见时，上下键移动选择，回车执行选中命令。
		if p.suggestRows > 0 {
			switch key {
			case "up":
				if p.suggestSelected > 0 {
					p.suggestSelected--
					p.renderFrame(prompt, state.runes, state.cursor)
				}
				continue
			case "down":
				if p.suggestSelected < len(p.suggestMatched)-1 {
					p.suggestSelected++
					p.renderFrame(prompt, state.runes, state.cursor)
				}
				continue
			case "enter":
				// 执行选中的命令，直接返回。
				selected := "/" + p.suggestMatched[p.suggestSelected].Name
				p.clearPrompt()
				raw.restore()
				p.addHistory(selected)
				return selected, true
			}
		}
		// Tab 补全：输入以 / 开头时，补全第一个前缀匹配的命令。
		if key == "tab" {
			p.applyTabCompletion(state)
			p.renderFrame(prompt, state.runes, state.cursor)
			continue
		}
		if line, done, ok := state.applyKey(key); done {
			raw.restore()
			// The renderer echoes submitted prompts as part of the run transcript.
			// Clear the editable input row so live-frame redraws do not depend on
			// terminal line-editor leftovers.
			p.clearPrompt()
			if ok {
				p.addHistory(line)
			}
			return line, ok
		}
		p.renderFrame(prompt, state.runes, state.cursor)
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
		if strings.HasPrefix(key, "paste:") {
			s.insertRunes([]rune(strings.TrimPrefix(key, "paste:")))
		} else {
			s.insertRunes([]rune(key))
		}
	}
	return "", false, true
}

func (s *lineState) insertRunes(input []rune) {
	if len(input) == 0 {
		return
	}
	next := make([]rune, 0, len(s.runes)+len(input))
	next = append(next, s.runes[:s.cursor]...)
	next = append(next, input...)
	next = append(next, s.runes[s.cursor:]...)
	s.runes = next
	s.cursor += len(input)
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

func (p *Prompt) renderFrame(promptStr string, runes []rune, cursor int) {
	p.clearPrompt()
	rows, cursorUp := renderPromptLine(p.output, promptStr, runes, cursor)
	p.promptRows = rows
	p.promptCursorUp = cursorUp
	p.renderCommandSuggestions(string(runes))
}

func (p *Prompt) renderPromptLine(prompt string, runes []rune, cursor int) {
	p.clearPrompt()
	rows, cursorUp := renderPromptLine(p.output, prompt, runes, cursor)
	p.promptRows = rows
	p.promptCursorUp = cursorUp
}

func (p *Prompt) clearPrompt() {
	// 先清除命令建议列表（渲染在输入行下方）。
	if p.suggestRows > 0 {
		// 光标当前在建议列表最后一行之后，上移到列表顶部。
		fmt.Fprintf(p.output, "\x1b[%dA", p.suggestRows)
		for i := 0; i < p.suggestRows; i++ {
			fmt.Fprint(p.output, "\r\x1b[2K")
			if i < p.suggestRows-1 {
				fmt.Fprint(p.output, "\n")
			}
		}
		// 清完后光标在列表最后一行，上移回列表顶部（输入行下方）。
		if p.suggestRows > 1 {
			fmt.Fprintf(p.output, "\x1b[%dA", p.suggestRows-1)
		}
		p.suggestRows = 0
	}
	// 再清除输入行。
	if p.promptRows <= 0 {
		fmt.Fprint(p.output, "\r\x1b[2K")
		return
	}
	if p.promptCursorUp > 0 {
		fmt.Fprintf(p.output, "\x1b[%dA", p.promptCursorUp)
	}
	for i := 0; i < p.promptRows; i++ {
		fmt.Fprint(p.output, "\r\x1b[2K")
		if i < p.promptRows-1 {
			fmt.Fprint(p.output, "\x1b[1B")
		}
	}
	if p.promptRows > 1 {
		fmt.Fprintf(p.output, "\x1b[%dA", p.promptRows-1)
	}
}

func renderPromptLine(output io.Writer, prompt string, runes []rune, cursor int) (int, int) {
	lines := promptDisplayLines(output, prompt, runes, cursor)
	cursorRow := 0
	for i, line := range lines {
		if line.HasCursor {
			cursorRow = i
		}
		renderPromptRow(output, prompt, line, i == 0, len(runes) == 0)
		if i < len(lines)-1 {
			// 用 \r\n 而非 \n，确保续行从行首开始，避免从光标列换行导致错位。
			fmt.Fprint(output, "\r\n")
		}
	}
	cursorUp := len(lines) - 1 - cursorRow
	if cursorUp > 0 {
		fmt.Fprintf(output, "\x1b[%dA", cursorUp)
	}
	return len(lines), cursorUp
}

type promptDisplayLine struct {
	Runes     []rune
	Cursor    int
	HasCursor bool
}

// promptDisplayLines 分两层把用户输入拆成屏幕行：
//  1. 按 '\n' 拆逻辑行（用户显式换行 / 粘贴的多行文本）。
//  2. 对每个逻辑行按终端可用宽度（maxWidth）折行成多个显示行，
//     光标只落在包含它的那一个显示行上。
//
// 这样当用户输入超过终端宽度时，剩余字符会自动折到下一行显示，
// 而不是被裁剪掉。
func promptDisplayLines(output io.Writer, prompt string, runes []rune, cursor int) []promptDisplayLine {
	if len(runes) == 0 {
		return []promptDisplayLine{{HasCursor: true}}
	}
	cursor = clamp(cursor, 0, len(runes))
	maxWidth := promptLineInputWidth(output, prompt)
	if maxWidth <= 0 {
		maxWidth = 1
	}

	type logicalLine struct {
		runes     []rune
		cursor    int
		hasCursor bool
	}
	var logical []logicalLine
	start := 0
	for i, r := range runes {
		if r != '\n' {
			continue
		}
		logical = append(logical, logicalLine{
			runes:     runes[start:i],
			cursor:    clamp(cursor-start, 0, i-start),
			hasCursor: cursor >= start && cursor <= i,
		})
		start = i + 1
	}
	logical = append(logical, logicalLine{
		runes:     runes[start:],
		cursor:    clamp(cursor-start, 0, len(runes)-start),
		hasCursor: cursor >= start,
	})

	var result []promptDisplayLine
	for _, ll := range logical {
		result = append(result, wrapLogicalLine(ll.runes, ll.cursor, ll.hasCursor, maxWidth)...)
	}
	return result
}

// wrapLogicalLine 把一个逻辑行按 maxWidth（cell width）折成多个显示行。
// cursor 是该行内相对于 runes 的光标位置（0..len(runes)），hasCursor 表示
// 该行是否包含全局光标。当光标刚好落在 wrap 边界时，让它出现在下一行的
// 行首，与 readline 类 UI 的直觉一致。
func wrapLogicalLine(runes []rune, cursor int, hasCursor bool, maxWidth int) []promptDisplayLine {
	if len(runes) == 0 {
		return []promptDisplayLine{{Cursor: cursor, HasCursor: hasCursor}}
	}
	var result []promptDisplayLine
	pos := 0
	for pos < len(runes) {
		end := pos
		used := 0
		for end < len(runes) {
			w := runeCellWidth(runes[end])
			if used+w > maxWidth {
				break
			}
			end++
			used += w
		}
		if end == pos {
			// 单个 rune 就超宽的极端情况（理论上 runeCellWidth 不会返回
			// 大于 maxWidth 的值，但保险起见至少前进一格避免死循环）。
			end = pos + 1
		}
		// 光标刚好落在 [pos, end) 内才属于当前显示行。若光标正好 == end
		// 且后面还有内容，则让它落到下一个 wrap 行的行首。
		lineHasCursor := hasCursor && cursor >= pos && cursor < end
		lineCursor := 0
		if lineHasCursor {
			lineCursor = cursor - pos
		}
		result = append(result, promptDisplayLine{
			Runes:     runes[pos:end],
			Cursor:    lineCursor,
			HasCursor: lineHasCursor,
		})
		pos = end
	}
	// 光标在该逻辑行末尾（== len(runes)）时，落在最后一个 wrap 行的末尾。
	if hasCursor && cursor == len(runes) {
		if len(result) == 0 {
			result = append(result, promptDisplayLine{HasCursor: true})
		} else {
			last := &result[len(result)-1]
			last.HasCursor = true
			last.Cursor = len(last.Runes)
		}
	}
	// 兜底：如果因为边界判断没有任何行拿到光标，放到首行行首，避免
	// 光标丢失。
	if hasCursor {
		any := false
		for _, r := range result {
			if r.HasCursor {
				any = true
				break
			}
		}
		if !any && len(result) > 0 {
			result[0].HasCursor = true
			result[0].Cursor = 0
		}
	}
	return result
}

func renderPromptRow(output io.Writer, prompt string, line promptDisplayLine, showPrompt bool, placeholder bool) {
	promptText := prompt
	if !showPrompt {
		// 续行用等宽空格对齐 prompt，使每行可用宽度一致。
		promptText = strings.Repeat(" ", displayWidth([]rune(prompt)))
	}
	display := string(line.Runes)
	style := promptBoxFG
	var cursorBack int
	if placeholder {
		// 空输入时显示 placeholder，光标回到 placeholder 末尾。
		display = promptPlaceholder
		style = promptBoxMutedFG
		cursorBack = displayWidth([]rune(display))
	} else if line.HasCursor {
		// 光标回退量 = 光标后内容的 cell 宽度（line.Runes 已是折行后的片段）。
		cursorBack = displayWidth(line.Runes[line.Cursor:])
	}
	fill := promptLineFillWidth(output, promptText, display)
	leftPad := strings.Repeat(" ", promptBoxLeftPad)
	rightPad := strings.Repeat(" ", promptBoxRightPad)
	gap := strings.Repeat(" ", promptPromptSpacing)
	fmt.Fprintf(output, "\r\x1b[2K%s%s%s%s%s%s%s%s%s", promptBoxBG, leftPad, promptBoxAccentFG, promptText, gap, style, display, strings.Repeat(" ", fill)+rightPad, promptBoxReset)
	fmt.Fprint(output, promptBoxReset)
	if line.HasCursor && cursorBack+fill+promptBoxRightPad > 0 {
		fmt.Fprintf(output, "\x1b[%dD", cursorBack+fill+promptBoxRightPad)
	}
}

func clamp(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func promptLineInputWidth(output io.Writer, prompt string) int {
	file, ok := output.(*os.File)
	if !ok || !isTerminal(file) {
		return 80
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return 80
	}
	available := width - promptBoxLeftPad - displayWidth([]rune(prompt)) - promptPromptSpacing - promptBoxRightPad - 1
	if available < 1 {
		return 1
	}
	return available
}

func promptLineFillWidth(output io.Writer, prompt string, display string) int {
	file, ok := output.(*os.File)
	if !ok || !isTerminal(file) {
		return 1
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return 1
	}
	used := promptBoxLeftPad + displayWidth([]rune(prompt)) + promptPromptSpacing + displayWidth([]rune(display)) + promptBoxRightPad
	fill := width - used - 1
	if fill < 1 {
		return 1
	}
	return fill
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

// renderCommandSuggestions 在输入行下方渲染 /命令 建议列表。
// 触发条件：输入以 / 开头、commands 非空、前缀不含空格（用户在输命令名而非参数）。
// 渲染格式：每行 "  /命令名<padding>描述"，命令名固定宽度左对齐。
func (p *Prompt) renderCommandSuggestions(input string) {
	p.suggestRows = 0
	if !strings.HasPrefix(input, "/") || len(p.commands) == 0 {
		p.suggestMatched = nil
		return
	}
	prefix := strings.TrimPrefix(input, "/")
	// 前缀含空格说明用户在输入参数（如 "/model qwen"），不再显示建议。
	if strings.Contains(prefix, " ") {
		p.suggestMatched = nil
		return
	}

	var matched []CommandSuggestion
	for _, cmd := range p.commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matched = append(matched, cmd)
		}
	}
	if len(matched) == 0 {
		p.suggestMatched = nil
		return
	}

	// 保存匹配列表，clamp 选中索引到合法范围。
	p.suggestMatched = matched
	if p.suggestSelected >= len(matched) {
		p.suggestSelected = len(matched) - 1
	}
	if p.suggestSelected < 0 {
		p.suggestSelected = 0
	}

	// 先回到行首，再换行到输入行下方，避免从光标列开始新行导致错位。
	fmt.Fprint(p.output, "\r\n")
	for i, cmd := range matched {
		// 选中行用 "> " 标记，其他行用 "  "。
		marker := "  "
		if i == p.suggestSelected {
			marker = "\x1b[38;5;117m>\x1b[0m "
		}
		name := "/" + cmd.Name
		nameWidth := displayWidth([]rune(name))
		// 命令名 + 前导 2 空格（或 marker），总宽 suggestNameWidth；不足补空格。
		namePad := suggestNameWidth - 2 - nameWidth
		if namePad < 1 {
			namePad = 1
		}
		// 每行末尾用 \r\n 而非 \n，确保下一行从行首开始。
		fmt.Fprintf(p.output, "%s%s%s%s%s%s%s\r\n",
			marker,
			suggestNameFG, name, suggestReset,
			strings.Repeat(" ", namePad),
			suggestDescFG, cmd.Desc+suggestReset)
	}
	p.suggestRows = len(matched) + 1 // +1 为前面的空行分隔
}
