package execution

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

type contextDeniedTool struct{ *fake }

func (*contextDeniedTool) Authorized(api.Context) bool { return false }

func TestExecutorRejectsToolDeniedByInvocationPolicyBeforeExecution(t *testing.T) {
	base := &fake{
		name: "scoped_action",
		d:    desc(t, "scoped_action", contract.Action, nil, []string{"state"}, false, 0, nil),
		fn: func(context.Context) (contract.Result, error) {
			return contract.ActionSuccessResult("", "scoped_action", map[string]any{}, 1), nil
		},
	}
	denied := &contextDeniedTool{fake: base}
	registry := tool.NewRegistry([]tool.Tool{denied}, []string{denied.Name()})
	executor := &Executor{Registry: registry, Exposed: map[string]bool{denied.Name(): true}}
	caller, err := api.NewContextWithModerationTarget("room", "bot", "creator", "", false, []string{"alice"}, []api.Capability{api.ModerationCommands}, "alice")
	if err != nil {
		t.Fatal(err)
	}

	result := executor.Execute(context.Background(), caller, Call{ID: "call-1", Name: denied.Name(), Arguments: json.RawMessage(`{}`)})
	if result.ErrorCode != "TOOL_NOT_AUTHORIZED" || result.EffectState != contract.EffectNotStarted || base.calls.Load() != 0 {
		t.Fatalf("result=%#v calls=%d", result, base.calls.Load())
	}
}
