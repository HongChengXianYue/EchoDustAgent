package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"local-agent/internal/llm"
	"local-agent/internal/logs"
	"local-agent/internal/runtimeevent"
	"local-agent/internal/tools"
)

const repeatedToolSignatureLimit = 3

type stepBudget struct {
	limit      int
	extensions int
	options    StepBudgetOptions
}

type stepProgress struct {
	toolCalls       int
	successfulTools int
	failedTools     int
	todoChanged     bool
	todoOpen        bool
	signature       string
}

type stepProgressHistory struct {
	recent []stepProgress
}

func newStepBudget(initial int, options StepBudgetOptions) stepBudget {
	if initial <= 0 {
		initial = 10
	}
	if options.AbsoluteMaxSteps < initial {
		options.AbsoluteMaxSteps = initial
	}
	if options.ExtensionSize <= 0 {
		options.ExtensionSize = 1
	}
	if options.MaxExtensions < 0 {
		options.MaxExtensions = 0
	}
	return stepBudget{limit: initial, options: options}
}

func (b stepBudget) canExtend() bool {
	return b.options.AdaptiveEnabled &&
		b.extensions < b.options.MaxExtensions &&
		b.limit < b.options.AbsoluteMaxSteps
}

func (b *stepBudget) extend() (before int, after int) {
	before = b.limit
	after = b.limit + b.options.ExtensionSize
	if after > b.options.AbsoluteMaxSteps {
		after = b.options.AbsoluteMaxSteps
	}
	b.limit = after
	b.extensions++
	return before, after
}

func (h *stepProgressHistory) record(progress stepProgress) {
	h.recent = append(h.recent, progress)
	if len(h.recent) > repeatedToolSignatureLimit {
		h.recent = h.recent[len(h.recent)-repeatedToolSignatureLimit:]
	}
}

func (h stepProgressHistory) shouldExtend() (bool, string) {
	if len(h.recent) == 0 {
		return false, "no tool progress before the step budget was exhausted"
	}
	latest := h.recent[len(h.recent)-1]
	if latest.toolCalls == 0 {
		if latest.todoChanged && latest.todoOpen && !h.repeatedToolSignature() {
			return true, "todo plan changed and open work remains"
		}
		return false, "latest step did not request workspace tools"
	}
	if latest.successfulTools == 0 {
		return false, "latest step had no successful tool results"
	}
	if h.repeatedToolSignature() {
		return false, "repeated identical tool calls indicate a likely loop"
	}
	if h.consecutiveFailedToolSteps() >= 2 {
		return false, "recent steps only produced failed tool results"
	}
	if latest.todoOpen {
		return true, "open todo work remains and tools are still succeeding"
	}
	return true, "tools are still succeeding"
}

func (h stepProgressHistory) repeatedToolSignature() bool {
	if len(h.recent) < repeatedToolSignatureLimit {
		return false
	}
	signature := h.recent[0].signature
	if signature == "" {
		return false
	}
	for _, progress := range h.recent[1:] {
		if progress.signature != signature {
			return false
		}
	}
	return true
}

func (h stepProgressHistory) consecutiveFailedToolSteps() int {
	count := 0
	for i := len(h.recent) - 1; i >= 0; i-- {
		progress := h.recent[i]
		if progress.toolCalls == 0 || progress.successfulTools > 0 {
			break
		}
		count++
	}
	return count
}

func stepProgressFromExecuted(calls []llm.ToolCall, executed []executedToolCall, todosBefore []tools.TodoItem, todosAfter []tools.TodoItem) stepProgress {
	progress := stepProgress{
		todoChanged: !todoItemsEqual(todosBefore, todosAfter),
		todoOpen:    hasOpenTodo(todosAfter),
		signature:   toolCallSignature(calls, todosAfter),
	}
	for _, call := range executed {
		if tools.IsUpdateTodosTool(call.call.Function.Name) {
			continue
		}
		progress.toolCalls++
		if call.result.Status == "error" {
			progress.failedTools++
			continue
		}
		progress.successfulTools++
	}
	return progress
}

func hasOpenTodo(todos []tools.TodoItem) bool {
	for _, item := range todos {
		if item.Status == tools.TodoPending || item.Status == tools.TodoInProgress {
			return true
		}
	}
	return false
}

func todoItemsEqual(left []tools.TodoItem, right []tools.TodoItem) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func toolCallSignature(calls []llm.ToolCall, todos []tools.TodoItem) string {
	var b strings.Builder
	for _, call := range calls {
		if tools.IsUpdateTodosTool(call.Function.Name) {
			continue
		}
		b.WriteString(call.Function.Name)
		b.WriteByte('\n')
		b.WriteString(strings.TrimSpace(call.Function.Arguments))
		b.WriteByte('\n')
	}
	if b.Len() == 0 && len(todos) > 0 {
		for _, item := range todos {
			b.WriteString(string(item.Status))
			b.WriteByte(':')
			b.WriteString(strings.TrimSpace(item.Text))
			b.WriteByte('\n')
		}
	}
	if b.Len() == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (a *Agent) maybeExtendStepBudget(ctx context.Context, step int, budget *stepBudget, history stepProgressHistory) bool {
	if step+1 < budget.limit {
		return true
	}
	if ctx.Err() != nil {
		a.emitStepBudgetStop(step, budget.limit, "context was cancelled")
		return false
	}
	if !budget.canExtend() {
		a.emitStepBudgetStop(step, budget.limit, "step budget extension limit reached")
		return false
	}
	if reason := a.contextExtensionBlockReason(ctx); reason != "" {
		a.emitStepBudgetStop(step, budget.limit, reason)
		return false
	}
	ok, reason := history.shouldExtend()
	if !ok {
		a.emitStepBudgetStop(step, budget.limit, reason)
		return false
	}
	before, after := budget.extend()
	logs.Infof("extended step budget: before=%d after=%d extension=%d reason=%s", before, after, budget.extensions, reason)
	a.emit(runtimeevent.Event{
		Step:    step,
		Type:    runtimeevent.TypeStepBudgetExtend,
		Message: fmt.Sprintf("extended step budget from %d to %d", before, after),
		Before:  before,
		After:   after,
		Count:   budget.extensions,
		Reason:  reason,
	})
	return true
}

func (a *Agent) emitStepBudgetStop(step int, limit int, reason string) {
	logs.Errorf("agent step budget exhausted: limit=%d reason=%s", limit, reason)
	a.emit(runtimeevent.Event{
		Step:    step,
		Type:    runtimeevent.TypeStepBudgetStop,
		Message: "step budget exhausted",
		Before:  limit,
		Reason:  reason,
	})
}

func (a *Agent) contextExtensionBlockReason(ctx context.Context) string {
	options := a.options.Context
	if !options.CompactEnabled || options.WindowTokens <= 0 || options.CompactForceRatioPercent <= 0 {
		return ""
	}
	before := estimateMessagesTokens(a.messages)
	forceThreshold := options.WindowTokens * options.CompactForceRatioPercent / 100
	if before < forceThreshold {
		return ""
	}
	a.maybeCompact(ctx)
	after := estimateMessagesTokens(a.messages)
	if after >= forceThreshold {
		return fmt.Sprintf("context remains near the force compaction threshold (~%d tokens)", after)
	}
	return ""
}
