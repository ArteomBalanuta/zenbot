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

func TestExecutorChargesInvalidArgumentsAgainstPerToolCallBudget(t *testing.T) {
	c := ctx(t)
	parameters := contract.SchemaObject(map[string]json.RawMessage{"value": contract.SchemaString()}, []string{"value"}, false)
	d, err := contract.NewDescriptor("read", "read", "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 0, contract.SchemaString(), []string{"r"}, nil, []string{"never"})
	if err != nil {
		t.Fatal(err)
	}
	read := &fake{name: "read", d: d, fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "read", Content: `"ok"`}, nil
	}}
	ledger := NewLedger(map[string]int{"read": 2}, 2)
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{read}, []string{read.Name()}), Ledger: ledger}

	if got := executor.Execute(context.Background(), c, Call{"invalid", read.Name(), json.RawMessage(`{"value":7}`)}); got.ErrorCode != "INVALID_ARGUMENTS" {
		t.Fatalf("invalid result=%#v", got)
	}
	if got := executor.Execute(context.Background(), c, Call{"corrected", read.Name(), json.RawMessage(`{"value":"ok"}`)}); got.IsError {
		t.Fatalf("corrected result=%#v", got)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	if ledger.Available(read.Name()) {
		t.Fatal("invalid model attempt did not consume the per-tool call budget")
	}
}

func TestExecutorChargesDuplicateCallsAgainstPerToolCallBudget(t *testing.T) {
	c := ctx(t)
	read := &fake{name: "read", d: desc(t, "read", contract.ReadOnly, []string{"r"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "read", Content: `"ok"`}, nil
	}}
	ledger := NewLedger(map[string]int{"read": 2}, 2)
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{read}, []string{read.Name()}), Ledger: ledger}

	if got := executor.Execute(context.Background(), c, Call{"first", read.Name(), json.RawMessage(`{}`)}); got.IsError {
		t.Fatalf("first result=%#v", got)
	}
	if got := executor.Execute(context.Background(), c, Call{"duplicate", read.Name(), json.RawMessage(`{}`)}); got.ErrorCode != "DUPLICATE_TOOL_CALL" {
		t.Fatalf("duplicate result=%#v", got)
	}
	if read.calls.Load() != 1 {
		t.Fatalf("tool executions=%d, want 1", read.calls.Load())
	}
	if ledger.Available(read.Name()) {
		t.Fatal("duplicate model attempt did not consume the per-tool call budget")
	}
}

func TestExecutorChargesMissingPrerequisitesAgainstFailureBudget(t *testing.T) {
	c := ctx(t)
	d, err := contract.NewDescriptor("dependent", "dependent", "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, contract.SchemaObject(nil, nil, false), nil, []string{"pre"}, true, 0, contract.SchemaString(), []string{"r"}, nil, []string{"never"})
	if err != nil {
		t.Fatal(err)
	}
	dependent := &fake{name: "dependent", d: d, fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "dependent", Content: `"ok"`}, nil
	}}
	ledger := NewLedger(map[string]int{"dependent": 2}, 1)
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{dependent}, []string{dependent.Name()}), Ledger: ledger}

	if got := executor.Execute(context.Background(), c, Call{"missing", dependent.Name(), json.RawMessage(`{}`)}); got.ErrorCode != "MISSING_PREREQUISITE" {
		t.Fatalf("missing prerequisite result=%#v", got)
	}
	if dependent.calls.Load() != 0 {
		t.Fatalf("tool executions=%d, want 0", dependent.calls.Load())
	}
	if ledger.Available(dependent.Name()) {
		t.Fatal("missing prerequisite did not consume the failure budget")
	}
}

func TestExecutorRejectsUnverifiedActionOutcome(t *testing.T) {
	c := ctx(t)
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "action", Content: `"ok"`}, nil
	}}
	ledger := NewLedger(map[string]int{"action": 2}, 1)
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{action.Name()}), Ledger: ledger}

	got := executor.Execute(context.Background(), c, Call{"call", action.Name(), json.RawMessage(`{}`)})
	if got.ErrorCode != "UNVERIFIED_ACTION_OUTCOME" {
		t.Fatalf("result=%#v", got)
	}
	if ledger.Available(action.Name()) {
		t.Fatal("unverified action outcome did not consume the failure budget")
	}
}

func TestExecutorRejectsUnverifiedRoomDelivery(t *testing.T) {
	c := ctx(t)
	d, err := contract.NewDescriptor(
		"delivery", "delivery", "description", "test", contract.AccessUser,
		contract.Action, contract.RoomDelivery, contract.SchemaObject(nil, nil, false),
		nil, nil, false, 0, contract.SchemaString(), nil, []string{"room_delivery"}, []string{"never"},
	)
	if err != nil {
		t.Fatal(err)
	}
	delivery := &fake{name: "delivery", d: d, fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "delivery", Content: `"ok"`, EffectsCommitted: true}, nil
	}}
	ledger := NewLedger(map[string]int{"delivery": 2}, 1)
	executor := &Executor{Registry: tool.NewRegistry([]tool.Tool{delivery}, []string{delivery.Name()}), Ledger: ledger}

	got := executor.Execute(context.Background(), c, Call{"call", delivery.Name(), json.RawMessage(`{}`)})
	if got.ErrorCode != "UNVERIFIED_ROOM_DELIVERY" {
		t.Fatalf("result=%#v", got)
	}
	if ledger.Available(delivery.Name()) {
		t.Fatal("unverified room delivery did not consume the failure budget")
	}
}
