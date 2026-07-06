package tui

import (
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

const mouseProtocolGuardWindow = 4

type mouseProtocolMatch int

const (
	mouseProtocolNoMatch mouseProtocolMatch = iota
	mouseProtocolPartial
	mouseProtocolFull
)

func (m *Model) armMouseProtocolFilter() {
	m.mouseProtocolGuard = mouseProtocolGuardWindow
}

func (m *Model) resetMouseProtocolFilter() {
	m.mouseProtocolGuard = 0
	m.mouseProtocolPending = ""
}

// Bubble Tea normally parses SGR mouse reports into MouseMsg values, but some
// terminals can occasionally leak the raw "[<...M" tail into the textarea
// during drag selection. Keep a short guard window after mouse activity and
// drop those protocol fragments before they reach the input widget.
func (m *Model) filterMouseProtocolKey(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		if m.mouseProtocolGuard > 0 && m.mouseProtocolPending == "" {
			m.mouseProtocolGuard--
		}
		return msg, false
	}
	if msg.Paste {
		m.resetMouseProtocolFilter()
		return msg, false
	}

	original := string(msg.Runes)
	if m.mouseProtocolGuard <= 0 && m.mouseProtocolPending == "" {
		return msg, false
	}

	combined := m.mouseProtocolPending + original
	filtered, pending, removed := stripMouseProtocolFragments(combined)
	m.mouseProtocolPending = pending

	if removed || pending != "" {
		m.mouseProtocolGuard = mouseProtocolGuardWindow
		if filtered == "" {
			return tea.KeyMsg{}, true
		}
		msg.Runes = []rune(filtered)
		return msg, false
	}

	if combined != original {
		msg.Runes = []rune(filtered)
	}
	if m.mouseProtocolGuard > 0 {
		m.mouseProtocolGuard--
	}
	return msg, false
}

func stripMouseProtocolFragments(input string) (filtered string, pending string, removed bool) {
	if input == "" {
		return "", "", false
	}

	var out strings.Builder
	for len(input) > 0 {
		if consumed, match := matchMouseProtocolPrefix(input); match == mouseProtocolFull {
			removed = true
			input = input[consumed:]
			continue
		} else if match == mouseProtocolPartial {
			return out.String(), input, removed
		}

		r, size := utf8.DecodeRuneInString(input)
		if size == 0 {
			break
		}
		out.WriteRune(r)
		input = input[size:]
	}
	return out.String(), "", removed
}

func matchMouseProtocolPrefix(input string) (int, mouseProtocolMatch) {
	if input == "" {
		return 0, mouseProtocolNoMatch
	}

	index := 0
	switch {
	case input[0] == '[':
		if len(input) == 1 {
			return 0, mouseProtocolPartial
		}
		if input[1] != '<' {
			return 0, mouseProtocolNoMatch
		}
		index = 2
	case input[0] == '\x1b':
		if len(input) == 1 {
			return 0, mouseProtocolPartial
		}
		if input[1] != '[' {
			return 0, mouseProtocolNoMatch
		}
		if len(input) == 2 {
			return 0, mouseProtocolPartial
		}
		if input[2] != '<' {
			return 0, mouseProtocolNoMatch
		}
		index = 3
	default:
		return 0, mouseProtocolNoMatch
	}

	for field := 0; field < 3; field++ {
		start := index
		for index < len(input) && input[index] >= '0' && input[index] <= '9' {
			index++
		}
		if start == index {
			if index == len(input) {
				return 0, mouseProtocolPartial
			}
			return 0, mouseProtocolNoMatch
		}
		if field == 2 {
			break
		}
		if index == len(input) {
			return 0, mouseProtocolPartial
		}
		if input[index] != ';' {
			return 0, mouseProtocolNoMatch
		}
		index++
		if index == len(input) {
			return 0, mouseProtocolPartial
		}
	}

	if index == len(input) {
		return 0, mouseProtocolPartial
	}
	if input[index] != 'M' && input[index] != 'm' {
		return 0, mouseProtocolNoMatch
	}
	return index + 1, mouseProtocolFull
}
