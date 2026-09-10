package command

import (
	"context"
	"testing"

	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type nukeSubmitterStub struct {
	*commandEngineStub
	requests []snapshot.RoomSnapshotRequest
	err      error
}

type uncredentialedNukeSubmitterStub struct {
	*commandEngineStub
}

func (s *uncredentialedNukeSubmitterStub) SubmitRoomSnapshot(snapshot.RoomSnapshotRequest) error {
	return nil
}

func (s *nukeSubmitterStub) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	s.requests = append(s.requests, request)
	return s.err
}

func TestNukeRegistrationRequiresCredentialedRoomSnapshotSubmitter(t *testing.T) {
	withoutSubmitter := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(withoutSubmitter); err != nil {
		t.Fatal(err)
	}
	if _, registered := (*withoutSubmitter.GetEnabledCommands())["nuke"]; registered {
		t.Fatal("nuke registered without room snapshot submitter")
	}
	uncredentialed := &uncredentialedNukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	if err := RegisterUserUtilities(uncredentialed); err != nil {
		t.Fatal(err)
	}
	if _, registered := (*uncredentialed.GetEnabledCommands())["nuke"]; registered {
		t.Fatal("nuke registered with only an uncredentialed room snapshot submitter")
	}

	withSubmitter := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	if err := RegisterUserUtilities(withSubmitter); err != nil {
		t.Fatal(err)
	}
	if _, registered := (*withSubmitter.GetEnabledCommands())["nuke"]; !registered {
		t.Fatal("nuke was not registered with room snapshot submitter")
	}
	if got, want := len(*withSubmitter.GetEnabledCommands()), len(*withoutSubmitter.GetEnabledCommands())+1; got != want {
		t.Fatalf("registered commands=%d, want baseline aliases plus canonical nuke (%d)", got, want)
	}
	for alias := range *withSubmitter.GetEnabledCommands() {
		if alias == "nuke" {
			continue
		}
		if _, baseline := (*withoutSubmitter.GetEnabledCommands())[alias]; !baseline {
			t.Fatalf("unexpected nuke registration alias %q", alias)
		}
	}
	registered := (*withSubmitter.GetEnabledCommands())["nuke"].Command(&model.ChatMessage{})
	if role := registered.GetRole(); role == nil || *role != model.MODERATOR {
		t.Fatalf("nuke role=%v, want moderator", role)
	}
	if aliases := registered.GetAliases(); len(aliases) != 1 || aliases[0] != "nuke" {
		t.Fatalf("nuke aliases=%v, want [nuke]", aliases)
	}
}

func TestNukeCommandRejectsMissingRoomWithExactUsage(t *testing.T) {
	engine := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	definition, ok := commandDefinitionFor("nuke")
	if !ok {
		t.Fatal("nuke definition is missing")
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "moderator", Text: "!nuke", IsWhisper: true}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.requests) != 0 || len(engine.chats) != 1 || engine.chats[0] != "moderator|\\n Example: !nuke hotlinks|true" {
		t.Fatalf("requests=%v chats=%v", engine.requests, engine.chats)
	}
}

func TestNukeCommandRejectsPreCancelledContextWithoutSubmission(t *testing.T) {
	engine := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	definition, ok := commandDefinitionFor("nuke")
	if !ok {
		t.Fatal("nuke definition is missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := definition.New(engine, &model.ChatMessage{Name: "moderator", Channel: "source-room", Text: "!nuke hotlinks"}).Execute(ctx)
	if status != model.FAILED || err != context.Canceled || len(engine.requests) != 0 {
		t.Fatalf("status=%v err=%v submissions=%d", status, err, len(engine.requests))
	}
}

func TestNukeCommandExecutionRequiresCredentialedRoomSnapshotSubmitter(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{}}
	definition, ok := commandDefinitionFor("nuke")
	if !ok {
		t.Fatal("nuke definition is missing")
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "moderator", Channel: "source-room", Text: "!nuke hotlinks"}).Execute(context.Background())
	if status != model.FAILED || err == nil || len(engine.chats) != 0 || len(engine.raws) != 0 {
		t.Fatalf("status=%v err=%v chats=%v raws=%v", status, err, engine.chats, engine.raws)
	}
}

func TestNukeCommandRequestsSourceShapedFailureReply(t *testing.T) {
	engine := &nukeSubmitterStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		err:               context.DeadlineExceeded,
	}
	definition, ok := commandDefinitionFor("nuke")
	if !ok {
		t.Fatal("nuke definition is missing")
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "moderator", Channel: "source-room", Text: "!nuke hotlinks"}).Execute(context.Background())
	if status != model.FAILED || err != context.DeadlineExceeded {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.requests) != 1 || engine.requests[0].ReplyMessage != "Unable to complete room operation." {
		t.Fatalf("failure request=%+v", engine.requests)
	}
}

func TestNukeCommandSubmitsNormalizedRemoteSnapshotWithoutImmediateReply(t *testing.T) {
	engine := &nukeSubmitterStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
	definition, ok := commandDefinitionFor("nuke")
	if !ok {
		t.Fatal("nuke definition is missing")
	}

	status, err := definition.New(engine, &model.ChatMessage{
		Name:      "moderator",
		Channel:   "source-room",
		Text:      "!nuke @hotlinks",
		IsWhisper: true,
	}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.chats) != 0 || len(engine.raws) != 0 {
		t.Fatalf("command emitted immediate output: chats=%v raws=%v", engine.chats, engine.raws)
	}
	if len(engine.requests) != 1 {
		t.Fatalf("snapshot submissions=%d, want 1", len(engine.requests))
	}
	request := engine.requests[0]
	if request.WorkflowID == "" || request.Author != "moderator" || request.SourceChannel != "source-room" || request.TargetChannel != "hotlinks" || !request.Whisper || request.Operation == nil {
		t.Fatalf("snapshot request=%+v", request)
	}
}
