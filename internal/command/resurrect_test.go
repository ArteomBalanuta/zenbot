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

type credentialedResurrectFallbackStub struct {
	*resurrectFallbackStub
	credentialedRequests []snapshot.RoomSnapshotRequest
}

type resurrectMove struct {
	from   string
	target string
	to     common.Channel
}

func (s *resurrectFallbackStub) MoveFromServingRoom(_ context.Context, from, target string, to common.Channel) (bool, error) {
	s.moves = append(s.moves, resurrectMove{from: from, target: target, to: to})
	return false, nil
}

func (s *resurrectFallbackStub) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	s.requests = append(s.requests, request)
	return nil
}

func (s *credentialedResurrectFallbackStub) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	s.credentialedRequests = append(s.credentialedRequests, request)
	return nil
}

func TestResurrectRegistrationRequiresBothLiveMoverAndCredentialedSnapshotSubmitter(t *testing.T) {
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

	uncredentialed := &resurrectFallbackStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	if err := RegisterUserUtilities(uncredentialed); err != nil {
		t.Fatal(err)
	}
	assertNoResurrectAliases(t, uncredentialed.commandEngineStub)

	credentialed := &credentialedResurrectFallbackStub{resurrectFallbackStub: &resurrectFallbackStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}}
	if err := RegisterUserUtilities(credentialed); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"move", "recover", "heal", "resurrect"} {
		metadata, ok := (*credentialed.GetEnabledCommands())[alias]
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

func TestResurrectCommandFallsBackToCredentialedSnapshotOnlyWhenNoLiveSourceServesFrom(t *testing.T) {
	engine := &credentialedResurrectFallbackStub{resurrectFallbackStub: &resurrectFallbackStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}}
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
	if len(engine.moves) != 1 || engine.moves[0] != (resurrectMove{from: "source-room", target: "Alice", to: common.Channel("destination-room")}) {
		t.Fatalf("live moves=%+v", engine.moves)
	}
	if len(engine.requests) != 0 {
		t.Fatalf("uncredentialed snapshot submissions=%d, want 0", len(engine.requests))
	}
	if len(engine.credentialedRequests) != 1 {
		t.Fatalf("credentialed snapshot submissions=%d, want 1", len(engine.credentialedRequests))
	}
	request := engine.credentialedRequests[0]
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
