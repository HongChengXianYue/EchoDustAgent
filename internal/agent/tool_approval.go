package agent

import (
	"context"
	"encoding/json"

	"local-agent/internal/approval"
	"local-agent/internal/runtimeevent"
)

func (a *Agent) approveTool(ctx context.Context, step int, tool string, category approval.Category, args json.RawMessage, impact approval.WriteImpact, loopApprovals map[string]bool) bool {
	if !approval.RequiresApproval(category) && !impact.Writes {
		return true
	}
	if a.approver == nil {
		return true
	}
	request := approvalRequest(tool, category, args, impact)
	key := approval.CacheKey(request)
	if request.Scope == approval.ScopeLoop && loopApprovals[key] {
		return true
	}
	// Session-scoped approvals can be remembered by the approver. If so, avoid
	// emitting another approval request/decision pair into the transcript.
	if cachedApprover, ok := a.approver.(approval.DecisionCache); ok {
		if decision, cached := cachedApprover.CachedDecision(request); cached {
			return decision == approval.DecisionAllow || decision == approval.DecisionAlways
		}
	}
	a.emit(runtimeevent.Event{
		Step:      step,
		Type:      runtimeevent.TypeApprovalRequest,
		Tool:      tool,
		Category:  category,
		Args:      args,
		Decisions: approval.DecisionOptions(request),
		Reason:    request.Reason,
	})
	decision := a.approver.Approve(ctx, request)
	if request.Scope == approval.ScopeLoop && decision == approval.DecisionAlways {
		loopApprovals[key] = true
	}
	a.emit(runtimeevent.Event{
		Step:     step,
		Type:     runtimeevent.TypeApprovalDecision,
		Tool:     tool,
		Category: category,
		Args:     args,
		Decision: string(decision),
		Reason:   request.Reason,
	})
	return decision == approval.DecisionAllow || decision == approval.DecisionAlways
}

func approvalRequest(tool string, category approval.Category, args json.RawMessage, impact approval.WriteImpact) approval.Request {
	request := approval.Request{
		Tool:     tool,
		Category: category,
		Args:     args,
		Reason:   "tool execution requested",
		Scope:    approval.ScopeSession,
	}
	switch {
	case impact.Writes && impact.External:
		request.Reason = "external write requested"
		request.Scope = approval.ScopeLoop
		request.Key = approval.ExternalWriteApprovalKey()
		request.Options = []approval.Decision{approval.DecisionAllow, approval.DecisionAlways, approval.DecisionDeny}
	case impact.Writes:
		request.Reason = "workspace write requested"
		request.Scope = approval.ScopeSession
		request.Key = approval.WorkspaceWriteApprovalKey()
		request.Options = []approval.Decision{approval.DecisionAlways, approval.DecisionDeny}
	}
	return request
}
