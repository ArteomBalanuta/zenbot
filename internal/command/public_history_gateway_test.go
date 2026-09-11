package command

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type cancelHistoryQueries struct {
	repository.UserQueryRepository
	cancel context.CancelFunc
}

func (q cancelHistoryQueries) LastOnline(ctx context.Context, target string) (repository.LastOnlineRecord, error) {
	record, err := q.UserQueryRepository.LastOnline(ctx, target)
	q.cancel()
	return record, err
}

func TestPublicHistoryGatewayAndToolsExposeSafeAmbiguity(t *testing.T) {
	db := h2fixture.Open(t, "history-ambiguity-gateway")
	if _, err := db.DB.Exec(`INSERT INTO messages(name,trip,message,created_on) VALUES('SelectorSecret','other','name fact',1),('someone','SelectorSecret','trip fact',2)`); err != nil {
		t.Fatal(err)
	}
	caller, _ := api.NewContext("room", "caller", "", "", false, nil)
	entry, _ := commandcatalog.AgentEntry("lastonline")
	for _, svc := range []*service.UserService{{Queries: db}, {LastSeen: db}} {
		engine := &gatewayEngine{authorized: true, commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: svc}}}
		gateway := NewAgentCommandGateway(engine)
		got, err := gateway.Execute(context.Background(), caller, "lastonline", "SelectorSecret")
		if err != nil || got.Status != commandgateway.OutcomeStatus("AMBIGUOUS") || got.EffectsCommitted || got.DataObserved || got.Action != nil || got.Delivery != nil {
			t.Errorf("gateway=%+v err=%v", got, err)
		}
		for _, native := range []bool{true, false} {
			var result contract.Result
			if native {
				result, err = (tool.SaturnCommand{Definition: entry, Gateway: gateway}).Execute(context.Background(), caller, []byte(`{"nick":"SelectorSecret"}`))
			} else {
				result, err = (tool.RunCommand{Gateway: gateway}).Execute(context.Background(), caller, []byte(`{"command":"lastonline","arguments":"SelectorSecret"}`))
			}
			envelope := string(result.Envelope())
			if err != nil || result.ErrorCode != "AMBIGUOUS_TARGET" || result.EffectState != contract.EffectNotCommitted || result.EffectsCommitted || result.ActionCount != 0 || result.DeliveryCount != 0 || len(result.ObservedData) != 0 || !strings.Contains(envelope, "nickname and trip") || !strings.Contains(envelope, "nicks") {
				t.Errorf("native=%v result=%s err=%v", native, envelope, err)
			}
			for _, private := range []string{"SelectorSecret", "name fact", "trip fact", "SQL", "ambiguous public history target"} {
				if strings.Contains(envelope, private) {
					t.Errorf("raw source data leaked: %s", envelope)
				}
			}
		}
		if engine.sends != 0 {
			t.Errorf("ambiguity sent %d messages", engine.sends)
		}
	}
}

func TestPublicHistoryGatewayCancellationOverridesAmbiguity(t *testing.T) {
	db := h2fixture.Open(t, "history-ambiguity-cancel")
	if _, err := db.DB.Exec(`INSERT INTO messages(name,trip,message,created_on) VALUES('target','other','name fact',1),('someone','target','trip fact',2)`); err != nil {
		t.Fatal(err)
	}
	caller, _ := api.NewContext("room", "caller", "", "", false, nil)
	entry, _ := commandcatalog.AgentEntry("lastonline")
	for _, native := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		engine := &gatewayEngine{authorized: true, commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Queries: cancelHistoryQueries{UserQueryRepository: db, cancel: cancel}}}}}
		gateway := NewAgentCommandGateway(engine)
		var result contract.Result
		var err error
		if native {
			result, err = (tool.SaturnCommand{Definition: entry, Gateway: gateway}).Execute(ctx, caller, []byte(`{"nick":"target"}`))
		} else {
			result, err = (tool.RunCommand{Gateway: gateway}).Execute(ctx, caller, []byte(`{"command":"lastonline","arguments":"target"}`))
		}
		cancel()
		if err != nil || result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || !strings.Contains(result.Content, "do not repeat") || engine.sends != 0 {
			t.Errorf("native=%v result=%+v err=%v sends=%d", native, result, err, engine.sends)
		}
	}
}
