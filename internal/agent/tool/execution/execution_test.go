package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

type fake struct {
	name  string
	d     contract.Descriptor
	fn    func(context.Context) (contract.Result, error)
	calls atomic.Int32
}

func (f *fake) Name() string                                        { return f.name }
func (f *fake) Descriptor(api.Context) (contract.Descriptor, error) { return f.d, nil }
func (f *fake) Execute(c context.Context, _ api.Context, _ json.RawMessage) (contract.Result, error) {
	f.calls.Add(1)
	return f.fn(c)
}
func ctx(t *testing.T) api.Context {
	c, e := api.NewContext("room", "nick", "", "", false, []string{})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func desc(t *testing.T, name string, effect contract.Effect, reads, writes []string, idem bool, timeout time.Duration, caps []string) contract.Descriptor {
	d, e := contract.NewDescriptor(name, name, "description", "test", contract.AccessUser, effect, contract.ModelData, json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), caps, nil, idem, timeout, json.RawMessage(`{"type":"string"}`), reads, writes, []string{"never"})
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestExecuteAllContiguousParallelOrderAndBarrier(t *testing.T) {
	c := ctx(t)
	var order atomic.Int32
	mk := func(n string, delay time.Duration) *fake {
		d := desc(t, n, contract.ReadOnly, []string{"r"}, nil, true, 0, nil)
		return &fake{name: n, d: d, fn: func(context.Context) (contract.Result, error) {
			time.Sleep(delay)
			order.Add(1)
			return contract.SuccessResult("", n, "ok"), nil
		}}
	}
	a, b, bar := mk("a", 30*time.Millisecond), mk("b", 1*time.Millisecond), mk("bar", 0)
	bar.d = desc(t, "bar", contract.Action, nil, []string{"r"}, false, 0, nil)
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{a, b, bar}, []string{"a", "b", "bar"})}
	r := ExecuteAll(context.Background(), e, c, []Call{{"1", "a", json.RawMessage(`{}`)}, {"2", "b", json.RawMessage(`{}`)}, {"3", "bar", json.RawMessage(`{}`)}})
	if len(r) != 3 || r[0].ToolName != "a" || r[1].ToolName != "b" {
		t.Fatalf("order: %#v", r)
	}
	if order.Load() != 3 {
		t.Fatal("all calls should run")
	}
}
func TestLedgerDuplicateLimitAndFailure(t *testing.T) {
	l := NewLedger(map[string]int{"x": 1}, 1)
	if l.Reserve("k", "x") != "" || l.Reserve("k", "x") != "DUPLICATE_TOOL_CALL" {
		t.Fatal("duplicate")
	}
	if l.Reserve("k2", "x") != "TOOL_CALL_LIMIT_REACHED" {
		t.Fatal("limit")
	}
	l.Failure("y")
	if l.Reserve("ykey", "y") != "TOOL_DISABLED" {
		t.Fatal("disabled")
	}
}
func TestExecutorStableAuthorizationAndFailures(t *testing.T) {
	c := ctx(t)
	d := desc(t, "x", contract.ReadOnly, []string{"r"}, nil, true, 0, []string{"CAP"})
	f := &fake{name: "x", d: d, fn: func(context.Context) (contract.Result, error) { return contract.Result{}, errors.New("secret") }}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"x"}), Ledger: NewLedger(nil, 0)}
	if e.Execute(context.Background(), c, Call{"1", "x", json.RawMessage(`{}`)}).ErrorCode != "TOOL_NOT_AUTHORIZED" {
		t.Fatal("auth")
	}
	c, _ = api.NewContextWithCapabilities("room", "nick", "", "", false, []string{}, []api.Capability{"CAP"})
	if e.Execute(context.Background(), c, Call{"1", "x", json.RawMessage(`{}`)}).ErrorCode != "TOOL_EXECUTION_FAILED" {
		t.Fatal("failure")
	}
	if e.Execute(context.Background(), c, Call{"2", "unknown", json.RawMessage(`{}`)}).ErrorCode != "TOOL_NOT_ALLOWED" {
		t.Fatal("allow")
	}
}
func TestExecutorTimeoutAndCancellation(t *testing.T) {
	c := ctx(t)
	d := desc(t, "x", contract.ReadOnly, []string{"r"}, nil, true, 5*time.Millisecond, nil)
	f := &fake{name: "x", d: d, fn: func(c context.Context) (contract.Result, error) { <-c.Done(); return contract.Result{}, c.Err() }}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"x"})}
	if e.Execute(context.Background(), c, Call{"1", "x", json.RawMessage(`{}`)}).ErrorCode != "TOOL_TIMEOUT" {
		t.Fatal("timeout")
	}
	q, cancel := context.WithCancel(context.Background())
	cancel()
	if e.Execute(q, c, Call{"2", "x", json.RawMessage(`{}`)}).ErrorCode != "TOOL_BATCH_CANCELLED" {
		t.Fatal("cancel")
	}
}
