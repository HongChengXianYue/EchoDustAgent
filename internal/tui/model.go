package tui

import (
	"context"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/session"
	"local-agent/internal/ui"
)

type RunFunc func(ctx context.Context, input string) error
type SlashFunc func(input string) (output string, handled bool, shouldExit bool)
type ResumeListFunc func() ([]session.Meta, error)
type ResumeSelectFunc func(sessionID string) (string, error)

type runtimeEventMsg struct {
	Event runtimeevent.Event
}

type runFinishedMsg struct {
	Err error
}

type approvalPromptMsg struct {
	Request  approval.Request
	Response chan approval.Decision
}

type SignalMsg struct {
	Signal os.Signal
}

// Keep a local copy of the wide startup banner so the Bubble Tea UI can
// render the same visual identity without depending on legacy UI internals.
var tuiBannerLines = []string{
	"███████╗ ██████╗██╗  ██╗ ██████╗     ██████╗ ██╗   ██╗███████╗████████╗     ██████╗ ██████╗ ██████╗ ███████╗",
	"██╔════╝██╔════╝██║  ██║██╔═══██╗    ██╔══██╗██║   ██║██╔════╝╚══██╔══╝    ██╔════╝██╔═══██╗██╔══██╗██╔════╝",
	"█████╗  ██║     ███████║██║   ██║    ██║  ██║██║   ██║███████╗   ██║       ██║     ██║   ██║██║  ██║█████╗  ",
	"██╔══╝  ██║     ██╔══██║██║   ██║    ██║  ██║██║   ██║╚════██║   ██║       ██║     ██║   ██║██║  ██║██╔══╝  ",
	"███████╗╚██████╗██║  ██║╚██████╔╝    ██████╔╝╚██████╔╝███████║   ██║       ╚██████╗╚██████╔╝██████╔╝███████╗",
	"╚══════╝ ╚═════╝╚═╝  ╚═╝ ╚═════╝     ╚═════╝  ╚═════╝ ╚══════╝   ╚═╝        ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝",
}

type approvalState struct {
	Request  approval.Request
	Options  []approval.Decision
	Selected int
	Response chan approval.Decision
}

type tokenState struct {
	Prompt     int
	Completion int
	Total      int
	Cached     int
}

type resumePickerState struct {
	Sessions []session.Meta
	Selected int
}

type subagentSession struct {
	Index     int
	Task      string
	Blocks    []transcriptBlock
	Status    string
	LastTitle string
	// Cache hit rate uses prompt-side tokens as the denominator, so subagents
	// track prompt usage separately from their cumulative total.
	Prompt     int
	TokenTotal int
	Cached     int
}

type Model struct {
	bridge        *Bridge
	options       ui.Options
	startup       ui.StartupInfo
	runFunc       RunFunc
	slashFunc     SlashFunc
	resumeList    ResumeListFunc
	resumeSelect  ResumeSelectFunc
	snapshotSaver func(session.UISnapshot)
	slashCommands []ui.CommandSuggestion
	commandByName map[string]ui.CommandSuggestion

	width  int
	height int

	input    textinput.Model
	viewport viewport.Model

	blocks           []transcriptBlock
	assistantDraft   string
	approval         *approvalState
	resumePicker     *resumePickerState
	todos            []runtimeevent.TodoItem
	subagents        map[int]*subagentSession
	subagentOrder    []int
	subagentTaskMap  map[string]int
	nextSubagentID   int
	showSubagents    bool
	selectedSubagent int
	viewingSubagent  bool
	subagentViewport viewport.Model
	subagentHeight   int
	history          []string
	historyPos       int
	historyDraft     string
	running          bool
	runStartBlock    int
	interrupting     bool
	cancelCurrent    context.CancelFunc
	lastRunHadFinal  bool
	runErrorReported bool
	lastTool         string
	tokens           tokenState

	markdownRenderer  *glamour.TermRenderer
	markdownWrapWidth int

	bannerStyle           lipgloss.Style
	bannerAltStyle        lipgloss.Style
	contentStyle          lipgloss.Style
	inputBoxStyle         lipgloss.Style
	userPromptMarkerStyle lipgloss.Style
	userPromptTextStyle   lipgloss.Style
	subagentBoxStyle      lipgloss.Style
	titleStyle            lipgloss.Style
	mutedStyle            lipgloss.Style
	todoStyle             lipgloss.Style
	todoDoneStyle         lipgloss.Style
	assistantBodyStyle    lipgloss.Style
	errorStyle            lipgloss.Style
	toolCallTitleStyle    lipgloss.Style
	toolCallDotStyle      lipgloss.Style
	diffMetaStyle         lipgloss.Style
	diffAddStyle          lipgloss.Style
	diffRemoveStyle       lipgloss.Style
	diffContextStyle      lipgloss.Style
	diffEllipsisStyle     lipgloss.Style
	approvalSelectedStyle lipgloss.Style
	approvalHintStyle     lipgloss.Style
	subagentTitleStyle    lipgloss.Style
	subagentSelectedStyle lipgloss.Style
	subagentOpenStyle     lipgloss.Style
	subagentIdleStyle     lipgloss.Style
}

func NewModel(options ui.Options, startup ui.StartupInfo, bridge *Bridge) *Model {
	options = mergeOptions(options)

	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = "Ask the agent"
	input.ShowSuggestions = true
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	input.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	input.CompletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))

	model := &Model{
		bridge:           bridge,
		options:          options,
		startup:          startup,
		input:            input,
		viewport:         viewport.New(0, 0),
		subagentViewport: viewport.New(0, 0),
		commandByName:    map[string]ui.CommandSuggestion{},
		subagents:        map[int]*subagentSession{},
		subagentTaskMap:  map[string]int{},
		nextSubagentID:   1,
		bannerStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		bannerAltStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		contentStyle:     lipgloss.NewStyle(),
		inputBoxStyle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
		userPromptMarkerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("221")).
			Bold(true),
		userPromptTextStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true),
		subagentBoxStyle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1),
		titleStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		mutedStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		todoStyle:             lipgloss.NewStyle().Foreground(lipgloss.Color("150")).Bold(true),
		todoDoneStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
		assistantBodyStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
		errorStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		toolCallTitleStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
		toolCallDotStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		diffMetaStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
		diffAddStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color("#8BD5A0")).Background(lipgloss.Color("#183126")),
		diffRemoveStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("#F2B8BD")).Background(lipgloss.Color("#352327")),
		diffContextStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		diffEllipsisStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		approvalSelectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		approvalHintStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		subagentTitleStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true),
		subagentSelectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		subagentOpenStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		subagentIdleStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
	model.input.Focus()
	model.historyPos = 0
	model.viewport.MouseWheelEnabled = true
	model.viewport.MouseWheelDelta = 3
	model.subagentViewport.MouseWheelEnabled = true
	model.subagentViewport.MouseWheelDelta = 3
	return model
}

func (m *Model) SetRunFunc(runFunc RunFunc) {
	m.runFunc = runFunc
}

func (m *Model) SetSlashFunc(slashFunc SlashFunc) {
	m.slashFunc = slashFunc
}

func (m *Model) SetResumePickerHandlers(list ResumeListFunc, selectFn ResumeSelectFunc) {
	m.resumeList = list
	m.resumeSelect = selectFn
}

func (m *Model) SetSessionSnapshotSaver(save func(session.UISnapshot)) {
	m.snapshotSaver = save
}

func (m *Model) SetSlashCommands(commands []ui.CommandSuggestion) {
	m.slashCommands = append([]ui.CommandSuggestion(nil), commands...)
	m.commandByName = make(map[string]ui.CommandSuggestion, len(commands))
	suggestions := make([]string, 0, len(commands))
	for _, command := range commands {
		m.commandByName["/"+command.Name] = command
		suggestions = append(suggestions, "/"+command.Name)
	}
	m.input.SetSuggestions(suggestions)
}

func (m *Model) HandleEvent(event runtimeevent.Event) {
	if m.bridge == nil {
		return
	}
	m.bridge.Send(runtimeEventMsg{Event: event})
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func mergeOptions(options ui.Options) ui.Options {
	defaults := ui.DefaultOptions()
	if options.SeparatorWidth == 0 {
		options.SeparatorWidth = defaults.SeparatorWidth
	}
	if options.LiveFrameMaxLines <= 0 {
		options.LiveFrameMaxLines = defaults.LiveFrameMaxLines
	}
	if options.LiveFrameHeightMargin <= 0 {
		options.LiveFrameHeightMargin = defaults.LiveFrameHeightMargin
	}
	if options.MaxExpandedLiveToolEvents <= 0 {
		options.MaxExpandedLiveToolEvents = defaults.MaxExpandedLiveToolEvents
	}
	if options.MarkdownWordWrap < 0 {
		options.MarkdownWordWrap = defaults.MarkdownWordWrap
	}
	if options.ToolPreviewOutputChars <= 0 {
		options.ToolPreviewOutputChars = defaults.ToolPreviewOutputChars
	}
	if options.ToolPreviewLongOutputChars <= 0 {
		options.ToolPreviewLongOutputChars = defaults.ToolPreviewLongOutputChars
	}
	if options.FileChangePreviewChars <= 0 {
		options.FileChangePreviewChars = defaults.FileChangePreviewChars
	}
	if options.ApprovalArgsPreviewChars <= 0 {
		options.ApprovalArgsPreviewChars = defaults.ApprovalArgsPreviewChars
	}
	return options
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
