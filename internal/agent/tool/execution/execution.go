package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

type Ledger struct {
	mu          sync.Mutex
	seen        map[string]bool
	counts      map[string]int
	failures    map[string]int
	successful  map[string]bool
	limits      map[string]int
	maxFailures int
}

func NewLedger(limits map[string]int, maxFailures int) *Ledger {
	ownedLimits := make(map[string]int, len(limits))
	for name, limit := range limits {
		ownedLimits[name] = limit
	}
	return &Ledger{seen: map[string]bool{}, counts: map[string]int{}, failures: map[string]int{}, successful: map[string]bool{}, limits: ownedLimits, maxFailures: maxFailures}
}
func (l *Ledger) Reserve(k, n string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen[k] {
		return "DUPLICATE_TOOL_CALL"
	}
	if lim := l.limits[n]; lim > 0 && l.counts[n] >= lim {
		return "TOOL_CALL_LIMIT_REACHED"
	}
	if l.failures[n] >= l.maxFailures && l.maxFailures > 0 {
		return "TOOL_DISABLED"
	}
	l.seen[k] = true
	l.counts[n]++
	return ""
}
func (l *Ledger) Failure(n string) { l.mu.Lock(); l.failures[n]++; l.mu.Unlock() }
func (l *Ledger) Success(n string) { l.mu.Lock(); l.successful[n] = true; l.mu.Unlock() }
func (l *Ledger) Missing(prerequisites []string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, prerequisite := range prerequisites {
		if !l.successful[prerequisite] {
			return true
		}
	}
	return false
}

type Cancellation struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewCancellation(parent context.Context, deadline time.Time) Cancellation {
	ctx, c := context.WithCancel(parent)
	if !deadline.IsZero() {
		var x context.CancelFunc
		ctx, x = context.WithDeadline(ctx, deadline)
		old := c
		c = func() { x(); old() }
	}
	return Cancellation{ctx, c}
}
func (c Cancellation) Context() context.Context { return c.ctx }
func (c Cancellation) Cancel()                  { c.cancel() }

type Executor struct {
	Registry       *tool.Registry
	Ledger         *Ledger
	DefaultTimeout time.Duration
}

var errNilResult = errors.New("nil tool result")

func invoke(t tool.Tool, ctx context.Context, agent api.Context, args json.RawMessage) (contract.Result, error) {
	type outcome struct {
		r   contract.Result
		err error
	}
	result := make(chan outcome, 1)
	go func() {
		var r contract.Result
		var err error
		defer func() {
			if recover() != nil {
				r = contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed")
				err = nil
			}
			result <- outcome{r, err}
		}()
		r, err = t.Execute(ctx, agent, args)
		if err == nil && r.ToolName == "" {
			err = errNilResult
		}
	}()
	select {
	case completed := <-result:
		return completed.r, completed.err
	case <-ctx.Done():
		return contract.Result{}, ctx.Err()
	}
}

func (e *Executor) Execute(ctx context.Context, agent api.Context, c Call) (result contract.Result) {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	observability.Info(ctx, "agent.tool.started", "tool", c.Name, "tool_call_id", c.ID)
	defer func() {
		status := "success"
		if result.IsError {
			status = "error"
		}
		observability.Info(ctx, "agent.tool.completed",
			"tool", c.Name,
			"tool_call_id", c.ID,
			"status", status,
			"error_code", result.ErrorCode,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}()
	defer func() {
		result.CallID = c.ID
		result.ToolName = c.Name
	}()
	if ctx.Err() != nil {
		code := "TOOL_BATCH_CANCELLED"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "TOOL_BATCH_DEADLINE"
		}
		return contract.ErrorResult(c.ID, c.Name, code, "tool batch execution cancelled")
	}
	if e.Registry == nil {
		return contract.ErrorResult(c.ID, c.Name, "UNKNOWN_TOOL", "unknown tool")
	}
	if !e.Registry.Allowed(c.Name) {
		return contract.ErrorResult(c.ID, c.Name, "TOOL_NOT_ALLOWED", "tool is not allowed")
	}
	t, ok := e.Registry.Lookup(c.Name)
	if !ok {
		return contract.ErrorResult(c.ID, c.Name, "UNKNOWN_TOOL", "unknown tool")
	}
	d, err := t.Descriptor(agent)
	if err != nil {
		return contract.ErrorResult(c.ID, c.Name, "INVALID_TOOL_CONTRACT", "invalid tool contract")
	}
	for _, cap := range d.RequiredCapabilities() {
		if !agent.HasCapability(api.Capability(cap)) {
			return contract.ErrorResult(c.ID, c.Name, "TOOL_NOT_AUTHORIZED", "tool is not authorized")
		}
	}
	if err := contract.ValidateArguments(d.Parameters(), c.Arguments); err != nil {
		return contract.ErrorResult(c.ID, c.Name, "INVALID_ARGUMENTS", err.Error())
	}
	if e.Ledger != nil && e.Ledger.Missing(d.RequiredSuccessfulTools()) {
		return contract.ErrorResult(c.ID, c.Name, "MISSING_PREREQUISITE", "required tool must succeed first")
	}
	if e.Ledger != nil {
		if code := e.Ledger.Reserve(Key(c), c.Name); code != "" {
			return contract.ErrorResult(c.ID, c.Name, code, "tool call rejected")
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
	r, err := invoke(t, ctx, agent, c.Arguments)
	if r.IsError {
		if e.Ledger != nil {
			e.Ledger.Failure(c.Name)
		}
		return contract.ErrorResult(c.ID, c.Name, r.ErrorCode, r.Content)
	}
	if err != nil {
		if e.Ledger != nil {
			e.Ledger.Failure(c.Name)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return contract.ErrorResult(c.ID, c.Name, "TOOL_TIMEOUT", "tool timed out")
		}
		if ctx.Err() != nil {
			return contract.ErrorResult(c.ID, c.Name, "TOOL_BATCH_CANCELLED", "tool execution cancelled")
		}
		return contract.ErrorResult(c.ID, c.Name, "TOOL_EXECUTION_FAILED", "tool execution failed")
	}
	if err := contract.ValidateResult(d.ResultSchema(), []byte(r.Content)); err != nil {
		if e.Ledger != nil {
			e.Ledger.Failure(c.Name)
		}
		return contract.ErrorResult(c.ID, c.Name, "INVALID_TOOL_RESULT", "tool result failed validation")
	}
	if e.Ledger != nil {
		e.Ledger.Success(c.Name)
	}
	return r
}
func Safe(d contract.Descriptor) bool {
	return d.IsReadOnly() && d.Idempotent() && len(d.RequiredSuccessfulTools()) == 0 && len(d.ResourceWrites()) == 0 && len(d.ResourceReads()) > 0
}
func Conflict(a, b contract.Descriptor) bool {
	if len(a.ResourceReads()) == 0 || len(b.ResourceReads()) == 0 {
		return true
	}
	for _, x := range a.ResourceWrites() {
		for _, y := range append(b.ResourceReads(), b.ResourceWrites()...) {
			if x == y {
				return true
			}
		}
	}
	for _, x := range b.ResourceWrites() {
		for _, y := range append(a.ResourceReads(), a.ResourceWrites()...) {
			if x == y {
				return true
			}
		}
	}
	return false
}
func ExecuteAll(ctx context.Context, e *Executor, agent api.Context, calls []Call) []contract.Result {
	out := make([]contract.Result, len(calls))
	if len(calls) == 0 {
		return out
	}
	if e == nil || e.Registry == nil {
		for index, call := range calls {
			out[index] = contract.ErrorResult(call.ID, call.Name, "UNKNOWN_TOOL", "unknown tool")
		}
		return out
	}
	stages, err := (BatchPlanner{Registry: e.Registry}).Plan(agent, calls)
	if err != nil {
		for index, call := range calls {
			out[index] = contract.ErrorResult(call.ID, call.Name, "INVALID_TOOL_PROTOCOL", err.Error())
		}
		return out
	}
	cursor := 0
	for _, stage := range stages {
		if !stage.Parallel {
			out[cursor] = e.Execute(ctx, agent, stage.Calls[0])
			cursor++
			continue
		}
		var wg sync.WaitGroup
		for offset, call := range stage.Calls {
			index := cursor + offset
			wg.Add(1)
			go func() {
				defer wg.Done()
				out[index] = e.Execute(ctx, agent, call)
			}()
		}
		wg.Wait()
		cursor += len(stage.Calls)
	}
	return out
}
