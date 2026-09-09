package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/listener"
	"zenbot/internal/model"
)

type recordingSupportRelay struct {
	requests []common.SupportRelayRequest
	err      error
}

func (r *recordingSupportRelay) RelayToSupport(_ context.Context, request common.SupportRelayRequest) error {
	r.requests = append(r.requests, request)
	return r.err
}

type supportRelayRegistrationEngine struct {
	*commandEngineStub
	*recordingSupportRelay
}

func (e *supportRelayRegistrationEngine) RegisterCommand(command common.Command) {
	if e.commands == nil {
		e.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range command.GetAliases() {
		e.commands[alias] = common.CommandMetadata{Alias: alias, Command: func(message *model.ChatMessage) common.Command {
			return command.NewInstance(e, message)
		}}
	}
}

func TestSupportRelayAliasesAreUnregisteredWithoutCapability(t *testing.T) {
	without := &commandEngineStub{}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	with := &supportRelayRegistrationEngine{commandEngineStub: &commandEngineStub{}, recordingSupportRelay: &recordingSupportRelay{}}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"ws", "wsay", "wsa", "wsayanon", "anonsay"} {
		if _, ok := (*without.GetEnabledCommands())[alias]; ok {
			t.Errorf("%q registered without SupportReplicaRelay", alias)
		}
		metadata, ok := (*with.GetEnabledCommands())[alias]
		if !ok {
			t.Errorf("%q not registered with SupportReplicaRelay", alias)
			continue
		}
		command := metadata.Command(&model.ChatMessage{})
		if role := command.GetRole(); role == nil || *role != model.USER {
			t.Errorf("%q role=%v, want USER", alias, role)
		}
		adapter, ok := command.(*legacyAdapter)
		if !ok {
			t.Errorf("%q command=%T, want legacy adapter", alias, command)
			continue
		}
		if _, generic := adapter.def.New(with, &model.ChatMessage{}).(*saturnCommand); generic {
			t.Errorf("%q remains a generic fallback", alias)
		}
	}
}

func TestSupportRelayCommandsForwardEveryAliasWithoutCallerReply(t *testing.T) {
	for _, tc := range []struct {
		alias string
		anon  bool
	}{{"ws", false}, {"wsay", false}, {"wsa", true}, {"wsayanon", true}, {"anonsay", true}} {
		t.Run(tc.alias, func(t *testing.T) {
			relay := &recordingSupportRelay{}
			engine := &supportRelayRegistrationEngine{commandEngineStub: &commandEngineStub{}, recordingSupportRelay: relay}
			definition, ok := commandDefinitionFor(tc.alias)
			if !ok {
				t.Fatalf("definition for %q is missing", tc.alias)
			}
			message := &model.ChatMessage{Name: "alice", Trip: "trip", Text: "!" + tc.alias + " hello world", IsWhisper: true}
			status, err := definition.New(engine, message).Execute(context.Background())
			if status != model.SUCCESSFUL || err != nil {
				t.Fatalf("Execute() = (%v, %v)", status, err)
			}
			if len(relay.requests) != 1 {
				t.Fatalf("requests=%#v", relay.requests)
			}
			got := relay.requests[0]
			if got.Author != "alice" || got.Trip != "trip" || got.Anonymous != tc.anon || len(got.Arguments) != 2 || got.Arguments[0] != "hello" || got.Arguments[1] != "world" {
				t.Fatalf("request=%#v", got)
			}
			if len(engine.chats) != 0 {
				t.Fatalf("caller replies=%v", engine.chats)
			}
		})
	}
}

func TestSupportRelayCommandsFailWithoutRelayOrAfterCancellation(t *testing.T) {
	definition, _ := commandDefinitionFor("ws")
	message := &model.ChatMessage{Name: "alice", Text: "!ws hello"}
	without := &commandEngineStub{}
	status, err := definition.New(without, message).Execute(context.Background())
	if status != model.FAILED || err == nil || len(without.chats) != 0 {
		t.Fatalf("without relay = (%v, %v), chats=%v", status, err, without.chats)
	}
	relay := &recordingSupportRelay{err: errors.New("send failed")}
	with := &supportRelayRegistrationEngine{commandEngineStub: &commandEngineStub{}, recordingSupportRelay: relay}
	status, err = definition.New(with, message).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, relay.err) || len(relay.requests) != 1 || len(with.chats) != 0 {
		t.Fatalf("relay failure = (%v, %v), requests=%v chats=%v", status, err, relay.requests, with.chats)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err = definition.New(with, message).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(relay.requests) != 1 || len(with.chats) != 0 {
		t.Fatalf("cancelled = (%v, %v), requests=%v chats=%v", status, err, relay.requests, with.chats)
	}
}

type deniedSupportRelayEngine struct {
	*supportRelayRegistrationEngine
}

func (*deniedSupportRelayEngine) IsUserAuthorized(*model.User, *model.Role) bool { return false }
func (e *deniedSupportRelayEngine) RegisterCommand(command common.Command) {
	if e.commands == nil {
		e.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range command.GetAliases() {
		e.commands[alias] = common.CommandMetadata{Alias: alias, Command: func(message *model.ChatMessage) common.Command {
			return command.NewInstance(e, message)
		}}
	}
}

func TestSupportRelayDispatchUsesNormalUserAuthorizationWithoutCallerReply(t *testing.T) {
	relay := &recordingSupportRelay{}
	allowed := &supportRelayRegistrationEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}}}, recordingSupportRelay: relay}
	if err := RegisterUserUtilities(allowed); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "trip", Text: "!wsa hello! <tag>"})
	if err != nil {
		t.Fatal(err)
	}
	listener.NewUserChatListener(allowed).Notify(string(payload))
	if len(relay.requests) != 1 || !relay.requests[0].Anonymous || len(allowed.chats) != 0 {
		t.Fatalf("allowed requests=%v replies=%v", relay.requests, allowed.chats)
	}

	deniedRelay := &recordingSupportRelay{}
	denied := &deniedSupportRelayEngine{supportRelayRegistrationEngine: &supportRelayRegistrationEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}}}, recordingSupportRelay: deniedRelay}}
	if err := RegisterUserUtilities(denied); err != nil {
		t.Fatal(err)
	}
	listener.NewUserChatListener(denied).Notify(string(payload))
	if len(deniedRelay.requests) != 0 {
		t.Fatalf("denied relay requests=%v", deniedRelay.requests)
	}
	if len(denied.chats) != 1 || denied.chats[0] != "alice| you are not authorized to run: wsa command.|false" {
		t.Fatalf("denied replies=%v", denied.chats)
	}
}
