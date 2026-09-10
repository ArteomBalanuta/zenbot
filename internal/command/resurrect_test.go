package command

import (
	"context"
	"reflect"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type resurrectFallbackStub struct {
	*commandEngineStub
	requests []snapshot.RoomSnapshotRequest
	moves    []resurrectMove
}

type resurrectMove struct {
	from   string
	target common.NickTarget
	to     common.Channel
}

func (s *resurrectFallbackStub) MoveFromServingRoom(_ context.Context, from string, target common.NickTarget, to common.Channel) (bool, error) {
	s.moves = append(s.moves, resurrectMove{from: from, target: target, to: to})
	return false, nil
}

func (s *resurrectFallbackStub) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	s.requests = append(s.requests, request)
	return nil
}

func TestResurrectRegistrationRequiresBothLiveMoverAndSnapshotSubmitter(t *testing.T) {
	baseline := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(baseline); err != nil {
		t.Fatal(err)
	}
	assertNoResurrectAliases(t, baseline)

	snapshotOnly := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	if err := RegisterUserUtilities(snapshotOnly); err != nil {
		t.Fatal(err)
	}
	assertNoResurrectAliases(t, snapshotOnly.commandEngineStub)

	both := &resurrectFallbackStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	if err := RegisterUserUtilities(both); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"move", "recover", "heal", "resurrect"} {
		metadata, ok := (*both.GetEnabledCommands())[alias]
		if !ok {
			t.Fatalf("missing resurrect alias %q", alias)
		}
		command := metadata.Command(&model.ChatMessage{})
		if role := command.GetRole(); role == nil || *role != model.MODERATOR {
			t.Fatalf("alias %q role=%v", alias, role)
		}
	}
}

func assertNoResurrectAliases(t *testing.T, engine *commandEngineStub) {
	t.Helper()
	for _, alias := range []string{"move", "recover", "heal", "resurrect"} {
		if _, registered := (*engine.GetEnabledCommands())[alias]; registered {
			t.Fatalf("resurrect alias %q registered without both capabilities", alias)
		}
	}
}

func TestResurrectCommandFallsBackToSnapshotOnlyWhenNoLiveSourceServesFrom(t *testing.T) {
	engine := &resurrectFallbackStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	definition, ok := commandDefinitionFor("move")
	if !ok {
		t.Fatal("resurrect definition is missing")
	}

	status, err := definition.New(engine, &model.ChatMessage{
		Name:      "moderator",
		Channel:   "invoking-room",
		Text:      "!move @Alice source-room destination-room",
		IsWhisper: true,
	}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.moves) != 1 || engine.moves[0] != (resurrectMove{from: "source-room", target: common.NickTarget("Alice"), to: common.Channel("destination-room")}) {
		t.Fatalf("live moves=%+v", engine.moves)
	}
	if len(engine.requests) != 1 {
		t.Fatalf("snapshot submissions=%d, want 1", len(engine.requests))
	}
	request := engine.requests[0]
	if request.WorkflowID == "" || request.Author != "moderator" || !request.Whisper || request.SourceChannel != "source-room" || request.TargetChannel != "source-room" || request.DestinationChannel != "destination-room" {
		t.Fatalf("snapshot request=%+v", request)
	}
	if operation := reflect.TypeOf(request.Operation); operation == nil || operation.Name() != "KickOrResurrectOperation" {
		t.Fatalf("snapshot operation=%v, want KickOrResurrectOperation", operation)
	}
	if len(engine.raws) != 0 || len(engine.chats) != 0 {
		t.Fatalf("command emitted immediate output: raws=%v chats=%v", engine.raws, engine.chats)
	}
}
