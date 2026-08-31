package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/llm"
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
	Registry *tool.Registry
	Ledger   *Ledger
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

func (e *Executor) Execute(ctx context.Context, agent api.Context, c Call) contract.Result {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if d.Timeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.Timeout())
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
	for i := 0; i < len(calls); {
		t, ok := e.Registry.Find(agent, calls[i].Name)
		var d contract.Descriptor
		if ok {
			d, _ = t.Descriptor(agent)
		}
		if !ok || !Safe(d) {
			out[i] = e.Execute(ctx, agent, calls[i])
			i++
			continue
		}
		j := i + 1
		for j < len(calls) {
			nt, nok := e.Registry.Find(agent, calls[j].Name)
			if !nok {
				break
			}
			nd, er := nt.Descriptor(agent)
			if er != nil || !Safe(nd) || Conflict(d, nd) {
				break
			}
			j++
		}
		var wg sync.WaitGroup
		for k := i; k < j; k++ {
			wg.Add(1)
			go func(k int) { defer wg.Done(); out[k] = e.Execute(ctx, agent, calls[k]) }(k)
		}
		wg.Wait()
		i = j
	}
	return out
}
