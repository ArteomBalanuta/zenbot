package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

type Call struct {
	ID, Name  string
	Arguments json.RawMessage
}

func FromLLM(c llm.LlmToolCall) Call {
	return Call{c.ID(), c.Name(), json.RawMessage(c.RawArguments())}
}

func Key(c Call) string { return c.Name + ":" + string(contract.CanonicalJSON(c.Arguments)) }

func ValidateBatchIdentity(calls []Call) error {
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			return fmt.Errorf("tool call ID must not be blank")
		}
		if strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool call name must not be blank for %s", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("duplicate tool call ID: %s", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

type Executor struct {
	Registry *tool.Registry
	Ledger   *Ledger
	// Exposed is the invocation's provider manifest. Nil uses registry policy;
	// an empty nonnil map permits no model calls.
	Exposed        map[string]bool
	DefaultTimeout time.Duration
}

type preparedCall struct {
	call       Call
	tool       tool.Tool
	descriptor contract.Descriptor
	rejection  *contract.Result
	admitted   bool
}

func notStarted(c Call, code, message string) contract.Result {
	return contract.ActionErrorResult(c.ID, c.Name, code, message, contract.EffectNotStarted)
}

// prepare admits calls in source order before workers start. Validation
// failures consume attempts, but not execution failure budgets.
func (e *Executor) prepare(agent api.Context, c Call) preparedCall {
	p := preparedCall{call: c}
	reject := func(code, message string) preparedCall {
		r := notStarted(c, code, message)
		p.rejection = &r
		return p
	}
	if err := ValidateBatchIdentity([]Call{c}); err != nil {
		return reject("INVALID_TOOL_PROTOCOL", err.Error())
	}
	if e == nil || e.Registry == nil {
		return reject("UNKNOWN_TOOL", "unknown tool; choose an available tool")
	}
	if !e.Registry.Allowed(c.Name) {
		return reject("TOOL_NOT_ALLOWED", "tool is not allowed; choose an available tool")
	}
	registered, ok := e.Registry.Lookup(c.Name)
	if !ok {
		return reject("UNKNOWN_TOOL", "unknown tool; choose an available tool")
	}
	d, err := registered.Descriptor(agent)
	if err != nil {
		return reject("INVALID_TOOL_CONTRACT", "invalid tool contract; choose another tool")
	}
	p.tool, p.descriptor = registered, d
	if e.Exposed != nil && !e.Exposed[c.Name] {
		return reject("TOOL_NOT_ALLOWED", "tool is not in the current invocation's manifest; choose an available tool or explain the limitation")
	}
	if d.InternalFallback() {
		return reject("TOOL_NOT_ALLOWED", "tool is not model-callable")
	}
	for _, capability := range d.RequiredCapabilities() {
		if !agent.HasCapability(api.Capability(capability)) {
			return reject("TOOL_NOT_AUTHORIZED", "tool is not authorized")
		}
	}
	if e.Ledger != nil {
		if code := e.Ledger.admit(c); code != "" {
			return reject(code, "tool attempt was rejected by the request budget or call identity; use another available tool")
		}
		p.admitted = true
	}
	if err := contract.ValidateArguments(d.Parameters(), c.Arguments); err != nil {
		return reject("INVALID_ARGUMENTS", err.Error())
	}
	return p
}

var errNilResult = errors.New("nil tool result")
var errToolPanic = errors.New("tool execution panicked")

func invokeDirect(t tool.Tool, ctx context.Context, agent api.Context, args json.RawMessage) (result contract.Result, err error) {
	defer func() {
		if recover() != nil {
			err = errToolPanic
		}
	}()
	result, err = t.Execute(ctx, agent, args)
	if err == nil && result.ToolName == "" {
		err = errNilResult
	}
	return result, err
}

func invokeRead(t tool.Tool, ctx context.Context, agent api.Context, args json.RawMessage) (contract.Result, error) {
	type outcome struct {
		r   contract.Result
		err error
	}
	result := make(chan outcome, 1)
	go func() {
		r, err := invokeDirect(t, ctx, agent, args)
		result <- outcome{r, err}
	}()
	select {
	case completed := <-result:
		return completed.r, completed.err
	case <-ctx.Done():
		return contract.Result{}, ctx.Err()
	}
}

func (e *Executor) Execute(ctx context.Context, agent api.Context, c Call) contract.Result {
	return e.executePrepared(ctx, agent, e.prepare(agent, c))
}

func (e *Executor) executePrepared(ctx context.Context, agent api.Context, p preparedCall) (result contract.Result) {
	if ctx == nil {
		ctx = context.Background()
	}
	c, d := p.call, p.descriptor
	started, invoked := time.Now(), false
	observability.Info(ctx, "agent.tool.started", "tool", c.Name, "tool_call_id", c.ID)
	defer func() {
		result.CallID, result.ToolName = c.ID, c.Name
		if e != nil && e.Ledger != nil && p.admitted {
			e.Ledger.finish(c, d, result, invoked)
		}
		status := "success"
		if result.IsError {
			status = "error"
		}
		observability.Info(ctx, "agent.tool.completed", "tool", c.Name, "tool_call_id", c.ID,
			"status", status, "error_code", result.ErrorCode, "duration_ms", time.Since(started).Milliseconds())
	}()
	if p.rejection != nil {
		return *p.rejection
	}
	if err := ctx.Err(); err != nil {
		code := "TOOL_BATCH_CANCELLED"
		if errors.Is(err, context.DeadlineExceeded) {
			code = "TOOL_BATCH_DEADLINE"
		}
		return notStarted(c, code, "tool batch execution cancelled before this call started")
	}
	if e.Ledger != nil {
		if missing := e.Ledger.missing(d.RequiredSuccessfulTools()); len(missing) > 0 {
			return notStarted(c, "MISSING_PREREQUISITE", "required tools must succeed first: "+strings.Join(missing, ", ")+"; then retry this call")
		}
		if rejection := e.Ledger.start(c, d); rejection != nil {
			return *rejection
		}
	}
	timeout := d.Timeout()
	if timeout <= 0 {
		timeout = e.DefaultTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	invoked = true
	var r contract.Result
	var err error
	if d.Effect() == contract.Action {
		// Actions stop before returning; tool implementations own cooperative
		// deadlines so cancellation cannot leave an unobserved background effect.
		r, err = invokeDirect(p.tool, ctx, agent, c.Arguments)
	} else {
		r, err = invokeRead(p.tool, ctx, agent, c.Arguments)
	}
	if d.Effect() == contract.Action {
		r = normalizeActionResult(r, err)
		if r.IsError {
			return r
		}
	} else if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return r.WithError("TOOL_TIMEOUT", "tool timed out; a later attempt may retry")
		}
		if ctx.Err() != nil {
			return r.WithError("TOOL_BATCH_CANCELLED", "tool execution cancelled")
		}
		return r.WithError("TOOL_EXECUTION_FAILED", "tool execution failed; a later attempt may retry")
	} else if r.IsError {
		return r.WithError(r.ErrorCode, r.Content)
	}
	if err := contract.ValidateResult(d.ResultSchema(), []byte(r.Content)); err != nil {
		return r.WithError("INVALID_TOOL_RESULT", "tool result failed validation")
	}
	if d.ResultMode() == contract.RoomDelivery && !r.VerifiedRoomDelivery() {
		return r.WithError("UNVERIFIED_ROOM_DELIVERY", "room delivery was not verified; inspect the effect receipt before taking further action")
	}
	return r
}

func normalizeActionResult(r contract.Result, err error) contract.Result {
	// Positive receipts outrank an omitted state. Explicit UNKNOWN remains
	// unknown even when some effects were observed.
	if r.EffectsCommitted || r.ActionCount > 0 || r.DeliveryCount > 0 {
		r.EffectsCommitted = true
		if r.EffectState != contract.EffectUnknown {
			if r.IsError || err != nil || r.EffectState == contract.EffectPartial {
				r.EffectState = contract.EffectPartial
			} else {
				r.EffectState = contract.EffectCommitted
			}
		}
	}
	if r.EffectState == contract.EffectUnknown || (r.EffectState == "" && !r.EffectsCommitted) {
		r.EffectState = contract.EffectUnknown
		message := "action outcome is unknown; actions are blocked for this turn. Read-only tools remain available to inspect its outcome"
		if r.ErrorCode != "" && r.ErrorCode != "ACTION_OUTCOME_UNKNOWN" {
			message += "; reported error: " + r.ErrorCode
		}
		if r.Content != "" {
			message += "; " + r.Content
		}
		return r.WithError("ACTION_OUTCOME_UNKNOWN", message)
	}
	if err != nil {
		return r.WithError("TOOL_EXECUTION_FAILED", "action failed; consult effect state and receipts before choosing the next call")
	}
	if r.IsError {
		return r.WithError(r.ErrorCode, r.Content)
	}
	if r.EffectState == contract.EffectPartial {
		return r.WithError("PARTIAL_ACTION_OUTCOME", "the action completed only partially; "+r.Content)
	}
	if !r.EffectsCommitted {
		return r.WithError("UNVERIFIED_ACTION_OUTCOME", "action completion was not verified")
	}
	return r
}
