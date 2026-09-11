package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestExecutorRetriesFailedReadAndRefreshesSuccessfulRead(t *testing.T) {
	var sequence atomic.Int32
	read := &fake{name: "read", d: desc(t, "read", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		if sequence.Add(1) == 1 {
			return contract.Result{}, errors.New("temporary failure")
		}
		return contract.Result{ToolName: "read", Content: fmt.Sprintf("\"snapshot-%d\"", sequence.Load())}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{read}, []string{"read"}), Ledger: NewLedger(map[string]int{"read": 3}, 2)}
	first := e.Execute(context.Background(), ctx(t), Call{"first", "read", json.RawMessage(`{}`)})
	if first.ErrorCode != "TOOL_EXECUTION_FAILED" {
		t.Fatalf("first result=%#v", first)
	}
	for i := 2; i <= 3; i++ {
		result := e.Execute(context.Background(), ctx(t), Call{fmt.Sprint(i), "read", json.RawMessage(`{}`)})
		if result.IsError || result.Content != fmt.Sprintf("\"snapshot-%d\"", i) {
			t.Fatalf("attempt %d result=%#v", i, result)
		}
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"exhausted", "read", json.RawMessage(`{}`)}); got.ErrorCode != "TOOL_CALL_LIMIT_REACHED" {
		t.Fatalf("budget result=%#v", got)
	}
}

func TestExecutorRepairsPrerequisiteWithoutPoisoningArguments(t *testing.T) {
	pre := &fake{name: "pre", d: desc(t, "pre", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "pre", Content: `"schema"`}, nil
	}}
	d, err := contract.NewDescriptor("dependent", "dependent", "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, contract.SchemaObject(nil, nil, false), nil, []string{"pre"}, true, 0, contract.SchemaString(), []string{"state"}, nil, []string{"never"})
	if err != nil {
		t.Fatal(err)
	}
	dependent := &fake{name: "dependent", d: d, fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "dependent", Content: `"rows"`}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{pre, dependent}, []string{"pre", "dependent"}), Ledger: NewLedger(map[string]int{"dependent": 2}, 1)}
	if got := e.Execute(context.Background(), ctx(t), Call{"early", "dependent", json.RawMessage(`{}`)}); got.ErrorCode != "MISSING_PREREQUISITE" {
		t.Fatalf("prerequisite result=%#v", got)
	}
	if !e.Ledger.Available("dependent") {
		t.Fatal("unexecuted prerequisite rejection disabled the tool")
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"schema", "pre", json.RawMessage(`{}`)}); got.IsError {
		t.Fatal(got)
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"repaired", "dependent", json.RawMessage(`{}`)}); got.IsError || got.Content != `"rows"` {
		t.Fatalf("repaired result=%#v", got)
	}
}

func TestExecutorPreservesCommittedReceiptWhenActionResultIsInvalid(t *testing.T) {
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "action", Content: `{}`, EffectsCommitted: true, DeliveryCount: 2}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(map[string]int{"action": 3}, 3)}
	got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
	if got.ErrorCode != "INVALID_TOOL_RESULT" || !got.EffectsCommitted || got.DeliveryCount != 2 {
		t.Fatalf("committed receipt was lost: %#v", got)
	}
	duplicate := e.Execute(context.Background(), ctx(t), Call{"repeat", "action", json.RawMessage(`{}`)})
	if duplicate.ErrorCode != "DUPLICATE_TOOL_CALL" || action.calls.Load() != 1 {
		t.Fatalf("completed action repeated: result=%#v calls=%d", duplicate, action.calls.Load())
	}
}

func TestExecutorPreservesReceiptsOnActionError(t *testing.T) {
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "action", Content: "second delivery failed", ErrorCode: "DELIVERY_FAILED", IsError: true, EffectsCommitted: true, DeliveryCount: 1}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(nil, 3)}
	got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
	if !got.IsError || !got.EffectsCommitted || got.DeliveryCount != 1 {
		t.Fatalf("partial receipt was lost: %#v", got)
	}
}

func TestExecutorUnknownActionKeepsReportedErrorAndDeliveredOutput(t *testing.T) {
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		result := contract.ActionErrorResult("", "action", "REMOTE_INTERRUPTED", "first output was delivered before disconnect", contract.EffectUnknown)
		result.ActionCount, result.DeliveryCount = 1, 1
		return result, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(nil, 3)}
	got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
	if got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || !got.EffectsCommitted || !strings.Contains(got.Content, "REMOTE_INTERRUPTED") || !strings.Contains(got.Content, "first output was delivered") {
		t.Fatalf("unknown action lost its error evidence: %#v", got)
	}
}

func TestExecutorRejectsConcurrentDuplicateActionWhileFirstIsRunning(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		close(started)
		<-release
		return contract.Result{ToolName: "action", Content: `"done"`, EffectsCommitted: true}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(nil, 3)}
	agent := ctx(t)
	finished := make(chan contract.Result, 1)
	go func() {
		finished <- e.Execute(context.Background(), agent, Call{"first", "action", json.RawMessage(`{}`)})
	}()
	<-started
	duplicate := e.Execute(context.Background(), agent, Call{"repeat", "action", json.RawMessage(`{}`)})
	close(release)
	first := <-finished
	if first.IsError || duplicate.ErrorCode != "DUPLICATE_TOOL_CALL" || duplicate.RelatedCallID != "first" || action.calls.Load() != 1 {
		t.Fatalf("first=%#v duplicate=%#v executions=%d", first, duplicate, action.calls.Load())
	}
}

func TestExecutorKnownPartialActionAllowsDistinctModelSelectedAction(t *testing.T) {
	partial := &fake{name: "partial", d: desc(t, "partial", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.ActionErrorResult("", "partial", "DELIVERY_FAILED", "one effect committed", contract.EffectPartial), nil
	}}
	next := &fake{name: "next", d: desc(t, "next", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "next", Content: `"done"`, EffectsCommitted: true}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{partial, next}, []string{"partial", "next"}), Ledger: NewLedger(nil, 3)}
	first := e.Execute(context.Background(), ctx(t), Call{"partial", "partial", json.RawMessage(`{}`)})
	if !first.IsError || first.EffectState != contract.EffectPartial {
		t.Fatalf("partial result=%#v", first)
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"repeat", "partial", json.RawMessage(`{}`)}); got.ErrorCode != "DUPLICATE_TOOL_CALL" {
		t.Fatalf("partial action was repeated: %#v", got)
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"new", "next", json.RawMessage(`{}`)}); got.IsError || next.calls.Load() != 1 || e.Ledger.UnknownActionCallID() != "" {
		t.Fatalf("distinct action was blocked by known partial receipt: %#v", got)
	}
}

func TestExecutorPartialReceiptCannotBecomeSuccessfulCompletion(t *testing.T) {
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "action", Content: `"only one operation completed"`, EffectState: contract.EffectPartial, EffectsCommitted: true, DeliveryCount: 1}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(nil, 3)}
	got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
	if !got.IsError || got.EffectState != contract.EffectPartial || !got.EffectsCommitted || got.VerifiedRoomDelivery() {
		t.Fatalf("partial action was reported complete: %#v", got)
	}
}

func TestExecutorAmbiguousActionStopsFurtherActionsButAllowsReads(t *testing.T) {
	for _, panicAction := range []bool{false, true} {
		t.Run(fmt.Sprintf("panic=%t", panicAction), func(t *testing.T) {
			action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
				if panicAction {
					panic("possibly committed")
				}
				return contract.Result{}, errors.New("connection lost")
			}}
			later := &fake{name: "later", d: desc(t, "later", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
				return contract.Result{ToolName: "later", Content: `"done"`, EffectsCommitted: true}, nil
			}}
			read := &fake{name: "read", d: desc(t, "read", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
				return contract.Result{ToolName: "read", Content: `"verified"`}, nil
			}}
			e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action, later, read}, []string{"action", "later", "read"}), Ledger: NewLedger(nil, 3)}
			got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
			if got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" {
				t.Fatalf("ambiguous action result=%#v", got)
			}
			if got := e.Execute(context.Background(), ctx(t), Call{"later", "later", json.RawMessage(`{}`)}); got.ErrorCode != "ACTION_NOT_EXECUTED" || later.calls.Load() != 0 {
				t.Fatalf("later action result=%#v executions=%d", got, later.calls.Load())
			}
			if got := e.Execute(context.Background(), ctx(t), Call{"verify", "read", json.RawMessage(`{}`)}); got.IsError || got.Content != `"verified"` {
				t.Fatalf("verification read result=%#v", got)
			}
		})
	}
}

func TestExecuteAllSkipsActionsAfterFailureAndReturnsIndependentReads(t *testing.T) {
	var rejected atomic.Bool
	action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
		if !rejected.Swap(true) {
			return contract.ActionErrorResult("", "action", "COMMAND_REJECTED", "choose another target", contract.EffectNotCommitted), nil
		}
		return contract.Result{ToolName: "action", Content: `"done"`, EffectsCommitted: true}, nil
	}}
	read := &fake{name: "read", d: desc(t, "read", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "read", Content: `"context"`}, nil
	}}
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action, read}, []string{"action", "read"}), Ledger: NewLedger(nil, 3)}
	batch := ExecuteAll(context.Background(), e, ctx(t), []Call{{"first", "action", json.RawMessage(`{}`)}, {"second", "action", json.RawMessage(`{}`)}, {"read", "read", json.RawMessage(`{}`)}})
	if batch[0].ErrorCode != "COMMAND_REJECTED" || batch[1].ErrorCode != "ACTION_NOT_EXECUTED" || batch[2].IsError || action.calls.Load() != 1 {
		t.Fatalf("batch=%#v action calls=%d", batch, action.calls.Load())
	}
	if got := e.Execute(context.Background(), ctx(t), Call{"repair", "action", json.RawMessage(`{}`)}); got.IsError || action.calls.Load() != 2 {
		t.Fatalf("known rejection could not be repaired: %#v calls=%d", got, action.calls.Load())
	}
}

func TestExecuteAllAdmitsSameToolInProviderOrder(t *testing.T) {
	read := &fake{name: "read", d: desc(t, "read", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "read", Content: `"value"`}, nil
	}}
	registry := tool.NewRegistry([]tool.Tool{read}, []string{"read"})
	for i := 0; i < 100; i++ {
		e := &Executor{Registry: registry, Ledger: NewLedger(map[string]int{"read": 1}, 2)}
		batch := ExecuteAll(context.Background(), e, ctx(t), []Call{{"first", "read", json.RawMessage(`{}`)}, {"second", "read", json.RawMessage(`{}`)}})
		if batch[0].IsError || batch[1].ErrorCode != "TOOL_CALL_LIMIT_REACHED" {
			t.Fatalf("run %d admitted out of provider order: %#v", i, batch)
		}
	}
}

func TestExecuteAllParallelizesReadsWithSatisfiedPrerequisite(t *testing.T) {
	pre := &fake{name: "pre", d: desc(t, "pre", contract.ReadOnly, []string{"state"}, nil, true, 0, nil), fn: func(context.Context) (contract.Result, error) {
		return contract.Result{ToolName: "pre", Content: `"schema"`}, nil
	}}
	var started atomic.Int32
	ready := make(chan struct{})
	makeRead := func(name string) *fake {
		d, err := contract.NewDescriptor(name, name, "description", "test", contract.AccessUser, contract.ReadOnly, contract.ModelData, contract.SchemaObject(nil, nil, false), nil, []string{"pre"}, true, time.Second, contract.SchemaString(), []string{"state"}, nil, []string{"never"})
		if err != nil {
			t.Fatal(err)
		}
		return &fake{name: name, d: d, fn: func(ctx context.Context) (contract.Result, error) {
			if started.Add(1) == 2 {
				close(ready)
			}
			select {
			case <-ready:
				return contract.Result{ToolName: name, Content: `"rows"`}, nil
			case <-ctx.Done():
				return contract.Result{}, ctx.Err()
			}
		}}
	}
	one, two := makeRead("one"), makeRead("two")
	e := &Executor{Registry: tool.NewRegistry([]tool.Tool{pre, one, two}, []string{"pre", "one", "two"}), Ledger: NewLedger(nil, 2)}
	if got := e.Execute(context.Background(), ctx(t), Call{"pre", "pre", json.RawMessage(`{}`)}); got.IsError {
		t.Fatal(got)
	}
	batch := ExecuteAll(context.Background(), e, ctx(t), []Call{{"one", "one", json.RawMessage(`{}`)}, {"two", "two", json.RawMessage(`{}`)}})
	if batch[0].IsError || batch[1].IsError {
		t.Fatalf("satisfied independent reads did not overlap: %#v", batch)
	}
}
