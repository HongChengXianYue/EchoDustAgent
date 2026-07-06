package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"local-agent/internal/approval"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.markLayoutDirty()
		m.syncLayout()
		return m, nil
	case runtimeEventMsg:
		m.applyRuntimeEvent(msg.Event)
		m.syncLayout()
		return m, nil
	case runTimerTickMsg:
		if m.running && !m.runStartedAt.IsZero() {
			m.runElapsedMS = msg.At.Sub(m.runStartedAt).Milliseconds()
			if m.chatRetry != nil {
				m.markViewportDirty()
			}
			m.syncLayout()
			return m, m.nextRunTimerTick()
		}
		return m, nil
	case approvalPromptMsg:
		m.approval = &approvalState{
			Request:  msg.Request,
			Options:  approval.DecisionOptions(msg.Request),
			Response: msg.Response,
		}
		m.markViewportDirty()
		m.syncLayout()
		m.viewport.GotoBottom()
		return m, nil
	case runFinishedMsg:
		m.cancelCurrent = nil
		m.running = false
		m.chatRetry = nil
		m.approval = nil
		if !m.runStartedAt.IsZero() && m.runElapsedMS == 0 {
			m.runElapsedMS = time.Since(m.runStartedAt).Milliseconds()
		}
		m.interrupting = false
		m.hideSubagentPanel()
		m.markLayoutDirty()
		m.markViewportDirty()
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) && !m.runErrorReported {
			if !m.lastRunHadFinal && strings.TrimSpace(m.assistantDraft) != "" {
				m.appendBlock(transcriptBlock{
					Kind:  blockAssistant,
					Title: "Agent (partial)",
					Body:  cleanTerminalText(m.assistantDraft),
				})
			}
			m.appendBlock(transcriptBlock{
				Kind:  blockError,
				Title: "Run failed",
				Body:  msg.Err.Error(),
			})
		}
		if msg.Err != nil {
			m.assistantDraft = ""
			m.markViewportDirty()
		}
		m.syncLayout()
		if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
			m.persistSessionSnapshot()
		}
		return m, nil
	case SignalMsg:
		if m.approval != nil {
			if m.running {
				m.resolveApproval(approval.DecisionDeny)
				m.interruptRun()
				return m, nil
			}
			m.resolveApproval(approval.DecisionDeny)
			return m, nil
		}
		if m.running {
			m.interruptRun()
			return m, nil
		}
		return m, tea.Quit
	case tea.MouseMsg:
		return m, m.updateActiveViewport(msg)
	case tea.KeyMsg:
		if m.approval != nil {
			return m.updateApproval(msg)
		}
		return m.updateKey(msg)
	}

	return m, m.updateInput(msg)
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.resumePickerActive() {
		switch msg.String() {
		case "esc":
			m.cancelResumePicker()
			m.syncLayout()
			return m, nil
		case "up", "k", "K":
			m.moveResumeSelection(-1)
			m.syncLayout()
			return m, nil
		case "down", "j", "J":
			m.moveResumeSelection(1)
			m.syncLayout()
			return m, nil
		case "enter":
			output := m.confirmResumeSelection()
			if strings.TrimSpace(output) != "" {
				m.appendBlock(transcriptBlock{Kind: blockInfo, Title: "Slash", Body: output})
			}
			m.syncLayout()
			return m, nil
		}
	}
	switch msg.String() {
	case "ctrl+c":
		if m.running {
			m.interruptRun()
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.running {
			m.interruptRun()
			return m, nil
		}
		if m.viewingSubagent {
			m.viewingSubagent = false
			m.markLayoutDirty()
			m.syncLayout()
			return m, nil
		}
	case "pgup", "pageup", "pgdown", "pagedown":
		return m, m.updateActiveViewport(msg)
	case "home":
		m.gotoActiveTop()
		return m, nil
	case "end":
		m.gotoActiveBottom()
		return m, nil
	case "enter":
		if m.shouldOpenSelectedSubagent() {
			m.viewingSubagent = true
			m.markLayoutDirty()
			m.markSubagentViewportDirty()
			m.syncLayout()
			return m, nil
		}
		return m, m.submitInput()
	case "up":
		if m.viewingSubagent {
			return m, m.updateActiveViewport(msg)
		}
		if m.shouldNavigateSubagents() {
			m.selectPreviousSubagent()
			return m, nil
		}
		if len(m.input.MatchedSuggestions()) == 0 && m.historyUp() {
			return m, nil
		}
	case "down":
		if m.viewingSubagent {
			return m, m.updateActiveViewport(msg)
		}
		if m.shouldNavigateSubagents() {
			m.selectNextSubagent()
			return m, nil
		}
		if len(m.input.MatchedSuggestions()) == 0 && m.historyDown() {
			return m, nil
		}
	}

	return m, m.updateInput(msg)
}

func (m *Model) updateApproval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		if m.running {
			m.resolveApproval(approval.DecisionDeny)
			m.interruptRun()
			break
		}
		m.resolveApproval(approval.DecisionDeny)
	case "n", "N", "d", "D":
		m.resolveApproval(approval.DecisionDeny)
	case "pgup", "pageup", "pgdown", "pagedown":
		return m, m.updateActiveViewport(msg)
	case "home":
		m.gotoActiveTop()
		return m, nil
	case "end":
		m.gotoActiveBottom()
		return m, nil
	case "left", "up", "k", "K":
		if m.approval != nil && len(m.approval.Options) > 0 {
			m.approval.Selected = (m.approval.Selected - 1 + len(m.approval.Options)) % len(m.approval.Options)
			m.markViewportDirty()
		}
	case "right", "down", "j", "J", "tab":
		if m.approval != nil && len(m.approval.Options) > 0 {
			m.approval.Selected = (m.approval.Selected + 1) % len(m.approval.Options)
			m.markViewportDirty()
		}
	case "a", "A", "y", "Y":
		if decision, ok := m.quickApprovalDecision(approval.DecisionAllow, approval.DecisionAlways); ok {
			m.resolveApproval(decision)
		}
	case "enter":
		if m.approval != nil && len(m.approval.Options) > 0 {
			m.resolveApproval(m.approval.Options[m.approval.Selected])
		}
	}
	m.syncLayout()
	return m, nil
}

func (m *Model) quickApprovalDecision(primary, fallback approval.Decision) (approval.Decision, bool) {
	if m.approval == nil {
		return "", false
	}
	for _, option := range m.approval.Options {
		if option == primary {
			return option, true
		}
	}
	for _, option := range m.approval.Options {
		if option == fallback {
			return option, true
		}
	}
	return "", false
}

func (m *Model) resolveApproval(decision approval.Decision) {
	if m.approval == nil {
		return
	}
	response := m.approval.Response
	m.approval = nil
	m.markViewportDirty()
	if response != nil {
		response <- decision
		close(response)
	}
}

func (m *Model) submitInput() tea.Cmd {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return nil
	}
	if input == "/resume" && !m.running {
		if handled, output := m.openResumePicker(); handled {
			m.addHistory(input)
			m.input.Reset()
			m.markLayoutDirty()
			m.historyPos = len(m.history)
			m.historyDraft = ""
			if strings.TrimSpace(output) != "" {
				m.appendBlock(transcriptBlock{Kind: blockInfo, Title: "Slash", Body: output})
			}
			m.syncLayout()
			return nil
		}
	}
	if m.slashFunc != nil && strings.HasPrefix(input, "/") {
		output, handled, shouldExit := m.slashFunc(input)
		if handled {
			m.addHistory(input)
			m.input.Reset()
			m.markLayoutDirty()
			m.historyPos = len(m.history)
			m.historyDraft = ""
			if strings.TrimSpace(output) != "" {
				m.appendBlock(transcriptBlock{Kind: blockInfo, Title: "Slash", Body: output})
			}
			m.syncLayout()
			if shouldExit {
				if m.running {
					m.interruptRun()
				}
				return tea.Quit
			}
			return nil
		}
	}
	if m.running {
		m.appendBlock(transcriptBlock{
			Kind:  blockInfo,
			Title: "Busy",
			Body:  "Agent is running. Wait for it to finish or press Esc/Ctrl+C to interrupt.",
		})
		m.syncLayout()
		return nil
	}
	m.addHistory(input)
	m.input.Reset()
	m.markLayoutDirty()
	m.historyPos = len(m.history)
	m.historyDraft = ""
	return m.startRun(input)
}

func (m *Model) startRun(input string) tea.Cmd {
	if m.runFunc == nil {
		m.appendBlock(transcriptBlock{Kind: blockError, Title: "Run failed", Body: "run function is not configured"})
		m.syncLayout()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelCurrent = cancel
	m.running = true
	m.runStartedAt = time.Now()
	m.runElapsedMS = 0
	m.interrupting = false
	m.lastRunHadFinal = false
	m.runErrorReported = false
	m.assistantDraft = ""
	m.chatRetry = nil
	m.markViewportDirty()
	runCmd := func() tea.Msg {
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("agent panic: %v", recovered)
			}
		}()
		err = m.runFunc(ctx, input)
		return runFinishedMsg{Err: err}
	}
	return tea.Batch(runCmd, m.nextRunTimerTick())
}

func (m *Model) interruptRun() {
	if m.cancelCurrent != nil {
		m.interrupting = true
		m.cancelCurrent()
	}
}

func (m *Model) addHistory(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if len(m.history) > 0 && m.history[len(m.history)-1] == line {
		return
	}
	m.history = append(m.history, line)
	m.historyPos = len(m.history)
}

func (m *Model) historyUp() bool {
	if len(m.history) == 0 || m.historyPos == 0 {
		return false
	}
	if m.historyPos == len(m.history) {
		m.historyDraft = m.input.Value()
	}
	m.historyPos--
	m.input.SetValue(m.history[m.historyPos])
	m.markLayoutDirty()
	return true
}

func (m *Model) historyDown() bool {
	if len(m.history) == 0 || m.historyPos >= len(m.history) {
		return false
	}
	m.historyPos++
	if m.historyPos == len(m.history) {
		m.input.SetValue(m.historyDraft)
		m.markLayoutDirty()
		return true
	}
	m.input.SetValue(m.history[m.historyPos])
	m.markLayoutDirty()
	return true
}

func (m *Model) updateInput(msg tea.Msg) tea.Cmd {
	beforeValue := m.input.Value()
	beforeSuggestionCount := len(m.matchedSlashCommands())
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if beforeValue != m.input.Value() || beforeSuggestionCount != len(m.matchedSlashCommands()) {
		m.markLayoutDirty()
	}
	return cmd
}

func (m *Model) nextRunTimerTick() tea.Cmd {
	if !m.running {
		return nil
	}
	return tea.Tick(200*time.Millisecond, func(at time.Time) tea.Msg {
		return runTimerTickMsg{At: at}
	})
}
