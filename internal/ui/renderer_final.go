package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func (r *BlockRenderer) renderFinal(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	fmt.Fprintln(r.output, separatorLine(r.options.SeparatorWidth))
	rendered, err := renderMarkdown(r.markdownRenderer, message)
	if err != nil {
		fmt.Fprintln(r.output, message)
		return
	}
	fmt.Fprint(r.output, rendered)
	if !strings.HasSuffix(rendered, "\n") {
		fmt.Fprintln(r.output)
	}
}

func renderMarkdown(renderer *glamour.TermRenderer, message string) (string, error) {
	if renderer == nil {
		return glamour.Render(message, "dark")
	}
	return renderer.Render(message)
}

func newMarkdownRenderer(wordWrap int) (*glamour.TermRenderer, error) {
	style := styles.DarkStyleConfig
	clearHeadingPrefixes(&style)
	softenCodeBlocks(&style)
	return glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(wordWrap),
	)
}

func clearHeadingPrefixes(style *ansi.StyleConfig) {
	for _, heading := range []*ansi.StyleBlock{
		&style.H1,
		&style.H2,
		&style.H3,
		&style.H4,
		&style.H5,
		&style.H6,
	} {
		heading.Prefix = ""
		heading.Suffix = ""
	}
}

func softenCodeBlocks(style *ansi.StyleConfig) {
	style.CodeBlock.Theme = ""
	style.CodeBlock.Chroma = nil
	style.CodeBlock.BackgroundColor = nil
	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.Indent = uintPtr(1)
}

func uintPtr(v uint) *uint {
	return &v
}
