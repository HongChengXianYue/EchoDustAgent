package approval

import "context"

type MemoryApprover struct {
	next   Approver
	always map[string]struct{}
}

func NewMemoryApprover(next Approver) *MemoryApprover {
	return &MemoryApprover{
		next:   next,
		always: map[string]struct{}{},
	}
}

func (a *MemoryApprover) Approve(ctx context.Context, request Request) Decision {
	key := CacheKey(request)
	scope := request.Scope
	if scope == "" {
		scope = ScopeSession
	}
	if scope == ScopeSession {
		if _, ok := a.always[key]; ok {
			return DecisionAlways
		}
	}
	if a.next == nil {
		return DecisionDeny
	}
	decision := a.next.Approve(ctx, request)
	if scope == ScopeSession && decision == DecisionAlways {
		a.always[key] = struct{}{}
		return DecisionAlways
	}
	return decision
}
