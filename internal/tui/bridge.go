package tui

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"local-agent/internal/approval"
)

// Bridge keeps Bubble Tea program access behind a tiny synchronization layer so
// background goroutines can safely forward runtime events and approval prompts
// into the UI loop.
type Bridge struct {
	mu           sync.RWMutex
	program      *tea.Program
	approvalMode ApprovalMode
}

func NewBridge() *Bridge {
	return &Bridge{approvalMode: ApprovalModePrompt}
}

func (b *Bridge) SetProgram(program *tea.Program) {
	b.mu.Lock()
	b.program = program
	b.mu.Unlock()
}

func (b *Bridge) Send(msg tea.Msg) bool {
	b.mu.RLock()
	program := b.program
	b.mu.RUnlock()
	if program == nil {
		return false
	}
	program.Send(msg)
	return true
}

func (b *Bridge) ApprovalMode() ApprovalMode {
	if b == nil {
		return ApprovalModePrompt
	}
	b.mu.RLock()
	mode := b.approvalMode
	b.mu.RUnlock()
	return mode.normalized()
}

func (b *Bridge) SetApprovalMode(mode ApprovalMode) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.approvalMode = mode.normalized()
	b.mu.Unlock()
}

type BubbleApprover struct {
	bridge *Bridge
}

func NewBubbleApprover(bridge *Bridge) *BubbleApprover {
	return &BubbleApprover{bridge: bridge}
}

func (a *BubbleApprover) Approve(ctx context.Context, request approval.Request) approval.Decision {
	if a == nil || a.bridge == nil {
		return approval.DecisionDeny
	}
	if decision, ok := a.bridge.ApprovalMode().approvalDecision(request); ok {
		return decision
	}
	response := make(chan approval.Decision, 1)
	if !a.bridge.Send(approvalPromptMsg{Request: request, Response: response}) {
		return approval.DecisionDeny
	}
	select {
	case decision := <-response:
		return decision
	case <-ctx.Done():
		return approval.DecisionDeny
	}
}
