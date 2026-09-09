package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

func moderationMessage(text string, whisper bool) *model.ChatMessage {
	return &model.ChatMessage{Name: "mod", Text: text, IsWhisper: whisper}
}

func TestBanUsesNormalizedNickAndExactConfirmation(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{}}
	cmd := (&Ban{}).NewInstance(e, moderationMessage("!ban @merc", true)).(*Ban)
	cmd.Execute()
	if got, want := e.raws, []string{`{"cmd":"ban","nick":"merc"}`}; !equalStrings(got, want) {
		t.Fatalf("raw=%v, want %v", got, want)
	}
	if got, want := e.chats, []string{"mod|merc has been banned|true"}; !equalStrings(got, want) {
		t.Fatalf("chat=%v, want %v", got, want)
	}
}

func TestBanWithoutTargetIsSilent(t *testing.T) {
	e := &commandEngineStub{}
	(&Ban{}).NewInstance(e, moderationMessage("!ban", false)).(*Ban).Execute()
	if len(e.raws) != 0 || !equalStrings(e.chats, []string{"mod|Example: !ban merc|false"}) {
		t.Fatalf("side effects=%v/%v", e.raws, e.chats)
	}
}

func TestKickRequiresActiveUserAndUsesSixCharacterDestination(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"merc": {Name: "merc"}}}
	(&Kick{}).NewInstance(e, moderationMessage("!kick @merc", false)).(*Kick).Execute()
	if len(e.raws) != 1 || len(e.raws[0]) == 0 {
		t.Fatalf("raw=%v", e.raws)
	}
	if len(e.chats) != 0 {
		t.Fatalf("chat=%v", e.chats)
	}

	e.raws, e.chats = nil, nil
	(&Kick{}).NewInstance(e, moderationMessage("!kick nobody", false)).(*Kick).Execute()
	if len(e.raws) != 0 || len(e.chats) != 0 {
		t.Fatalf("missing target effects=%v/%v", e.raws, e.chats)
	}
}

func TestUnbanPreservesHashAndUsageBoundary(t *testing.T) {
	e := &commandEngineStub{}
	(&Unban{}).NewInstance(e, moderationMessage("!unban HjkUEWNlIRH35Xk", true)).(*Unban).Execute()
	if got, want := e.raws, []string{`{"cmd":"unban","hash":"HjkUEWNlIRH35Xk"}`}; !equalStrings(got, want) {
		t.Fatalf("raw=%v, want %v", got, want)
	}
	if got, want := e.chats, []string{"mod|HjkUEWNlIRH35Xk has been unbanned|true"}; !equalStrings(got, want) {
		t.Fatalf("chat=%v, want %v", got, want)
	}
	e.raws, e.chats = nil, nil
	(&Unban{}).NewInstance(e, moderationMessage("!unban", false)).(*Unban).Execute()
	if len(e.raws) != 0 || len(e.chats) != 1 || e.chats[0] != "mod|Example: !unban HjkUEWNlIRH35Xk|false" {
		t.Fatalf("usage effects=%v/%v", e.raws, e.chats)
	}
}

func TestUnbanAllAndLockBoundaries(t *testing.T) {
	e := &commandEngineStub{}
	(&UnbanAll{}).NewInstance(e, moderationMessage("!pardonall", false)).(*UnbanAll).Execute()
	if !equalStrings(e.raws, []string{`{"cmd":"unbanall"}`}) || !equalStrings(e.chats, []string{"mod|mercy.|false"}) {
		t.Fatalf("unbanall effects=%v/%v", e.raws, e.chats)
	}

	e.raws, e.chats = nil, nil
	(&Lock{}).NewInstance(e, moderationMessage("!lock on", true)).(*Lock).Execute()
	if !equalStrings(e.raws, []string{`{"cmd":"lockroom"}`}) {
		t.Fatalf("lock raw=%v", e.raws)
	}
	if len(e.chats) != 1 || e.chats[0] != "mod| Room locked!|true" {
		t.Fatalf("lock chat=%v", e.chats)
	}

	e.raws, e.chats = nil, nil
	(&Lock{}).NewInstance(e, moderationMessage("!lock", false)).(*Lock).Execute()
	if len(e.raws) != 0 || len(e.chats) != 1 || e.chats[0] != "mod|!lock [on|off]|false" {
		t.Fatalf("lock usage=%v/%v", e.raws, e.chats)
	}
}

func TestDeauthorizeUsesSourceRawCommandAndUsage(t *testing.T) {
	e := &commandEngineStub{}
	d, _ := commandDefinitionFor("deauth")
	status, err := d.New(e, moderationMessage("!deauth trip-x", true)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{`{"cmd":"deauthtrip","trip":"trip-x"}`}) || !equalStrings(e.chats, []string{"mod| deauthorized trip: trip-x|true"}) {
		t.Fatalf("status=%v err=%v raw=%v chats=%v", status, err, e.raws, e.chats)
	}
}

func TestCaptchaUsesSourceDefaultAndOnOffProtocol(t *testing.T) {
	for _, tc := range []struct {
		text, raw, reply string
		status           model.Status
	}{
		{"!captcha", `{"cmd":"enablecaptcha"}`, "mod| Captcha enabled!|false", model.SUCCESSFUL},
		{"!captcha off", `{"cmd":"disablecaptcha"}`, "mod| Captcha disabled!|false", model.SUCCESSFUL},
	} {
		e := &commandEngineStub{}
		d, _ := commandDefinitionFor("captcha")
		status, err := d.New(e, moderationMessage(tc.text, false)).Execute(context.Background())
		if status != tc.status || err != nil || !equalStrings(e.raws, []string{tc.raw}) || !equalStrings(e.chats, []string{tc.reply}) {
			t.Fatalf("%s status=%v err=%v raw=%v chats=%v", tc.text, status, err, e.raws, e.chats)
		}
	}
}

func TestMuteRequiresActiveTargetAndRetainsRawProtocol(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"Merc": {Name: "Merc", Hash: "hash-x"}}}
	d, _ := commandDefinitionFor("dumb")
	status, err := d.New(e, moderationMessage("!dumb @merc", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{`{"cmd":"mute","nick":"merc"}`}) || !equalStrings(e.chats, []string{"mod|merc hash-x has been muted|false"}) {
		t.Fatalf("status=%v err=%v raw=%v chats=%v", status, err, e.raws, e.chats)
	}
}

func TestUnmuteUsesSourceHashProtocol(t *testing.T) {
	e := &commandEngineStub{}
	d, _ := commandDefinitionFor("undumb")
	status, err := d.New(e, moderationMessage("!undumb hash-x", true)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{`{"cmd":"unmute","hash":"hash-x"}`}) || !equalStrings(e.chats, []string{"mod|hash-x has been unmuted|true"}) {
		t.Fatalf("status=%v err=%v raw=%v chats=%v", status, err, e.raws, e.chats)
	}
}

func TestColorAndFlairRequireActiveUserAndUseSourcePayloads(t *testing.T) {
	for _, tc := range []struct{ text, raw, reply string }{
		{"!color @Merc 00ff00", `{"cmd":"forcecolor","color":"00ff00","nick":"Merc"}`, ""},
		{"!flair Merc trusted", `{"cmd":"forceflair","flair":"trusted","nick":"Merc"}`, "mod|\\n Flair set successfully!|false"},
	} {
		e := &commandEngineStub{users: map[string]*model.User{"Merc": {Name: "Merc"}}}
		canonical := "color"
		if strings.HasPrefix(tc.text, "!flair") {
			canonical = "flair"
		}
		d, _ := commandDefinitionFor(canonical)
		status, err := d.New(e, moderationMessage(tc.text, false)).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{tc.raw}) {
			t.Fatalf("%s status=%v err=%v raws=%v", tc.text, status, err, e.raws)
		}
		if tc.reply != "" && !equalStrings(e.chats, []string{tc.reply}) {
			t.Fatalf("%s chats=%v", tc.text, e.chats)
		}
	}
}

func TestOverflowAliasesUseSourceRawAction(t *testing.T) {
	e := &commandEngineStub{}
	d, _ := commandDefinitionFor("hug")
	status, err := d.New(e, moderationMessage("!hug @Merc", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{`{"cmd":"overflow","nick":"Merc"}`}) || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v raw=%v chats=%v", status, err, e.raws, e.chats)
	}
}

type shadowManagementFake struct {
	selectors []string
	rows      []model.BanRecord
}

func (f *shadowManagementFake) PersistShadowBanSelector(_ context.Context, name, _ string) error {
	f.selectors = append(f.selectors, name)
	return nil
}
func (f *shadowManagementFake) ListShadowBans(context.Context) ([]model.BanRecord, error) {
	return f.rows, nil
}
func (f *shadowManagementFake) RemoveShadowBan(context.Context, string) error { return nil }

var _ repository.ShadowBanManagementRepository = (*shadowManagementFake)(nil)

func TestShadowBanOfflineSelectorUsesTypedPersistence(t *testing.T) {
	fake := &shadowManagementFake{}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: fake}}
	d, _ := commandDefinitionFor("sban")
	status, err := d.New(e, moderationMessage("!sban offline", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(fake.selectors, []string{"offline"}) || !equalStrings(e.chats, []string{"mod|banned: offline|false"}) {
		t.Fatalf("status=%v err=%v selectors=%v chats=%v", status, err, fake.selectors, e.chats)
	}
}

func TestShadowBanListRendersSourceShape(t *testing.T) {
	fake := &shadowManagementFake{rows: []model.BanRecord{{Hash: "raw", Trip: "", Name: "offline"}}}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: fake}}
	d, _ := commandDefinitionFor("banlist")
	status, err := d.New(e, moderationMessage("!banlist", true)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.chats, []string{"mod|Banned hashes, trips, names: \\nraw - ------ - offline\\n|true"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestUnshadowBanRemovesTypedSelector(t *testing.T) {
	fake := &shadowManagementFake{}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: fake}}
	d, _ := commandDefinitionFor("unblock")
	status, err := d.New(e, moderationMessage("!unblock target", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.chats, []string{"mod| unbanned target|false"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestModeratorCommandRolesAndAliases(t *testing.T) {
	cases := []struct {
		cmd interface {
			GetRole() *model.Role
			GetAliases() []string
		}
		aliases []string
	}{
		{&Ban{}, []string{"ban"}}, {&Kick{}, []string{"kick", "k", "out"}},
		{&Unban{}, []string{"unban"}}, {&UnbanAll{}, []string{"unbanall", "pardonall"}},
		{&Lock{}, []string{"lock", "lockroom"}}, {&Unlock{}, []string{"unlock", "unlockroom"}},
	}
	for _, tc := range cases {
		if !equalStrings(tc.cmd.GetAliases(), tc.aliases) {
			t.Fatalf("command aliases mismatch: %v", tc.aliases)
		}
	}
	e := &commandEngineStub{}
	for _, cmd := range []common.Command{(&Ban{}).NewInstance(e, moderationMessage("!ban x", false)), (&Kick{}).NewInstance(e, moderationMessage("!kick x", false)), (&Unban{}).NewInstance(e, moderationMessage("!unban x", false)), (&UnbanAll{}).NewInstance(e, moderationMessage("!unbanall", false)), (&Lock{}).NewInstance(e, moderationMessage("!lock on", false)), (&Unlock{}).NewInstance(e, moderationMessage("!unlock", false))} {
		if *cmd.GetRole() != model.MODERATOR {
			t.Fatalf("role=%v, want moderator", *cmd.GetRole())
		}
	}
}

func TestModeratorCommandIsBlockedForUnauthorizedAuthor(t *testing.T) {
	e := &deniedModerationEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice"},
	}}}
	e.RegisterCommand(&Ban{})
	payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Text: "!ban merc"})
	listener.NewUserChatListener(e).Notify(string(payload))
	if len(e.raws) != 0 {
		t.Fatalf("unauthorized raw side effects=%v", e.raws)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice| you are not authorized to run: ban command.|false" {
		t.Fatalf("authorization response=%v", e.chats)
	}
}

type deniedModerationEngine struct{ *commandEngineStub }

func (*deniedModerationEngine) IsUserAuthorized(_ *model.User, _ *model.Role) bool { return false }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
