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
	mu      sync.RWMutex
	program *tea.Program
}

func NewBridge() *Bridge {
	return &Bridge{}
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
