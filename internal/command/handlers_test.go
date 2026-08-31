package command

import (
	"context"
	"errors"
	"testing"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/turn"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type directAgentInvokerStub struct {
	prompt string
	text   string
	err    error
	calls  int
}

func (s *directAgentInvokerStub) Invoke(_ context.Context, _ *model.ChatMessage, prompt string) (string, error) {
	s.calls++
	s.prompt = prompt
	return s.text, s.err
}

type commandEngineStub struct {
	common.Engine
	chats    []string
	raws     []string
	users    map[string]*model.User
	afkUsers map[*model.User]string
	bundle   *service.Bundle
	commands map[string]common.CommandMetadata
	subs     map[string]struct{}
}

func (s *commandEngineStub) ServiceBundle() *service.Bundle { return s.bundle }

func (s *commandEngineStub) SendChatMessage(author, text string, whisper bool) (string, error) {
	s.chats = append(s.chats, author+"|"+text+"|"+boolString(whisper))
	return text, nil
}
func (s *commandEngineStub) SendWhisperMessage(author, text string) (string, error) {
	s.chats = append(s.chats, author+"|"+text+"|true")
	return "/whisper @" + author + " " + text, nil
}
func (s *commandEngineStub) SendAddressedMessage(author, text string, whisper bool) (string, error) {
	message := "@" + author + " " + text
	if whisper {
		message = "/whisper @" + author + " " + text
	}
	s.chats = append(s.chats, message)
	return message, nil
}
func (s *commandEngineStub) SendRawMessage(text string) { s.raws = append(s.raws, text) }
func (s *commandEngineStub) SubscribeTrip(trip string) bool {
	if s.subs == nil {
		s.subs = map[string]struct{}{}
	}
	if _, ok := s.subs[trip]; ok {
		return false
	}
	s.subs[trip] = struct{}{}
	return true
}
func (s *commandEngineStub) UnsubscribeTrip(trip string) bool {
	if _, ok := s.subs[trip]; !ok {
		return false
	}
	delete(s.subs, trip)
	return true
}
func (s *commandEngineStub) IsSubscribedTrip(trip string) bool { _, ok := s.subs[trip]; return ok }
func (s *commandEngineStub) GetSubscribedTrips() []string {
	out := make([]string, 0, len(s.subs))
	for trip := range s.subs {
		out = append(out, trip)
	}
	return out
}
func (s *commandEngineStub) Ban(text string) {
	s.raws = append(s.raws, `{"cmd":"ban","nick":"`+text+`"}`)
}
func (s *commandEngineStub) Unban(text string) {
	s.raws = append(s.raws, `{"cmd":"unban","hash":"`+text+`"}`)
}
func (s *commandEngineStub) UnbanAll() { s.raws = append(s.raws, `{"cmd":"unbanall"}`) }
func (s *commandEngineStub) Lock()     { s.raws = append(s.raws, `{"cmd":"lockroom"}`) }
func (s *commandEngineStub) Unlock()   { s.raws = append(s.raws, `{"cmd":"unlockroom"}`) }
func (s *commandEngineStub) Kick(name, channel string) {
	s.raws = append(s.raws, `{"cmd":"kick","nick":"`+name+`","to":"`+channel+`"}`)
}
func (s *commandEngineStub) AddAfkUser(u *model.User, reason string) {
	if s.afkUsers == nil {
		s.afkUsers = map[*model.User]string{}
	}
	s.afkUsers[u] = reason
}
func (s *commandEngineStub) GetAfkUsers() *map[*model.User]string        { return &s.afkUsers }
func (s *commandEngineStub) GetActiveUserByName(name string) *model.User { return s.users[name] }
func (s *commandEngineStub) GetActiveUsers() *map[*model.User]struct{} {
	m := map[*model.User]struct{}{}
	for _, u := range s.users {
		m[u] = struct{}{}
	}
	return &m
}
func (s *commandEngineStub) GetChannel() string                                 { return "programming" }
func (s *commandEngineStub) GetName() string                                    { return "zenbot" }
func (s *commandEngineStub) GetPrefix() string                                  { return "!" }
func (s *commandEngineStub) IsUserAuthorized(_ *model.User, _ *model.Role) bool { return true }
func (s *commandEngineStub) LogMessage(_, _, _, _, _ string) (int64, error)     { return 0, nil }
func (s *commandEngineStub) RemoveIfAfk(_ *model.User)                          {}
func (s *commandEngineStub) NotifyAfkIfMentioned(_ *model.ChatMessage)          {}
func (s *commandEngineStub) RegisterCommand(c common.Command) {
	if s.commands == nil {
		s.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range c.GetAliases() {
		s.commands[alias] = common.CommandMetadata{Alias: alias, Command: func(m *model.ChatMessage) common.Command {
			return c.NewInstance(s, m)
		}}
	}
}
func (s *commandEngineStub) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &s.commands
}
func (s *commandEngineStub) boolString(_ bool) string { return "" }
func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
func whisperType(v bool) string {
	if v {
		return "whisper"
	}
	return ""
}

func TestConcreteCommandsExecuteWithSaturnSemantics(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"merc": {Name: "merc", Hash: "h"}, "alice": {Name: "alice", Trip: "trip"}}}
	cases := []struct {
		alias, text       string
		wantChat, wantRaw string
	}{
		{"say", "!say hello world", "|hello world |false", ""},
		{"afk", "!afk lunch", "alice| is afk|true", ""},
	}
	for _, tc := range cases {
		e.chats, e.raws = nil, nil
		d, ok := commandDefinitionFor(tc.alias)
		if !ok {
			t.Fatalf("missing definition %s", tc.alias)
		}
		msg := &model.ChatMessage{Text: tc.text, Name: "alice", Trip: "trip", IsWhisper: true}
		status, err := d.New(e, msg).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("%s: status=%v err=%v", tc.alias, status, err)
		}
		if tc.wantChat != "" && (len(e.chats) != 1 || e.chats[0] != tc.wantChat) {
			t.Fatalf("%s chat=%v", tc.alias, e.chats)
		}
		if tc.wantRaw != "" && (len(e.raws) != 1 || e.raws[0] != tc.wantRaw) {
			t.Fatalf("%s raw=%v", tc.alias, e.raws)
		}
	}
}

func TestDirectLCommandForwardsResponseAndInvokerFailure(t *testing.T) {
	for _, tc := range []struct {
		name, response, wantChat string
		err                      error
		wantStatus               model.Status
	}{
		{name: "response", response: "agent reply", wantChat: "alice|agent reply|false", wantStatus: model.SUCCESSFUL},
		{name: "failure", err: errors.New("provider unavailable"), wantStatus: model.FAILED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
			invoker := &directAgentInvokerStub{text: tc.response, err: tc.err}
			d, ok := directLDefinition(invoker)
			if !ok {
				t.Fatal("direct l definition was not available")
			}
			status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!l hello world"}).Execute(context.Background())
			if status != tc.wantStatus {
				t.Fatalf("status=%v, want %v", status, tc.wantStatus)
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("error=%v, want %v", err, tc.err)
			}
			if invoker.calls != 1 || invoker.prompt != "hello world" {
				t.Fatalf("invocations=%d prompt=%q", invoker.calls, invoker.prompt)
			}
			if tc.wantChat == "" {
				if len(e.chats) != 0 {
					t.Fatalf("unexpected chat=%v", e.chats)
				}
			} else if len(e.chats) != 1 || e.chats[0] != tc.wantChat {
				t.Fatalf("chat=%v, want %q", e.chats, tc.wantChat)
			}
		})
	}
}

type persistentDirectInvokerStub struct {
	directAgentInvokerStub
	persisted int
}

func (s *persistentDirectInvokerStub) Persist(context.Context, *model.ChatMessage, string, string) error {
	s.persisted++
	return nil
}

type directDeliveryInvokerStub struct {
	directAgentInvokerStub
	persisted int
}

func (s *directDeliveryInvokerStub) InvokeCompletion(_ context.Context, _ *model.ChatMessage, prompt string) (runtime.DirectCompletion, error) {
	s.calls++
	s.prompt = prompt
	return runtime.NewDirectCompletion(s.text, []turn.PersistableEvidence{{Tool: "room_users", Content: `{}`}}), s.err
}
func (s *directDeliveryInvokerStub) PersistDelivery(_ context.Context, _ *model.ChatMessage, _ string, completion runtime.DirectCompletion) error {
	if completion.Text() != s.text || len(completion.DurableEvidence()) != 1 {
		return errors.New("missing direct delivery artifact")
	}
	s.persisted++
	return nil
}

type failingCommandEngineStub struct {
	*commandEngineStub
	err error
}

func (s *failingCommandEngineStub) SendChatMessage(author, text string, whisper bool) (string, error) {
	s.chats = append(s.chats, author+"|"+text+"|"+boolString(whisper))
	return "", s.err
}

func TestDirectLCommandOnlyPersistsAfterVisibleSuccessfulDelivery(t *testing.T) {
	t.Run("no reply is not delivered or persisted", func(t *testing.T) {
		e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
		invoker := &persistentDirectInvokerStub{directAgentInvokerStub: directAgentInvokerStub{text: ""}}
		d, ok := directLDefinition(invoker)
		if !ok {
			t.Fatal("direct l definition was not available")
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!l hello"}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(e.chats) != 0 || invoker.persisted != 0 {
			t.Fatalf("no-reply delivery=%v persisted=%d", e.chats, invoker.persisted)
		}
	})

	t.Run("delivery failure is not persisted", func(t *testing.T) {
		e := &failingCommandEngineStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}, err: errors.New("sink failed")}
		invoker := &persistentDirectInvokerStub{directAgentInvokerStub: directAgentInvokerStub{text: "visible"}}
		d, ok := directLDefinition(invoker)
		if !ok {
			t.Fatal("direct l definition was not available")
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!l hello"}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, e.err) {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(e.chats) != 1 || invoker.persisted != 0 {
			t.Fatalf("delivery=%v persisted=%d", e.chats, invoker.persisted)
		}
	})

	t.Run("visible successful delivery persists once", func(t *testing.T) {
		e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
		invoker := &persistentDirectInvokerStub{directAgentInvokerStub: directAgentInvokerStub{text: "visible"}}
		d, ok := directLDefinition(invoker)
		if !ok {
			t.Fatal("direct l definition was not available")
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!l hello"}).Execute(context.Background())
		if err != nil || status != model.SUCCESSFUL || len(e.chats) != 1 || invoker.persisted != 1 {
			t.Fatalf("status=%v err=%v delivery=%v persisted=%d", status, err, e.chats, invoker.persisted)
		}
	})
}

func TestDirectLCommandPersistsDirectDeliveryArtifactAfterVisibleSend(t *testing.T) {
	invoker := &directDeliveryInvokerStub{directAgentInvokerStub: directAgentInvokerStub{text: "agent reply"}}
	e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	d, ok := directLDefinition(invoker)
	if !ok {
		t.Fatal("direct l definition missing")
	}
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!l prompt"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL || invoker.persisted != 1 {
		t.Fatalf("status=%v err=%v persisted=%d", status, err, invoker.persisted)
	}
}
