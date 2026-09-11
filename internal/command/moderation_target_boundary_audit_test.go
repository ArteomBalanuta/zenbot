package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/model"
)

// Keep real core serialization at the last boundary; the ordinary command fake
// alone cannot detect a second normalization after a target has been resolved.
type targetBoundaryAuditEngine struct {
	*commandEngineStub
	wire         *core.EngineImpl
	afterLookup  func()
	operationErr error
}

func (e *targetBoundaryAuditEngine) GetActiveUserByName(name string) *model.User {
	user := e.commandEngineStub.GetActiveUserByName(name)
	if e.afterLookup != nil {
		e.afterLookup()
		e.afterLookup = nil
	}
	return user
}

func (e *targetBoundaryAuditEngine) KickNick(ctx context.Context, target common.NickTarget) error {
	return e.wire.KickNick(ctx, target)
}
func (e *targetBoundaryAuditEngine) BanNick(ctx context.Context, target common.NickTarget) error {
	if e.operationErr != nil {
		return e.operationErr
	}
	return e.wire.BanNick(ctx, target)
}
func (e *targetBoundaryAuditEngine) OverflowNick(ctx context.Context, target common.NickTarget) error {
	if e.operationErr != nil {
		return e.operationErr
	}
	return e.wire.OverflowNick(ctx, target)
}

func newTargetBoundaryAuditEngine(users map[string]*model.User) *targetBoundaryAuditEngine {
	return &targetBoundaryAuditEngine{
		commandEngineStub: &commandEngineStub{users: users},
		wire:              &core.EngineImpl{OutMessageQueue: make(chan string, 8)},
	}
}

func targetBoundaryAuditNicks(t *testing.T, engine *targetBoundaryAuditEngine) []string {
	t.Helper()
	var nicks []string
	for len(engine.wire.OutMessageQueue) > 0 {
		var payload struct {
			Nick string `json:"nick"`
		}
		if err := json.Unmarshal([]byte(<-engine.wire.OutMessageQueue), &payload); err != nil {
			t.Fatal(err)
		}
		nicks = append(nicks, payload.Nick)
	}
	return nicks
}

func TestModerationTargetBoundaryAuditCanonicalNameSurvivesCore(t *testing.T) {
	for _, input := range []string{"!kick @@alice", "!kick -c @alice"} {
		t.Run(input, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{
				"alice": {Name: "alice", Hash: "plain"}, "@alice": {Name: "@alice", Hash: "marked"},
			})
			definition, _ := commandDefinitionFor("kick")
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: input}).Execute(context.Background())
			nicks := targetBoundaryAuditNicks(t, engine)
			if status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "@alice" {
				t.Fatalf("resolved identity changed before transport: status=%s err=%v nicks=%v", status, err, nicks)
			}
		})
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowResolveActiveCanonicalUser(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		t.Run(canonical, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " @merc"}).Execute(context.Background())
			nicks := targetBoundaryAuditNicks(t, engine)
			if status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "Merc" {
				t.Fatalf("active canonical target not retained: status=%s err=%v nicks=%v", status, err, nicks)
			}
		})
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowRejectAbsentOrExtraTargets(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		for _, arguments := range []string{"absent", "Merc another"} {
			t.Run(canonical+"/"+arguments, func(t *testing.T) {
				engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
				definition, _ := commandDefinitionFor(canonical)
				status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " " + arguments}).Execute(context.Background())
				if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || len(nicks) != 0 {
					t.Fatalf("invalid selection sent a request: status=%s err=%v nicks=%v", status, err, nicks)
				}
			})
		}
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowHonorCancellationAroundLookup(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		for _, phase := range []string{"before lookup", "after lookup"} {
			t.Run(canonical+"/"+phase, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
				if phase == "before lookup" {
					cancel()
				} else {
					engine.afterLookup = cancel
				}
				definition, _ := commandDefinitionFor(canonical)
				status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " @Merc"}).Execute(ctx)
				if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || !errors.Is(err, context.Canceled) || len(nicks) != 0 || len(engine.chats) != 0 {
					t.Fatalf("cancelled selection had effects: status=%s err=%v nicks=%v chats=%v", status, err, nicks, engine.chats)
				}
			})
		}
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowPropagateSendFailure(t *testing.T) {
	sendErr := errors.New("send failed")
	for _, canonical := range []string{"ban", "overflow"} {
		t.Run(canonical, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
			engine.operationErr = sendErr
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " Merc"}).Execute(context.Background())
			if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || !errors.Is(err, sendErr) || len(nicks) != 0 || len(engine.chats) != 0 {
				t.Fatalf("send failure fabricated confirmation: status=%s err=%v nicks=%v chats=%v", status, err, nicks, engine.chats)
			}
		})
	}
}
