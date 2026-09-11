package command

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type moderationReviewEngine struct{ *commandEngineStub }

func (e *moderationReviewEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	result, err := request.Operation.Apply(snapshot.RoomSnapshotContext{
		TargetChannel: request.TargetChannel,
		SendRaw: func(raw string) error {
			e.raws = append(e.raws, raw)
			return nil
		},
	}, snapshot.Snapshot{Users: []*model.User{{Name: "innocent"}}})
	if request.OnComplete != nil {
		request.OnComplete(result)
	}
	return err
}

func TestAgentCommandGatewayModerationReviewRejectsCommandsOutsideMatchingMute(t *testing.T) {
	database := h2fixture.Open(t, "moderation-review-authority")
	seedMailGroupCCommandRecipient(t, database.DB)
	identity := &identityFake{}
	shadowBans := &shadowBanRepositoryStub{}
	engine := &moderationReviewEngine{commandEngineStub: &commandEngineStub{
		users: map[string]*model.User{
			"reviewed": {Name: "reviewed", Trip: "reviewed-trip", Hash: "reviewed-hash"},
		},
		bundle: &service.Bundle{
			Mail:       &service.MailService{DB: database.DB, GroupB: database},
			Notes:      &service.NoteService{DB: database.DB},
			Users:      &service.UserService{Identity: identity},
			ShadowBans: &service.ShadowBanService{Repo: shadowBans},
		},
	}}
	if err := engine.bundle.Notes.Save("creator-trip", "creator private note"); err != nil {
		t.Fatal(err)
	}
	caller, err := api.NewContextWithModerationTarget(
		"room", "bot", "creator-trip", "", false,
		[]string{"reviewed", "innocent"},
		[]api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands},
		"reviewed",
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		command, arguments string
	}{
		{command: "nuke", arguments: "other-room"},
		{command: "lock", arguments: "on"},
		{command: "notes", arguments: "purge"},
		{command: "register", arguments: "new-name new-trip"},
		{command: "mail", arguments: "Merc private message"},
		{command: "shadowban", arguments: "reviewed"},
		{command: "kick", arguments: "@reviewed"},
	} {
		result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, test.command, test.arguments)
		if err == nil || result.Status != commandgateway.OutcomeRejected || result.EffectsCommitted {
			t.Fatalf("%s result=%#v err=%v", test.command, result, err)
		}
	}

	if len(engine.raws) != 0 || len(engine.chats) != 0 || len(identity.registered) != 0 || len(shadowBans.persisted) != 0 {
		t.Fatalf("review commands mutated state: raws=%#v chats=%#v registered=%#v shadowBans=%#v", engine.raws, engine.chats, identity.registered, shadowBans.persisted)
	}
	notes, err := engine.bundle.Notes.List("creator-trip")
	if err != nil || len(notes) != 1 || notes[0] != "creator private note" {
		t.Fatalf("creator notes changed: notes=%#v err=%v", notes, err)
	}
	var mailRows int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM mail").Scan(&mailRows); err != nil || mailRows != 0 {
		t.Fatalf("review queued mail: rows=%d err=%v", mailRows, err)
	}
}

func TestAgentCommandGatewayModerationReviewAdmitsOnlyNormalizedMatchingMute(t *testing.T) {
	engine := &moderationReviewEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{
		"Reviewed": {Name: "Reviewed", Hash: "reviewed-hash"},
	}}}
	caller, err := api.NewContextWithModerationTarget("room", "bot", "creator-trip", "", false, []string{"Reviewed"}, []api.Capability{api.ModerationCommands}, "reviewed")
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "mute", "@REVIEWED")
	if err != nil || result.Status != commandgateway.OutcomeSucceeded || !result.EffectsCommitted {
		t.Fatalf("matching mute result=%#v err=%v", result, err)
	}
	if len(engine.raws) != 1 || engine.raws[0] != `{"cmd":"mute","nick":"Reviewed"}` {
		t.Fatalf("mute effects=%#v", engine.raws)
	}

	result, err = NewAgentCommandGateway(engine).Execute(context.Background(), caller, "mute", "someone-else")
	if err == nil || result.Status != commandgateway.OutcomeRejected || len(engine.raws) != 1 {
		t.Fatalf("retargeted mute result=%#v err=%v raws=%#v", result, err, engine.raws)
	}
}

func TestAgentCommandGatewayNukeRequiresPermanentBanOutsideModerationReview(t *testing.T) {
	for _, test := range []struct {
		name         string
		capabilities []api.Capability
		wantStatus   commandgateway.OutcomeStatus
		wantEffects  int
	}{
		{name: "moderator denied", capabilities: []api.Capability{api.ModerationCommands}, wantStatus: commandgateway.OutcomeRejected},
		{name: "creator admitted", capabilities: []api.Capability{api.PermanentBan}, wantStatus: commandgateway.OutcomeSucceeded, wantEffects: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &moderationReviewEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
			caller, err := api.NewContextWithCapabilities("room", "caller", "trip", "", false, []string{}, test.capabilities)
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, "nuke", "other-room")
			if result.Status != test.wantStatus || len(engine.raws) != test.wantEffects {
				t.Fatalf("result=%#v err=%v raws=%#v", result, err, engine.raws)
			}
		})
	}
}
