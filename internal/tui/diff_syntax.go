package tui

import (
	"path/filepath"
	"strings"

	chroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

type diffStyledSpan struct {
	Text  string
	Style lipgloss.Style
}

type diffSyntaxHighlighter struct {
	lexer chroma.Lexer
}

func newDiffSyntaxHighlighter(diffText string) *diffSyntaxHighlighter {
	path := diffSyntaxPath(diffText)
	sample := diffSyntaxSample(diffText)

	lexer := lexers.Match(path)
	if lexer == nil && strings.TrimSpace(sample) != "" {
		lexer = lexers.Analyse(sample)
	}
	if (lexer == nil || lexer.Config().Name == "fallback") && strings.TrimSpace(sample) != "" {
		lexer = diffHeuristicLexer(sample)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return &diffSyntaxHighlighter{lexer: chroma.Coalesce(lexer)}
}

func (h *diffSyntaxHighlighter) highlight(text string, baseStyle lipgloss.Style) []diffStyledSpan {
	if h == nil || h.lexer == nil || text == "" {
		return []diffStyledSpan{{Text: text, Style: baseStyle}}
	}
	iterator, err := h.lexer.Tokenise(nil, text)
	if err != nil {
		return []diffStyledSpan{{Text: text, Style: baseStyle}}
	}

	spans := make([]diffStyledSpan, 0, 8)
	for token := iterator(); token != chroma.EOF; token = iterator() {
		if token.Value == "" {
			continue
		}
		spans = append(spans, diffStyledSpan{
			Text:  token.Value,
			Style: diffTokenStyle(baseStyle, token.Type),
		})
	}
	if len(spans) == 0 {
		return []diffStyledSpan{{Text: text, Style: baseStyle}}
	}
	return spans
}

func diffSyntaxPath(diffText string) string {
	for _, line := range strings.Split(strings.TrimRight(diffText, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			if path := diffSyntaxCleanPath(strings.TrimSpace(strings.TrimPrefix(line, "+++ "))); path != "" {
				return path
			}
		case strings.HasPrefix(line, "--- "):
			if path := diffSyntaxCleanPath(strings.TrimSpace(strings.TrimPrefix(line, "--- "))); path != "" {
				return path
			}
		}
	}
	return ""
}

func diffSyntaxSample(diffText string) string {
	lines := make([]string, 0, 16)
	for _, line := range strings.Split(strings.TrimRight(diffText, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
			continue
		case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"), strings.HasPrefix(line, " "):
			lines = append(lines, line[1:])
		}
	}
	return strings.Join(lines, "\n")
}

func diffTokenStyle(baseStyle lipgloss.Style, tokenType chroma.TokenType) lipgloss.Style {
	style := diffInheritedTextStyle(baseStyle)
	switch {
	case tokenType.InSubCategory(chroma.Comment):
		return style.Foreground(lipgloss.Color("#7F849C")).Italic(true)
	case tokenType.InSubCategory(chroma.Keyword):
		return style.Foreground(lipgloss.Color("#8AADF4")).Bold(true)
	case tokenType.InSubCategory(chroma.NameFunction):
		return style.Foreground(lipgloss.Color("#C6A0F6"))
	case tokenType.InSubCategory(chroma.NameBuiltin):
		return style.Foreground(lipgloss.Color("#EED49F"))
	case tokenType.InSubCategory(chroma.NameClass),
		tokenType.InSubCategory(chroma.NameNamespace),
		tokenType.InSubCategory(chroma.NameDecorator),
		tokenType.InSubCategory(chroma.KeywordType):
		return style.Foreground(lipgloss.Color("#7DC4E4"))
	case tokenType.InSubCategory(chroma.NameConstant):
		return style.Foreground(lipgloss.Color("#F5BDE6"))
	case tokenType.InSubCategory(chroma.LiteralString):
		return style.Foreground(lipgloss.Color("#A6DA95"))
	case tokenType.InSubCategory(chroma.LiteralNumber):
		return style.Foreground(lipgloss.Color("#F5A97F"))
	case tokenType.InSubCategory(chroma.Operator):
		return style.Foreground(lipgloss.Color("#91D7E3"))
	default:
		return style
	}
}

func diffInheritedTextStyle(baseStyle lipgloss.Style) lipgloss.Style {
	style := lipgloss.NewStyle()
	if foreground := baseStyle.GetForeground(); foreground != nil {
		style = style.Foreground(foreground)
	}
	if background := baseStyle.GetBackground(); background != nil {
		style = style.Background(background)
	}
	if baseStyle.GetBold() {
		style = style.Bold(true)
	}
	if baseStyle.GetItalic() {
		style = style.Italic(true)
	}
	return style
}

func diffHeuristicLexer(sample string) chroma.Lexer {
	trimmed := strings.TrimSpace(sample)
	switch {
	case looksLikeJSON(trimmed):
		return lexers.Get("json")
	default:
		return nil
	}
}

func looksLikeJSON(text string) bool {
	if text == "" {
		return false
	}
	if !(strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) {
		return false
	}
	return strings.Contains(text, "\":") || strings.Contains(text, "\": ")
}

func diffSyntaxCleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		path = path[2:]
	}
	return filepath.ToSlash(path)
}
