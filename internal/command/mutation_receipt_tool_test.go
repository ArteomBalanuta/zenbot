package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/sqlitefixture"
)

type receiptRoomDirectory struct{}

func (receiptRoomDirectory) FindRoomUsers(room string) (tool.RoomUserSnapshot, bool) {
	return tool.RoomUserSnapshot{Room: room, Users: []string{"caller"}}, true
}

func TestMutationReceiptToolExecutorPreservesUnknownAndBlocksReplay(t *testing.T) {
	db := sqlitefixture.Open(t, "mutation-receipt-tool")
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Notes: &service.NoteService{DB: db.DB}}}, sendErr: errors.New("private transport credential")}
	definition, _ := commandcatalog.AgentEntry("note")
	save := tool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(e)}
	read := tool.RoomUsers{Directory: receiptRoomDirectory{}}
	executor := &execution.Executor{Registry: tool.NewRegistry([]tool.Tool{save, read}, []string{save.Name(), read.Name()}), Ledger: execution.NewLedger(nil, 5)}
	caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
	args := json.RawMessage(`{"text":"private note body"}`)
	got := executor.Execute(context.Background(), caller, execution.Call{ID: "first", Name: save.Name(), Arguments: args})
	if !got.IsError || got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || got.EffectState != contract.EffectUnknown || !got.EffectsCommitted || got.ActionCount != 1 || got.DeliveryCount != 0 {
		t.Fatalf("lost committed receipt: %+v", got)
	}
	if strings.Contains(got.Content, "private") || strings.Contains(got.Content, "credential") {
		t.Fatalf("private error exposed: %+v", got)
	}
	for i, args := range []json.RawMessage{args, json.RawMessage(`{"text":"a different note"}`)} {
		blocked := executor.Execute(context.Background(), caller, execution.Call{ID: []string{"repeat", "different"}[i], Name: save.Name(), Arguments: args})
		if !blocked.IsError || blocked.EffectState != contract.EffectNotStarted {
			t.Fatalf("action not blocked: %+v", blocked)
		}
	}
	observed := executor.Execute(context.Background(), caller, execution.Call{ID: "read", Name: read.Name(), Arguments: json.RawMessage(`{}`)})
	if observed.IsError || !strings.Contains(observed.Content, "caller") {
		t.Fatalf("read blocked: %+v", observed)
	}
	var rows int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&rows); err != nil || rows != 1 || e.sends != 1 {
		t.Fatalf("rows=%d sends=%d err=%v", rows, e.sends, err)
	}
}

func TestMutationReceiptShadowRemovalOnlyCountsChangedRows(t *testing.T) {
	db := sqlitefixture.Open(t, "mutation-receipt-shadow-rows")
	s := &service.ShadowBanService{Repo: db}
	ctx, receipt := common.WithMutationRecorder(context.Background())
	if changed, err := s.Remove(ctx, "absent"); err != nil || changed != 0 || receipt.Count() != 0 {
		t.Fatalf("empty targeted delete: changed=%d err=%v count=%d", changed, err, receipt.Count())
	}
	if changed, err := s.RemoveAll(ctx); err != nil || changed != 0 || receipt.Count() != 0 {
		t.Fatalf("empty bulk delete: changed=%d err=%v count=%d", changed, err, receipt.Count())
	}
	if err := s.Persist(ctx, repository.ShadowBanRecord{Name: "One"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.Remove(ctx, "One"); err != nil || changed != 1 || receipt.Count() != 2 {
		t.Fatalf("targeted delete: changed=%d err=%v count=%d", changed, err, receipt.Count())
	}
	if err := s.Persist(ctx, repository.ShadowBanRecord{Name: "Two"}); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.RemoveAll(ctx); err != nil || changed != 1 || receipt.Count() != 4 {
		t.Fatalf("bulk delete: changed=%d err=%v count=%d", changed, err, receipt.Count())
	}
}

func TestMutationReceiptToolExecutorDoesNotReplaySuccessfulCommit(t *testing.T) {
	db := sqlitefixture.Open(t, "mutation-receipt-success-tool")
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Notes: &service.NoteService{DB: db.DB}}}}
	definition, _ := commandcatalog.AgentEntry("note")
	save := tool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(e)}
	executor := &execution.Executor{Registry: tool.NewRegistry([]tool.Tool{save}, []string{save.Name()}), Ledger: execution.NewLedger(nil, 3)}
	caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
	args := json.RawMessage(`{"text":"private note body"}`)
	got := executor.Execute(context.Background(), caller, execution.Call{ID: "first", Name: save.Name(), Arguments: args})
	if got.IsError || !got.EffectsCommitted || got.ActionCount != 2 || got.DeliveryCount != 1 {
		t.Fatalf("source plus transport counts changed: %+v", got)
	}
	got = executor.Execute(context.Background(), caller, execution.Call{ID: "repeat", Name: save.Name(), Arguments: args})
	if got.ErrorCode != "DUPLICATE_TOOL_CALL" {
		t.Fatalf("repeat permitted: %+v", got)
	}
	var rows int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&rows); err != nil || rows != 1 || e.sends != 1 {
		t.Fatalf("rows=%d sends=%d err=%v", rows, e.sends, err)
	}
}

func TestMutationReceiptToolExecutorPreservesKnownPartialSource(t *testing.T) {
	db := sqlitefixture.Open(t, "mutation-receipt-partial-tool")
	e := &mutationExitEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: db}}, users: map[string]*model.User{"Present": {Name: "Present", Trip: "ExactTrip"}}}}, kickErr: repository.ErrNotFound}
	definition, _ := commandcatalog.AgentEntry("shadowban")
	action := tool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(e)}
	executor := &execution.Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{action.Name()}), Ledger: execution.NewLedger(nil, 3)}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "Trip", "", false, []string{}, []api.Capability{api.ModerationCommands})
	args := json.RawMessage(`{"mode":"exact","target":"Present"}`)
	got := executor.Execute(context.Background(), caller, execution.Call{ID: "first", Name: action.Name(), Arguments: args})
	if !got.IsError || got.ErrorCode != "NOT_FOUND" || got.EffectState != contract.EffectPartial || !got.EffectsCommitted || got.ActionCount != 1 || got.DeliveryCount != 0 {
		t.Fatalf("partial receipt lost: %+v", got)
	}
	got = executor.Execute(context.Background(), caller, execution.Call{ID: "repeat", Name: action.Name(), Arguments: args})
	if got.ErrorCode != "DUPLICATE_TOOL_CALL" {
		t.Fatalf("partial commit replayed: %+v", got)
	}
	var rows int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM banned_users WHERE name='Present'").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
}

func TestMutationReceiptToolExecutorUnknownCommitNeverBecomesRetryable(t *testing.T) {
	db := sqlitefixture.Open(t, "mutation-receipt-unknown-commit-tool")
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: receiptIdentity{IdentityRepository: db, sourceErr: repository.ErrCommitOutcomeUnknown}}}}, sendErr: errors.New("ack failed")}
	definition, _ := commandcatalog.AgentEntry("register")
	action := tool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(e)}
	executor := &execution.Executor{Registry: tool.NewRegistry([]tool.Tool{action}, []string{action.Name()}), Ledger: execution.NewLedger(nil, 3)}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "Trip", "", false, []string{}, []api.Capability{api.AdminCommands, api.ModerationCommands})
	args := json.RawMessage(`{"nick":"Name","trip":"ExactTrip"}`)
	got := executor.Execute(context.Background(), caller, execution.Call{ID: "first", Name: action.Name(), Arguments: args})
	if got.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || got.EffectState != contract.EffectUnknown || got.EffectsCommitted || got.ActionCount != 0 {
		t.Fatalf("unknown commit manufactured receipt or retry: %+v", got)
	}
	got = executor.Execute(context.Background(), caller, execution.Call{ID: "repeat", Name: action.Name(), Arguments: args})
	if got.ErrorCode != "ACTION_NOT_EXECUTED" || e.sends != 1 {
		t.Fatalf("unknown commit replayed: %+v sends=%d", got, e.sends)
	}
}
