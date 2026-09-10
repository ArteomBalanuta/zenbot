package execution

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestExecutorRejectsInvalidDuplicateLimitNilAndPanic(t *testing.T) {
	c := ctx(t)
	mk := func(name string, fn func(context.Context) (contract.Result, error)) *fake {
		return &fake{name: name, d: desc(t, name, contract.ReadOnly, []string{"r"}, nil, true, 0, nil), fn: fn}
	}
	nilTool := mk("nil", func(context.Context) (contract.Result, error) { return contract.Result{}, nil })
	panicTool := mk("panic", func(context.Context) (contract.Result, error) { panic("secret") })
	invalid := mk("invalid", func(context.Context) (contract.Result, error) {
		return contract.SuccessResult("", "invalid", "ok"), nil
	})
	invalid.d = func() contract.Descriptor {
		d := desc(t, "invalid", contract.ReadOnly, []string{"r"}, nil, true, 0, nil)
		return d
	}()
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{nilTool, panicTool, invalid}, []string{"nil", "panic", "invalid"}), Ledger: NewLedger(map[string]int{"nil": 1}, 0)}
	if r := e.Execute(context.Background(), c, Call{"1", "nil", json.RawMessage(`{}`)}); r.ErrorCode != "TOOL_EXECUTION_FAILED" {
		t.Fatalf("nil=%s", r.ErrorCode)
	}
	if r := e.Execute(context.Background(), c, Call{"2", "panic", json.RawMessage(`{}`)}); r.ErrorCode != "TOOL_EXECUTION_FAILED" {
		t.Fatalf("panic=%s", r.ErrorCode)
	}
	if r := e.Execute(context.Background(), c, Call{"3", "invalid", json.RawMessage(`{"x":1}`)}); r.ErrorCode != "INVALID_ARGUMENTS" {
		t.Fatalf("args=%s", r.ErrorCode)
	}
	if r := e.Execute(context.Background(), c, Call{"4", "nil", json.RawMessage(`{}`)}); r.ErrorCode != "DUPLICATE_TOOL_CALL" {
		t.Fatalf("duplicate=%s", r.ErrorCode)
	}
	if Conflict(desc(t, "a", contract.ReadOnly, []string{"r"}, nil, true, 0, nil), desc(t, "b", contract.ReadOnly, []string{"r"}, nil, true, 0, nil)) {
		t.Fatal("readers of same resource may run together")
	}
}

func TestExecutorEnforcesSuccessfulPrerequisitesAndCountsErrorResults(t *testing.T) {
	c := ctx(t)
	preParams := contract.SchemaObject(map[string]json.RawMessage{"again": contract.SchemaString()}, nil, true)
	preDesc, err := contract.NewDescriptor("pre", "pre", "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, preParams, nil, nil, true, 0, json.RawMessage(`{"type":"string"}`), []string{"r"}, nil, []string{"never"})
	if err != nil {
		t.Fatal(err)
	}
	pre := &fake{name: "pre", d: preDesc, fn: func(context.Context) (contract.Result, error) {
		return contract.ErrorResult("", "pre", "EXPECTED", "rejected"), nil
	}}
	dependentDesc, err := contract.NewDescriptor("dependent", "dependent", "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`), nil, []string{"pre"}, true, 0, json.RawMessage(`{"type":"string"}`), []string{"r"}, nil, []string{"never"})
	if err != nil {
		t.Fatal(err)
	}
	dependent := &fake{name: "dependent", d: dependentDesc, fn: func(context.Context) (contract.Result, error) {
		return contract.SuccessResult("", "dependent", "ok"), nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{pre, dependent}, []string{"pre", "dependent"}), Ledger: NewLedger(nil, 1)}
	if r := e.Execute(context.Background(), c, Call{"1", "dependent", json.RawMessage(`{}`)}); r.ErrorCode != "MISSING_PREREQUISITE" {
		t.Fatalf("got %s", r.ErrorCode)
	}
	if r := e.Execute(context.Background(), c, Call{"2", "pre", json.RawMessage(`{}`)}); r.ErrorCode != "EXPECTED" {
		t.Fatalf("got %s", r.ErrorCode)
	}
	if r := e.Execute(context.Background(), c, Call{"3", "pre", json.RawMessage(`{"again":"x"}`)}); r.ErrorCode != "TOOL_DISABLED" {
		t.Fatalf("got %s", r.ErrorCode)
	}
}

func TestExecutorDoesNotStartAlreadyCancelledCall(t *testing.T) {
	c := ctx(t)
	var calls atomic.Int32
	f := &fake{name: "cancelled", d: desc(t, "cancelled", contract.ReadOnly, []string{"r"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		calls.Add(1)
		return contract.SuccessResult("", "cancelled", "ok"), nil
	}}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{f}, []string{"cancelled"})}
	if r := e.Execute(parent, c, Call{"1", "cancelled", json.RawMessage(`{}`)}); r.ErrorCode != "TOOL_BATCH_CANCELLED" {
		t.Fatalf("got %s", r.ErrorCode)
	}
	if calls.Load() != 0 {
		t.Fatal("cancelled call must not start")
	}
}
