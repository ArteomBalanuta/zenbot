package tool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestLifecycleResultSchemaWithholdsMalformedFactsAndPreservesReceipts(t *testing.T) {
	for _, status := range []commandgateway.OutcomeStatus{commandgateway.OutcomeSucceeded, commandgateway.OutcomeUnknown} {
		for _, tc := range []struct {
			name, data string
			valid      bool
		}{
			{"accepted", `{"operation":"restart","scope":"host","requestStatus":"accepted","completionStatus":"not_observed"}`, true},
			{"coalesced", `{"operation":"shutdown","scope":"host","requestStatus":"coalesced","completionStatus":"not_observed"}`, true},
			{"completed", `{"operation":"restart","scope":"host","requestStatus":"accepted","completionStatus":"succeeded"}`, true},
			{"failed", `{"operation":"restart","scope":"host","requestStatus":"accepted","completionStatus":"failed","failureCode":"HOST_LIFECYCLE_FAILED"}`, true},
			{"unbounded history", `{"operation":"restart","scope":"host","requestStatus":"accepted","completionStatus":"not_observed","history":[]}`, false},
			{"missing completion", `{"operation":"restart","scope":"host","requestStatus":"accepted"}`, false},
			{"wrong scope", `{"operation":"restart","scope":"process","requestStatus":"accepted","completionStatus":"not_observed"}`, false},
			{"invented state", `{"operation":"restart","scope":"host","requestStatus":"scheduled","completionStatus":"not_observed"}`, false},
			{"raw error", `{"operation":"restart","scope":"host","requestStatus":"accepted","completionStatus":"failed","failureCode":"credential diagnostic"}`, false},
			{"malformed", `{"operation":`, false},
		} {
			t.Run(string(status)+"/"+tc.name, func(t *testing.T) {
				caller, err := api.NewContextWithCapabilities("room", "caller", "trip", "", false, []string{}, []api.Capability{api.AdminCommands})
				if err != nil {
					t.Fatal(err)
				}
				gateway := &runCommandGatewayStub{result: commandgateway.Execution{Status: status, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 2}, Delivery: &commandgateway.DeliveryReceipt{Count: 1}, DataObserved: true, Data: json.RawMessage(tc.data)}}
				command := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, "restart"), Gateway: gateway}
				result, err := command.Execute(context.Background(), caller, []byte(`{}`))
				if err != nil || result.ActionCount != 2 || result.DeliveryCount != 1 || !result.EffectsCommitted || result.IsError != (status == commandgateway.OutcomeUnknown) || strings.Contains(string(result.Envelope()), "credential diagnostic") {
					t.Fatalf("result=%+v err=%v", result, err)
				}
				if status == commandgateway.OutcomeUnknown && (result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown) {
					t.Fatalf("error evidence changed: %+v", result)
				}
				if strings.Contains(string(result.Envelope()), `"operation"`) != tc.valid {
					t.Fatalf("invalid facts leaked or valid facts lost: %s", result.Envelope())
				}
				descriptor, _ := command.Descriptor(caller)
				payload := []byte(result.Content)
				if result.IsError {
					payload = result.ObservedData
				}
				if len(payload) > 0 && contract.ValidateResult(descriptor.ResultSchema(), payload) != nil {
					t.Fatalf("invalid resulting payload=%s", payload)
				}
			})
		}
	}
}
