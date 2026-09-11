package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type remoteMsgChannelEngine struct {
	*commandEngineStub
	requests []snapshot.RoomSnapshotRequest
	err      error
}

func (e *remoteMsgChannelEngine) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.requests = append(e.requests, request)
	return e.err
}

func (e *remoteMsgChannelEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	e.requests = append(e.requests, request)
	return e.err
}

func TestMsgChannelRemoteCapability(t *testing.T) {
	t.Run("capability-bearing engine submits one remote workflow", func(t *testing.T) {
		engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
		definition, _ := commandDefinitionFor("msgroom")

		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom other hello"}).Execute(context.Background())

		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(engine.requests) != 1 {
			t.Fatalf("remote requests=%d, want 1", len(engine.requests))
		}
		if len(engine.chats) != 0 || len(engine.raws) != 0 {
			t.Fatalf("direct side effects chats=%v raws=%v", engine.chats, engine.raws)
		}
	})

	t.Run("engine without capability fails closed", func(t *testing.T) {
		engine := &commandEngineStub{}
		definition, _ := commandDefinitionFor("msgroom")

		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom other hello"}).Execute(context.Background())

		if status != model.FAILED || err == nil {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(engine.chats) != 0 || len(engine.raws) != 0 {
			t.Fatalf("side effects chats=%v raws=%v", engine.chats, engine.raws)
		}
	})
}

func TestListRemoteCapabilitySubmitsCredentialedSnapshot(t *testing.T) {
	original := listWorkflowID
	listWorkflowID = func() (string, error) { return "list-0123456789abcdef", nil }
	t.Cleanup(func() { listWorkflowID = original })

	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	definition, _ := commandDefinitionFor("list")
	message := &model.ChatMessage{Name: "alice", Channel: "programming", IsWhisper: true, Text: "!list lounge"}

	status, err := definition.New(engine, message).Execute(context.Background())

	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.requests) != 1 {
		t.Fatalf("requests=%d, want 1", len(engine.requests))
	}
	request := engine.requests[0]
	if request.WorkflowID != "list-0123456789abcdef" || request.Author != "alice" || request.SourceChannel != "programming" || request.TargetChannel != "lounge" || !request.Whisper || request.Operation == nil {
		t.Fatalf("request=%+v", request)
	}
}

func TestMsgChannelRemoteKeepsLocalPath(t *testing.T) {
	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	definition, _ := commandDefinitionFor("msgroom")

	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom programming hello"}).Execute(context.Background())

	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.requests) != 0 {
		t.Fatalf("remote requests=%d, want 0", len(engine.requests))
	}
	if len(engine.chats) != 1 || engine.chats[0] != "alice |anonymous mail from: ?programming message: hello|false" {
		t.Fatalf("chats=%v", engine.chats)
	}
}

func TestMsgChannelRemoteRequest(t *testing.T) {
	original := msgChannelWorkflowID
	msgChannelWorkflowID = func() (string, error) { return "0123456789abcdef", nil }
	t.Cleanup(func() { msgChannelWorkflowID = original })

	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	definition, _ := commandDefinitionFor("msgchannel")
	message := &model.ChatMessage{Name: "alice", Channel: "different-inbound-room", IsWhisper: true, Text: "!msgchannel ?other? image ![](https://example.test/image.png)"}
	status, err := definition.New(engine, message).Execute(context.Background())

	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(engine.requests) != 1 {
		t.Fatalf("requests=%d, want 1", len(engine.requests))
	}
	request := engine.requests[0]
	if request.WorkflowID != "0123456789abcdef" || request.SourceChannel != "programming" || request.TargetChannel != "other?" || !request.Whisper || request.Author != "alice" || request.ReplyMessage == "" {
		t.Fatalf("request=%+v", request)
	}
	if request.RemoteMessage != "image ![](https://example.test/image.png)\\n anonymous mail from: ?programming" {
		t.Fatalf("message=%q", request.RemoteMessage)
	}
}

func TestMsgChannelRemoteCancellation(t *testing.T) {
	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}}
	definition, _ := commandDefinitionFor("msgroom")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom other hello"}).Execute(ctx)

	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.requests) != 0 || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v requests=%v chats=%v", status, err, engine.requests, engine.chats)
	}
}

func TestMsgChannelRemoteSubmissionFailure(t *testing.T) {
	submitErr := errors.New("submission failed")
	engine := &remoteMsgChannelEngine{commandEngineStub: &commandEngineStub{}, err: submitErr}
	definition, _ := commandDefinitionFor("msgroom")

	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom other hello"}).Execute(context.Background())

	if status != model.FAILED || !errors.Is(err, submitErr) || len(engine.requests) != 1 || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v requests=%v chats=%v", status, err, engine.requests, engine.chats)
	}
}

func TestMsgChannelRegistration(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	for _, alias := range []string{"msgchannel", "msgroom"} {
		metadata, ok := (*engine.GetEnabledCommands())[alias]
		if !ok {
			t.Fatalf("%q was not registered", alias)
		}
		command := metadata.Command(&model.ChatMessage{Name: "alice", Text: "!" + alias + " programming hello"})
		if command.GetRole() == nil || *command.GetRole() != model.REGULAR {
			t.Fatalf("%q role=%v, want REGULAR", alias, command.GetRole())
		}
		adapter, ok := command.(*legacyAdapter)
		if !ok {
			t.Fatalf("%q command=%T, want *legacyAdapter", alias, command)
		}
		if _, ok := adapter.def.New(engine, adapter.msg).(*msgChannelCommand); !ok {
			t.Fatalf("%q concrete command=%T, want *msgChannelCommand", alias, adapter.def.New(engine, adapter.msg))
		}
	}
}

type deniedMsgChannelEngine struct{ *commandEngineStub }

func (e *deniedMsgChannelEngine) IsUserAuthorized(*model.User, *model.Role) bool {
	e.authorizationCalls++
	return false
}

func TestMsgChannelDispatchAuthorization(t *testing.T) {
	t.Run("allowed author reaches concrete command", func(t *testing.T) {
		engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}
		if err := RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!msgroom programming hello"})
		if err != nil {
			t.Fatal(err)
		}
		listener.NewUserChatListener(engine).NotifyContext(context.Background(), string(payload))
		if engine.authorizationCalls != 1 {
			t.Fatalf("authorization calls=%d, want 1", engine.authorizationCalls)
		}
		if len(engine.chats) != 1 {
			t.Fatalf("chats=%v, want one concrete command send", engine.chats)
		}
	})

	t.Run("unauthorized author receives existing denial without command send", func(t *testing.T) {
		engine := &deniedMsgChannelEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}}
		if err := RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!msgchannel programming hello"})
		if err != nil {
			t.Fatal(err)
		}
		listener.NewUserChatListener(engine).Notify(string(payload))
		if engine.authorizationCalls != 1 {
			t.Fatalf("authorization calls=%d, want 1", engine.authorizationCalls)
		}
		if len(engine.chats) != 1 || engine.chats[0] != "alice| you are not authorized to run: msgchannel command.|false" {
			t.Fatalf("chats=%v, want existing authorization denial only", engine.chats)
		}
	})
}

func TestMsgChannelLocalDelivery(t *testing.T) {
	cases := []struct {
		name, alias, text, want string
		whisper                 bool
	}{
		{
			name:  "source current-room vector",
			alias: "msgchannel",
			text:  "!msgchannel programming test message",
			want:  "alice |anonymous mail from: ?programming message: test message|false",
		},
		{
			name:    "msgroom removes one leading room marker and preserves whisper",
			alias:   "msgroom",
			text:    "!msgroom ?programming   hello   world   ",
			want:    "alice |anonymous mail from: ?programming message: hello   world|true",
			whisper: true,
		},
		{
			name:  "literal image marker puts provenance after rendered body",
			alias: "msgchannel",
			text:  "!msgchannel programming image ![](https://example.test/image.png)   ",
			want:  "alice |image ![](https://example.test/image.png)\\n anonymous mail from: ?programming|false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &commandEngineStub{}
			definition, ok := commandDefinitionFor(tc.alias)
			if !ok {
				t.Fatalf("missing %q definition", tc.alias)
			}
			message := &model.ChatMessage{Name: "alice", Text: tc.text, IsWhisper: tc.whisper}
			status, err := definition.New(engine, message).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if len(engine.chats) != 1 || engine.chats[0] != tc.want {
				t.Fatalf("chats=%v, want [%q]", engine.chats, tc.want)
			}
			if len(engine.raws) != 0 {
				t.Fatalf("raws=%v, want no raw sends", engine.raws)
			}
		})
	}
}

func TestMsgChannelFailuresAndRemoteCapability(t *testing.T) {
	t.Run("missing room or body replies with source usage", func(t *testing.T) {
		for _, text := range []string{"!msgchannel", "!msgchannel programming"} {
			t.Run(text, func(t *testing.T) {
				engine := &commandEngineStub{}
				definition, _ := commandDefinitionFor("msgchannel")
				status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: text}).Execute(context.Background())
				if err != nil || status != model.FAILED {
					t.Fatalf("status=%v err=%v", status, err)
				}
				want := "alice| Example: !msgroom your-room your message|false"
				if len(engine.chats) != 1 || engine.chats[0] != want {
					t.Fatalf("chats=%v, want [%q]", engine.chats, want)
				}
			})
		}
	})

	t.Run("all question mark room replies blank-room failure", func(t *testing.T) {
		engine := &commandEngineStub{}
		definition, _ := commandDefinitionFor("msgchannel")
		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgchannel ? ?"}).Execute(context.Background())
		if err != nil || status != model.FAILED {
			t.Fatalf("status=%v err=%v", status, err)
		}
		want := "alice|Room name cannot be blank.|false"
		if len(engine.chats) != 1 || engine.chats[0] != want {
			t.Fatalf("chats=%v, want [%q]", engine.chats, want)
		}
	})

	t.Run("remote room requires snapshot capability without outbound chat", func(t *testing.T) {
		engine := &commandEngineStub{}
		definition, _ := commandDefinitionFor("msgroom")
		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgroom other-room hello"}).Execute(context.Background())
		if status != model.FAILED || err == nil || err.Error() != "room snapshot submitter is not configured" {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(engine.chats) != 0 || len(engine.raws) != 0 {
			t.Fatalf("remote side effects chats=%v raws=%v", engine.chats, engine.raws)
		}
	})
}

type msgChannelSendErrorEngine struct {
	*commandEngineStub
	err error
}

func (e *msgChannelSendErrorEngine) SendChatMessage(author, text string, whisper bool) (string, error) {
	e.chats = append(e.chats, author+"|"+text+"|"+boolString(whisper))
	return "", e.err
}

func TestMsgChannelCancellationAndSendError(t *testing.T) {
	t.Run("cancelled context sends nothing", func(t *testing.T) {
		engine := &commandEngineStub{}
		definition, _ := commandDefinitionFor("msgchannel")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgchannel programming hello"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(engine.chats) != 0 {
			t.Fatalf("chats=%v, want none", engine.chats)
		}
	})

	t.Run("send error fails without reply or retry", func(t *testing.T) {
		engine := &msgChannelSendErrorEngine{commandEngineStub: &commandEngineStub{}, err: errors.New("send failed")}
		definition, _ := commandDefinitionFor("msgchannel")
		status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!msgchannel programming hello"}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, engine.err) {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(engine.chats) != 1 || len(engine.raws) != 0 {
			t.Fatalf("side effects chats=%v raws=%v", engine.chats, engine.raws)
		}
	})
}

func TestMsgChannelListenerIntegration(t *testing.T) {
	for _, tc := range []struct {
		alias string
		type_ string
		want  string
	}{
		{"msgchannel", "", "alice |anonymous mail from: ?programming message: hello world|false"},
		{"msgroom", "whisper", "alice |anonymous mail from: ?programming message: hello world|true"},
	} {
		t.Run(tc.alias+"/"+tc.type_, func(t *testing.T) {
			engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}
			if err := RegisterUserUtilities(engine); err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(model.ChatMessage{Name: "alice", Type: tc.type_, Text: "!" + tc.alias + " programming hello world"})
			if err != nil {
				t.Fatal(err)
			}
			listener.NewUserChatListener(engine).Notify(string(payload))
			if len(engine.chats) != 1 || engine.chats[0] != tc.want {
				t.Fatalf("chats=%v, want [%q]", engine.chats, tc.want)
			}
		})
	}

	t.Run("denied resolved user gets only existing authorization denial", func(t *testing.T) {
		engine := &deniedMsgChannelEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}}
		if err := RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!msgroom programming hello"})
		if err != nil {
			t.Fatal(err)
		}
		listener.NewUserChatListener(engine).Notify(string(payload))
		want := "alice| you are not authorized to run: msgroom command.|false"
		if len(engine.chats) != 1 || engine.chats[0] != want {
			t.Fatalf("chats=%v, want [%q]", engine.chats, want)
		}
	})
}
