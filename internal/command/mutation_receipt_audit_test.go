package command

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestMutationReceiptAuditCommittedDatabaseChangesSurviveAckFailure(t *testing.T) {
	database := h2fixture.Open(t, "mutation-receipt-audit")
	ctx := context.Background()
	if err := database.Register(ctx, "Recipient", "RecipientTrip", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	services := &service.Bundle{
		Users:      &service.UserService{Identity: database, GroupB: database},
		Security:   service.NewSecurityService(&config.Config{}, database),
		Mail:       &service.MailService{DB: database.DB, GroupB: database},
		Notes:      &service.NoteService{DB: database.DB},
		ShadowBans: &service.ShadowBanService{Repo: database},
	}
	caller, err := api.NewContextWithCapabilities("programming", "caller", "CallerTrip", "hash", false, []string{}, []api.Capability{api.AdminCommands, api.ModerationCommands})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ command, arguments, query string }{
		{"register", "Registered RegisteredTrip", "SELECT COUNT(*) FROM names WHERE name='Registered'"},
		{"access", "Grantee USER", "SELECT COUNT(*) FROM trips WHERE trip='Grantee' AND type='USER'"},
		{"note", "a private saved note", "SELECT COUNT(*) FROM notes WHERE trip='CallerTrip'"},
		{"mail", "Recipient delivered later", "SELECT COUNT(*) FROM mail WHERE receiver='RecipientTrip'"},
		{"shadowban", "OfflineTarget", "SELECT COUNT(*) FROM banned_users WHERE name='OfflineTarget'"},
	} {
		t.Run(test.command, func(t *testing.T) {
			engine := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: services, users: map[string]*model.User{}}, sendErr: errors.New("ack failed")}
			result, err := NewAgentCommandGateway(engine).Execute(ctx, caller, test.command, test.arguments)
			var committed int
			if queryErr := database.DB.QueryRowContext(ctx, test.query).Scan(&committed); queryErr != nil || committed != 1 {
				t.Fatalf("fixture did not commit intended change: rows=%d err=%v", committed, queryErr)
			}
			if !result.EffectsCommitted || result.Action == nil || result.Action.Count <= 0 {
				t.Errorf("committed %s lost its source receipt after acknowledgment failure: result=%+v err=%v", test.command, result, err)
			}
			if result.Status == commandgateway.OutcomeSucceeded || result.Delivery != nil || len(result.Messages) != 0 || engine.sends != 1 {
				t.Errorf("failed acknowledgment was claimed successful or replayed: result=%+v sends=%d err=%v", result, engine.sends, err)
			}
		})
	}
}

type mutationExitEngine struct {
	*gatewayEngine
	panicAck    bool
	panicAudit  bool
	cancelAudit context.CancelFunc
	kickErr     error
}

func (e *mutationExitEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	if e.panicAck {
		panic("private acknowledgment panic")
	}
	return e.gatewayEngine.SendChatMessage(author, text, whisper)
}
func (e *mutationExitEngine) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	if e.cancelAudit != nil {
		e.cancelAudit()
	}
	if e.panicAudit {
		panic("private audit panic")
	}
	return 1, nil
}
func (e *mutationExitEngine) KickNick(context.Context, common.NickTarget) error { return e.kickErr }

func TestMutationReceiptAuditFurtherCommittedExits(t *testing.T) {
	database := h2fixture.Open(t, "mutation-receipt-exits")
	services := &service.Bundle{Users: &service.UserService{Identity: database, GroupB: database}, Notes: &service.NoteService{DB: database.DB}, ShadowBans: &service.ShadowBanService{Repo: database}}
	caller, _ := api.NewContextWithCapabilities("room", "caller", "CallerTrip", "hash", false, []string{}, []api.Capability{api.AdminCommands, api.ModerationCommands})
	for _, tc := range []struct {
		name, command, arguments          string
		setup                             string
		query                             string
		panicAck, panicAudit, cancelAudit bool
		kickErr                           error
		wantCount                         int
	}{
		{name: "remove", command: "remove", arguments: "RemoveMe", setup: "register", query: "SELECT COUNT(*) FROM names WHERE name='RemoveMe'", wantCount: 1},
		{name: "purge", command: "notes", arguments: "purge", setup: "note", query: "SELECT COUNT(*) FROM notes WHERE trip='CallerTrip'", wantCount: 1},
		{name: "reverse", command: "unshadowban", arguments: "Target", setup: "shadow", query: "SELECT COUNT(*) FROM banned_users WHERE name='Target'", wantCount: 1},
		{name: "kick failure", command: "shadowban", arguments: "Present", kickErr: errors.New("kick failed"), wantCount: 1},
		{name: "handler panic", command: "note", arguments: "secret", panicAck: true, wantCount: 1},
		{name: "audit panic", command: "note", arguments: "secret", panicAudit: true, wantCount: 1},
		{name: "audit cancellation", command: "note", arguments: "secret", cancelAudit: true, wantCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch tc.setup {
			case "register":
				if err := database.Register(ctx, "RemoveMe", "RemoveTrip", model.REGULAR); err != nil {
					t.Fatal(err)
				}
			case "note":
				if err := services.Notes.Save(ctx, "CallerTrip", "secret"); err != nil {
					t.Fatal(err)
				}
			case "shadow":
				if err := services.ShadowBans.Persist(ctx, repository.ShadowBanRecord{Name: "Target"}); err != nil {
					t.Fatal(err)
				}
			}
			e := &mutationExitEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{bundle: services, users: map[string]*model.User{"Present": {Name: "Present", Trip: "PresentTrip"}}}, sendErr: errors.New("ack failed")}, panicAck: tc.panicAck, panicAudit: tc.panicAudit, kickErr: tc.kickErr}
			if tc.cancelAudit {
				e.cancelAudit = cancel
			}
			got, err := NewAgentCommandGateway(e).Execute(ctx, caller, tc.command, tc.arguments)
			if got.Action == nil || got.Action.Count != tc.wantCount || !got.EffectsCommitted || got.Status != commandgateway.OutcomeUnknown || err != nil {
				t.Fatalf("result=%+v err=%v", got, err)
			}
			if tc.query != "" {
				var remaining int
				if err := database.DB.QueryRowContext(context.Background(), tc.query).Scan(&remaining); err != nil || remaining != 0 {
					t.Fatalf("remaining=%d err=%v", remaining, err)
				}
			}
		})
	}
}

func TestMutationReceiptAuditConcurrentInvocations(t *testing.T) {
	database := h2fixture.Open(t, "mutation-receipt-concurrent")
	services := &service.Bundle{Notes: &service.NoteService{DB: database.DB}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			caller, _ := api.NewContext("room", "caller", fmt.Sprint("Trip", i), "", false, []string{})
			e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: services}, sendErr: errors.New("ack failed")}
			got, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, "note", "private note")
			if err != nil || got.Action == nil || got.Action.Count != 1 {
				t.Errorf("invocation %d result=%+v err=%v", i, got, err)
			}
		}(i)
	}
	wg.Wait()
	var rows int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&rows); err != nil || rows != 8 {
		t.Fatalf("rows=%d err=%v", rows, err)
	}
}

func TestMutationReceiptAuditAFKAndSubscriptionResultSemantics(t *testing.T) {
	caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
	for _, cmd := range []string{"afk", "sub", "unsub"} {
		t.Run(cmd, func(t *testing.T) {
			e := &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller", Trip: "Trip"}}}, sendErr: errors.New("ack failed")}
			if cmd == "unsub" {
				e.SubscribeTrip("Trip")
			}
			got, err := NewAgentCommandGateway(e).Execute(context.Background(), caller, cmd, "away")
			if err != nil || got.Action == nil || got.Action.Count != 1 || got.Status != commandgateway.OutcomeUnknown {
				t.Fatalf("result=%+v err=%v", got, err)
			}
			if cmd == "afk" && len(*e.GetAfkUsers()) != 1 {
				t.Fatal("AFK state missing")
			}
			if cmd == "sub" {
				got, _ = NewAgentCommandGateway(e).Execute(context.Background(), caller, cmd, "")
				if got.Action != nil || got.EffectsCommitted {
					t.Fatalf("duplicate subscription manufactured commit: %+v", got)
				}
			}
		})
	}
}

func TestMutationReceiptAuditSubscriptionSurvivesAckFailure(t *testing.T) {
	engine := &gatewayEngine{commandEngineStub: commandEngineStub{}, sendErr: errors.New("ack failed")}
	caller, err := api.NewContext("programming", "caller", "CallerTrip", "hash", false, []string{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "sub", "")
	if !engine.IsSubscribedTrip("CallerTrip") {
		t.Fatal("fixture did not establish subscription")
	}
	if !result.EffectsCommitted || result.Action == nil || result.Action.Count <= 0 || result.Status == commandgateway.OutcomeSucceeded {
		t.Fatalf("subscription state survived but its receipt did not: result=%+v err=%v", result, err)
	}
}
