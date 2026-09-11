package execution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestExecutorValidatesErrorObservationsWithoutLosingUnknownBarrier(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		valid      bool
	}{
		{"valid", `"observed room"`, true}, {"wrong schema", `{"room":"secret"}`, false}, {"malformed", `"bad`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action := &fake{name: "action", d: desc(t, "action", contract.Action, nil, []string{"state"}, false, 0, nil), fn: func(context.Context) (contract.Result, error) {
				r := contract.ActionErrorResult("", "action", "ACTION_OUTCOME_UNKNOWN", "safe failure", contract.EffectUnknown)
				r.ObservedData = json.RawMessage(tc.data)
				r.ActionCount = 2
				return r, nil
			}}
			e := &Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{"action"}), Ledger: NewLedger(nil, 3)}
			got := e.Execute(context.Background(), ctx(t), Call{"first", "action", json.RawMessage(`{}`)})
			if (len(got.ObservedData) > 0) != tc.valid || got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || got.EffectState != contract.EffectUnknown || got.ActionCount != 2 || got.DeliveryCount != 0 || !strings.Contains(got.Content, "safe failure") || got.VerifiedRoomDelivery() {
				t.Fatalf("result=%+v", got)
			}
			if next := e.Execute(context.Background(), ctx(t), Call{"repeat", "action", json.RawMessage(`{}`)}); !next.IsError || action.calls.Load() != 1 || e.Ledger.UnknownActionCallID() != "first" {
				t.Fatalf("unknown action repeated: %+v", next)
			}
		})
	}
}
