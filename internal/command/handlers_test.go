package command

import (
	"context"
	"encoding/json"
	"testing"
	"zenbot/internal/common"
	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type commandEngineStub struct {
	common.Engine
	chats              []string
	raws               []string
	users              map[string]*model.User
	afkUsers           map[*model.User]string
	bundle             *service.Bundle
	commands           map[string]common.CommandMetadata
	subs               map[string]struct{}
	authorizationCalls int
	audits             []model.CommandAuditRecord
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
func (s *commandEngineStub) BanNick(_ context.Context, target common.NickTarget) error {
	s.Ban(string(target))
	return nil
}
func (s *commandEngineStub) UnbanHash(_ context.Context, hash common.BanHash) error {
	s.Unban(string(hash))
	return nil
}
func (s *commandEngineStub) UnbanAllContext(_ context.Context) error { s.UnbanAll(); return nil }
func (s *commandEngineStub) LockRoom(_ context.Context) error        { s.Lock(); return nil }
func (s *commandEngineStub) UnlockRoom(_ context.Context) error      { s.Unlock(); return nil }
func (s *commandEngineStub) EnableCaptcha(_ context.Context) error {
	s.raws = append(s.raws, `{"cmd":"enablecaptcha"}`)
	return nil
}
func (s *commandEngineStub) DisableCaptcha(_ context.Context) error {
	s.raws = append(s.raws, `{"cmd":"disablecaptcha"}`)
	return nil
}
func (s *commandEngineStub) AuthorizeTrip(_ context.Context, trip common.Trip) error {
	s.raws = append(s.raws, `{"cmd":"authtrip","trip":"`+string(trip)+`"}`)
	return nil
}
func (s *commandEngineStub) DeauthorizeTrip(_ context.Context, trip common.Trip) error {
	s.raws = append(s.raws, `{"cmd":"deauthtrip","trip":"`+string(trip)+`"}`)
	return nil
}
func (s *commandEngineStub) MuteNick(_ context.Context, target common.NickTarget) error {
	s.raws = append(s.raws, `{"cmd":"mute","nick":"`+string(target)+`"}`)
	return nil
}
func (s *commandEngineStub) UnmuteHash(_ context.Context, hash common.BanHash) error {
	s.raws = append(s.raws, `{"cmd":"unmute","hash":"`+string(hash)+`"}`)
	return nil
}
func (s *commandEngineStub) ForceFlair(_ context.Context, target common.NickTarget, flair common.Flair) error {
	s.raws = append(s.raws, `{"cmd":"forceflair","nick":"`+string(target)+`","flair":"`+string(flair)+`"}`)
	return nil
}
func (s *commandEngineStub) ForceColor(_ context.Context, target common.NickTarget, color common.Color) error {
	s.raws = append(s.raws, `{"cmd":"forcecolor","nick":"`+string(target)+`","color":"`+string(color)+`"}`)
	return nil
}
func (s *commandEngineStub) KickNick(_ context.Context, target common.NickTarget) error {
	s.raws = append(s.raws, `{"cmd":"kick","nick":"`+string(target)+`"}`)
	return nil
}
func (s *commandEngineStub) KickNickTo(_ context.Context, target common.NickTarget, channel common.Channel) error {
	s.raws = append(s.raws, `{"cmd":"kick","nick":"`+string(target)+`","to":"`+string(channel)+`"}`)
	return nil
}
func (s *commandEngineStub) OverflowNick(_ context.Context, target common.NickTarget) error {
	s.raws = append(s.raws, `{"cmd":"overflow","nick":"`+string(target)+`"}`)
	return nil
}
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
func (s *commandEngineStub) GetChannel() string { return "programming" }
func (s *commandEngineStub) GetName() string    { return "zenbot" }
func (s *commandEngineStub) GetPrefix() string  { return "!" }
func (s *commandEngineStub) IsUserAuthorized(_ *model.User, _ *model.Role) bool {
	s.authorizationCalls++
	return true
}
func (s *commandEngineStub) LogMessage(_, _, _, _, _ string) (int64, error) { return 0, nil }
func (s *commandEngineStub) LogCommand(_ context.Context, record model.CommandAuditRecord) (int64, error) {
	s.audits = append(s.audits, record)
	return int64(len(s.audits)), nil
}
func (s *commandEngineStub) RemoveIfAfk(_ *model.User)                 {}
func (s *commandEngineStub) NotifyAfkIfMentioned(_ *model.ChatMessage) {}
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

type recordingDirectAgentSubmitter struct {
	calls   int
	ctx     context.Context
	message *model.ChatMessage
	prompt  string
}

func (s *recordingDirectAgentSubmitter) Submit(ctx context.Context, message *model.ChatMessage, prompt string) error {
	s.calls++
	s.ctx = ctx
	s.message = message
	s.prompt = prompt
	return nil
}

func TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
	submitter := &recordingDirectAgentSubmitter{}
	d, ok := directLDefinition(submitter)
	if !ok {
		t.Fatal("direct l definition was not available")
	}
	message := &model.ChatMessage{Name: "alice", Text: "!l   hello world   "}
	ctx := context.Background()
	status, err := d.New(e, message).Execute(ctx)
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if submitter.calls != 1 || submitter.ctx != ctx || submitter.message != message || submitter.prompt != "hello world" {
		t.Fatalf("submission = %#v", submitter)
	}
	if len(e.chats) != 0 || len(e.raws) != 0 {
		t.Fatalf("command side effects chats=%v raws=%v", e.chats, e.raws)
	}
}

func TestLiveLegacyDispatchPassesEngineCancellationToDirectLBeforeAdmission(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice", Hash: "hash"},
	}}
	submitter := &recordingDirectAgentSubmitter{}
	definition, ok := directLDefinition(submitter)
	if !ok {
		t.Fatal("direct l definition was not available")
	}
	engine.RegisterCommand(&legacyAdapter{engine: engine, def: definition})
	raw, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!l question"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	listener.NewUserChatListener(engine).NotifyContext(ctx, string(raw))

	if engine.authorizationCalls != 1 {
		t.Fatalf("authorization calls=%d, want 1", engine.authorizationCalls)
	}
	if submitter.calls != 0 {
		t.Fatalf("submissions=%d, want 0", submitter.calls)
	}
	if len(engine.chats) != 0 || len(engine.raws) != 0 {
		t.Fatalf("command output chats=%v raws=%v", engine.chats, engine.raws)
	}
}
