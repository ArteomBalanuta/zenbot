package tool_test

import (
	"context"
	"encoding/json"
	"testing"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
)

func TestSaturnVerifiedSilentActionsUseModelDataAndNeedPositiveEvidence(t *testing.T) {
	caller, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan})
	for _, tc := range []struct{ name, args string }{{"kick", `{"nick":"alice"}`}, {"color", `{"nick":"alice","color":"00ff00"}`}, {"overflow", `{"nick":"alice"}`}, {"nuke", `{"room":"remote"}`}, {"resurrect", `{"nick":"alice","source":"remote","destination":"room"}`}} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &runCommandGatewayStub{result: commandgateway.Execution{Status: commandgateway.OutcomeSucceeded, EffectsCommitted: true, Action: &commandgateway.ActionReceipt{Count: 2}}}
			tool := agenttool.SaturnCommand{Definition: agentCommandDefinition(t, tc.name), Gateway: gateway}
			descriptor, err := tool.Descriptor(caller)
			if err != nil || descriptor.ResultMode() != contract.ModelData {
				t.Errorf("mode=%s err=%v", descriptor.ResultMode(), err)
			}
			result, err := tool.Execute(context.Background(), caller, json.RawMessage(tc.args))
			if err != nil || result.IsError || result.ActionCount != 2 || result.DeliveryCount != 0 {
				t.Errorf("result=%+v err=%v", result, err)
			}
			gateway.result.Action = nil
			result, err = tool.Execute(context.Background(), caller, json.RawMessage(tc.args))
			if err != nil || !result.IsError || result.ErrorCode != "UNVERIFIED_ACTION_OUTCOME" {
				t.Fatalf("no evidence result=%+v err=%v", result, err)
			}
		})
	}
}
