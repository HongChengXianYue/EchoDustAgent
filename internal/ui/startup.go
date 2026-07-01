package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	startupBlue      = "\x1b[38;5;39m"
	startupLightBlue = "\x1b[38;5;117m"
	startupWhite     = "\x1b[38;5;255m"
	startupMuted     = "\x1b[38;5;248m"
	startupReset     = "\x1b[0m"
	startupWideWidth = 80
	startupTitle     = "ECHO DUST CODE"
)

var startupBannerLines = []string{
	"███████╗ ██████╗██╗  ██╗ ██████╗     ██████╗ ██╗   ██╗███████╗████████╗     ██████╗ ██████╗ ██████╗ ███████╗",
	"██╔════╝██╔════╝██║  ██║██╔═══██╗    ██╔══██╗██║   ██║██╔════╝╚══██╔══╝    ██╔════╝██╔═══██╗██╔══██╗██╔════╝",
	"█████╗  ██║     ███████║██║   ██║    ██║  ██║██║   ██║███████╗   ██║       ██║     ██║   ██║██║  ██║█████╗  ",
	"██╔══╝  ██║     ██╔══██║██║   ██║    ██║  ██║██║   ██║╚════██║   ██║       ██║     ██║   ██║██║  ██║██╔══╝  ",
	"███████╗╚██████╗██║  ██║╚██████╔╝    ██████╔╝╚██████╔╝███████║   ██║       ╚██████╗╚██████╔╝██████╔╝███████╗",
	"╚══════╝ ╚═════╝╚═╝  ╚═╝ ╚═════╝     ╚═════╝  ╚═════╝ ╚══════╝   ╚═╝        ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝",
}

type StartupInfo struct {
	Workdir    string
	Model      string
	WireAPI    string
	MCPEnabled bool
	MCPTools   int
	LogFile    string
}

func RenderStartupBanner(output io.Writer, info StartupInfo) {
	if shouldRenderWideStartup(output) {
		renderWideStartup(output, info)
		return
	}
	renderCompactStartup(output, info)
}

func shouldRenderWideStartup(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok || !isTerminal(file) {
		return false
	}
	width, _, err := term.GetSize(int(file.Fd()))
	return err == nil && width >= startupWideWidth
}

func renderWideStartup(output io.Writer, info StartupInfo) {
	// 取终端宽度，用于将大字 banner 居中。
	termWidth := 0
	if file, ok := output.(*os.File); ok && isTerminal(file) {
		w, _, err := term.GetSize(int(file.Fd()))
		if err == nil {
			termWidth = w
		}
	}
	bannerWidth := 0
	for _, line := range startupBannerLines {
		if w := len([]rune(line)); w > bannerWidth {
			bannerWidth = w
		}
	}
	padding := 0
	if termWidth > bannerWidth {
		padding = (termWidth - bannerWidth) / 2
	}

	fmt.Fprintln(output)
	for i, line := range startupBannerLines {
		color := startupBlue
		if i%2 == 1 {
			color = startupLightBlue
		}
		fmt.Fprintln(output, strings.Repeat(" ", padding)+color+line+startupReset)
	}
	fmt.Fprintln(output)
}

func renderCompactStartup(output io.Writer, info StartupInfo) {
	fmt.Fprintln(output, startupBlue+startupTitle+startupReset)
	fmt.Fprintln(output)
}

// RenderStartupDetails 按需打印启动详情（workdir / model / mcp tools / log file
// 等）。供 main 循环在用户输入 /info 时调用，启动时不自动输出。
func RenderStartupDetails(output io.Writer, info StartupInfo) {
	renderStartupDetails(output, info, "")
}

func renderStartupDetails(output io.Writer, info StartupInfo, indent string) {
	for _, line := range startupDetailLines(info) {
		label, value, ok := strings.Cut(line, ": ")
		if ok {
			fmt.Fprintf(output, "%s%s%s:%s %s%s\n", indent, startupMuted, label, startupReset, startupWhite, value+startupReset)
			continue
		}
		fmt.Fprintf(output, "%s%s%s%s\n", indent, startupLightBlue, line, startupReset)
	}
}

func startupDetailLines(info StartupInfo) []string {
	lines := []string{
		"workdir: " + info.Workdir,
		"model: " + info.Model,
		"wire api: " + info.WireAPI,
	}
	if info.MCPEnabled {
		lines = append(lines, fmt.Sprintf("mcp tools: %d", info.MCPTools))
	}
	lines = append(lines, "log file: "+info.LogFile)
	return lines
}
