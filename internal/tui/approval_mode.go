package tui

import "local-agent/internal/approval"

type ApprovalMode int

const (
	ApprovalModePrompt ApprovalMode = iota
	ApprovalModeAcceptAll
	ApprovalModeFullAgree
)

func (m ApprovalMode) normalized() ApprovalMode {
	switch m {
	case ApprovalModeAcceptAll, ApprovalModeFullAgree:
		return m
	default:
		return ApprovalModePrompt
	}
}

func (m ApprovalMode) next() ApprovalMode {
	switch m.normalized() {
	case ApprovalModePrompt:
		return ApprovalModeAcceptAll
	case ApprovalModeAcceptAll:
		return ApprovalModeFullAgree
	default:
		return ApprovalModePrompt
	}
}

func (m ApprovalMode) label() string {
	switch m.normalized() {
	case ApprovalModeAcceptAll:
		return "accept all"
	case ApprovalModeFullAgree:
		return "fully agree"
	default:
		return "approval"
	}
}

func (m ApprovalMode) approvalDecision(request approval.Request) (approval.Decision, bool) {
	switch m.normalized() {
	case ApprovalModeAcceptAll:
		if request.Category == approval.CategoryExternalOrDestructive ||
			request.Category == approval.CategorySystemPrivileged {
			return "", false
		}
	case ApprovalModeFullAgree:
	default:
		return "", false
	}

	// Auto modes should not poison MemoryApprover's session cache. Prefer
	// one-shot allow, and only fall back to always for legacy requests that do
	// not expose an allow-once option.
	for _, option := range approval.DecisionOptions(request) {
		if option == approval.DecisionAllow {
			return option, true
		}
	}
	for _, option := range approval.DecisionOptions(request) {
		if option == approval.DecisionAlways {
			return option, true
		}
	}
	return "", false
}
