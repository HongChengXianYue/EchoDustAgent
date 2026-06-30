package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	startupBlue       = "\x1b[38;5;39m"
	startupLightBlue  = "\x1b[38;5;117m"
	startupWhite      = "\x1b[38;5;255m"
	startupMuted      = "\x1b[38;5;248m"
	startupReset      = "\x1b[0m"
	startupWideWidth  = 80
	startupTitle      = "ECHO DUST CODE"
	startupQuitNotice = "type exit or quit to stop"
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
	fmt.Fprintln(output)
	for i, line := range startupBannerLines {
		color := startupBlue
		if i%2 == 1 {
			color = startupLightBlue
		}
		fmt.Fprintln(output, color+line+startupReset)
	}
	fmt.Fprintln(output)
	renderStartupDetails(output, info, "  ")
	fmt.Fprintln(output)
}

func renderCompactStartup(output io.Writer, info StartupInfo) {
	fmt.Fprintln(output, startupBlue+startupTitle+startupReset)
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
	lines = append(lines,
		"log file: "+info.LogFile,
		startupQuitNotice,
	)
	return lines
}
