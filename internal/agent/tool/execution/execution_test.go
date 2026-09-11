package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestExecutorLogsToolLifecycleWithoutArgumentsOrResult(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	f := &fake{name: "lookup", d: desc(t, "lookup", contract.ReadOnly, []string{"rows"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{CallID: "call-1", ToolName: "lookup", Content: `"private-value"`}, nil
	}}
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"lookup"})}
	requestCtx := observability.WithRequest(context.Background(), observability.Request{ID: "tool-request"})
	result := executor.Execute(requestCtx, ctx(t), Call{"call-1", "lookup", json.RawMessage(`{}`)})
	if result.IsError {
		t.Fatalf("tool failed: %#v", result)
	}

	logged := output.String()
	for _, expected := range []string{"agent.tool.started", "agent.tool.completed", "request_id=tool-request", "tool=lookup", "tool_call_id=call-1", "status=success"} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("log %q does not contain %q", logged, expected)
		}
	}
	if strings.Contains(logged, "private-value") {
		t.Fatalf("tool payload leaked into log: %q", logged)
	}
}

func TestExecutorRejectsInternalFallbackEvenWhenItsNameIsInjected(t *testing.T) {
	descriptor, err := contract.NewDescriptor(
		"legacy_fallback", "Legacy fallback", "Internal executor-only fallback.", "test",
		contract.AccessUser, contract.ReadOnly, contract.ModelData,
		contract.SchemaObject(nil, nil, false), nil, nil, true, time.Second,
		contract.SchemaObject(nil, nil, true), []string{"test"}, nil,
		[]string{"Do not expose this provider."}, contract.WithPrimaryIntent("room_presence"), contract.WithInternalFallback(),
	)
	if err != nil {
		t.Fatal(err)
	}
	fallback := &fake{name: "legacy_fallback", d: descriptor, fn: func(context.Context) (contract.Result, error) {
		return contract.SuccessResult("", "legacy_fallback", map[string]any{"unexpected": true}), nil
	}}
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{fallback}, []string{fallback.Name()})}

	result := executor.Execute(context.Background(), ctx(t), Call{"injected", fallback.Name(), json.RawMessage(`{}`)})
	if result.ErrorCode != "TOOL_NOT_ALLOWED" || fallback.calls.Load() != 0 {
		t.Fatalf("result=%#v calls=%d", result, fallback.calls.Load())
	}
}

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

func TestExecutorCancelledActionDoesNotReturnBeforeActionStops(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	action := &fake{
		name: "action",
		d:    desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil),
		fn: func(ctx context.Context) (contract.Result, error) {
			close(started)
			<-release
			close(finished)
			return contract.Result{}, ctx.Err()
		},
	}
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"})}
	agentCtx := ctx(t)
	requestCtx, cancel := context.WithCancel(context.Background())
	result := make(chan contract.Result, 1)
	go func() {
		result <- executor.Execute(requestCtx, agentCtx, Call{"call-1", "action", json.RawMessage(`{}`)})
	}()

	<-started
	cancel()
	select {
	case got := <-result:
		t.Fatalf("Execute returned before action stopped: %#v", got)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	got := <-result
	select {
	case <-finished:
	default:
		t.Fatal("Execute returned before action signaled completion")
	}
	if got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" {
		t.Fatalf("ErrorCode=%q, want ACTION_OUTCOME_UNKNOWN", got.ErrorCode)
	}
}

func TestExecutorUsesConfiguredDefaultTimeoutWhenDescriptorHasNone(t *testing.T) {
	c := ctx(t)
	d := desc(t, "x", contract.ReadOnly, []string{"r"}, nil, true, 0, nil)
	f := &fake{name: "x", d: d, fn: func(c context.Context) (contract.Result, error) {
		select {
		case <-c.Done():
			return contract.Result{}, c.Err()
		case <-time.After(50 * time.Millisecond):
			return contract.SuccessResult("", "x", "late"), nil
		}
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"x"}), DefaultTimeout: 5 * time.Millisecond}
	if got := e.Execute(context.Background(), c, Call{"1", "x", json.RawMessage(`{}`)}); got.ErrorCode != "TOOL_TIMEOUT" {
		t.Fatalf("result=%#v", got)
	}
}

func TestExecutorBindsResultIdentityToOriginatingCall(t *testing.T) {
	c := ctx(t)
	f := &fake{
		name: "lookup",
		d:    desc(t, "lookup", contract.ReadOnly, []string{"rows"}, nil, true, 0, nil),
		fn: func(context.Context) (contract.Result, error) {
			return contract.Result{CallID: "wrong-call", ToolName: "wrong-tool", Content: `"ok"`}, nil
		},
	}
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"lookup"})}
	result := executor.Execute(context.Background(), c, Call{ID: "call-7", Name: "lookup", Arguments: json.RawMessage(`{}`)})
	if result.IsError {
		t.Fatalf("Execute returned error: %#v", result)
	}
	if result.CallID != "call-7" || result.ToolName != "lookup" {
		t.Fatalf("result identity=(%q, %q), want (%q, %q)", result.CallID, result.ToolName, "call-7", "lookup")
	}
}

func TestValidateBatchIdentityRejectsBlankAndDuplicateCallIDs(t *testing.T) {
	tests := []struct {
		name  string
		calls []Call
	}{
		{name: "blank id", calls: []Call{{ID: " ", Name: "lookup"}}},
		{name: "blank name", calls: []Call{{ID: "one", Name: " "}}},
		{name: "duplicate id", calls: []Call{{ID: "same", Name: "lookup"}, {ID: "same", Name: "other"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateBatchIdentity(tc.calls); err == nil {
				t.Fatalf("ValidateBatchIdentity(%#v) succeeded; want protocol error", tc.calls)
			}
		})
	}
	if err := ValidateBatchIdentity([]Call{{ID: "one", Name: "lookup"}, {ID: "two", Name: "lookup"}}); err != nil {
		t.Fatalf("unique call identities rejected: %v", err)
	}
}
