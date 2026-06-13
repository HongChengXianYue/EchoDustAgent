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
	if _, ok := a.always[key]; ok {
		return DecisionAlways
	}
	if a.next == nil {
		return DecisionDeny
	}
	decision := a.next.Approve(ctx, request)
	if decision == DecisionAlways {
		a.always[key] = struct{}{}
	}
	return decision
}
