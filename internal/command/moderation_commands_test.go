package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

func moderationMessage(text string, whisper bool) *model.ChatMessage {
	return &model.ChatMessage{Name: "mod", Text: text, IsWhisper: whisper}
}

func TestRegisteredKickNormalizesMentionAndUsesTypedSaturnProtocol(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{"Merc": {Name: "Merc"}}}
	definition, ok := commandDefinitionFor("kick")
	if !ok {
		t.Fatal("registered kick command is missing")
	}
	status, err := definition.New(e, moderationMessage("!kick @merc", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if got, want := e.raws, []string{`{"cmd":"kick","nick":"Merc"}`}; !equalStrings(got, want) {
		t.Fatalf("raw=%v, want %v", got, want)
	}
	if len(e.chats) != 0 {
		t.Fatalf("kick unexpectedly posted confirmation: %v", e.chats)
	}
}

func TestRegisteredKickSupportsSaturnMultiAndContainsModes(t *testing.T) {
	definition, ok := commandDefinitionFor("kick")
	if !ok {
		t.Fatal("registered kick command is missing")
	}
	for _, tc := range []struct {
		name string
		text string
		want map[string]bool
	}{
		{
			name: "multiple normalized targets",
			text: "!kick -m @RaiderOne RaiderTwo absent",
			want: map[string]bool{"RaiderOne": true, "RaiderTwo": true},
		},
		{
			name: "active names containing value",
			text: "!kick -c Raider",
			want: map[string]bool{"RaiderOne": true, "RaiderTwo": true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &commandEngineStub{users: map[string]*model.User{
				"RaiderOne": {Name: "RaiderOne"},
				"RaiderTwo": {Name: "RaiderTwo"},
				"Resident":  {Name: "Resident"},
			}}
			status, err := definition.New(e, moderationMessage(tc.text, false)).Execute(context.Background())
			if status != model.SUCCESSFUL || err != nil {
				t.Fatalf("status=%v err=%v", status, err)
			}
			got := make(map[string]bool, len(e.raws))
			for _, payload := range e.raws {
				var fields map[string]string
				if err := json.Unmarshal([]byte(payload), &fields); err != nil || fields["cmd"] != "kick" || fields["nick"] == "" || fields["to"] != "" {
					t.Fatalf("invalid kick payload %q: fields=%v err=%v", payload, fields, err)
				}
				got[fields["nick"]] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("kicked=%v, want %v", got, tc.want)
			}
			for nick := range tc.want {
				if !got[nick] {
					t.Fatalf("kicked=%v, missing %q", got, nick)
				}
			}
		})
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
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.raws, []string{`{"cmd":"mute","nick":"Merc"}`}) || !equalStrings(e.chats, []string{"mod|Merc hash-x has been muted|false"}) {
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
		if status != model.SUCCESSFUL || err != nil || len(e.raws) != 1 || !sameJSONObject(e.raws[0], tc.raw) {
			t.Fatalf("%s status=%v err=%v raws=%v", tc.text, status, err, e.raws)
		}
		if tc.reply != "" && !equalStrings(e.chats, []string{tc.reply}) {
			t.Fatalf("%s chats=%v", tc.text, e.chats)
		}
	}
}

func sameJSONObject(left, right string) bool {
	var leftObject, rightObject map[string]any
	if json.Unmarshal([]byte(left), &leftObject) != nil || json.Unmarshal([]byte(right), &rightObject) != nil {
		return false
	}
	if len(leftObject) != len(rightObject) {
		return false
	}
	for key, value := range leftObject {
		if rightObject[key] != value {
			return false
		}
	}
	return true
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
	rows      []repository.ShadowBanRecord
}

func (f *shadowManagementFake) PersistShadowBanRecord(_ context.Context, record repository.ShadowBanRecord) error {
	f.selectors = append(f.selectors, record.Name)
	return nil
}
func (f *shadowManagementFake) ListShadowBans(context.Context) ([]repository.ShadowBanRecord, error) {
	return f.rows, nil
}
func (f *shadowManagementFake) HasShadowBanMatch(ctx context.Context, trip, name, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for _, record := range f.rows {
		if (strings.TrimSpace(trip) != "" && trip == record.Trip) || (strings.TrimSpace(name) != "" && name == record.Name) || (strings.TrimSpace(hash) != "" && hash == record.Hash) {
			return true, nil
		}
	}
	return false, nil
}
func (f *shadowManagementFake) RemoveShadowBanBySourceTarget(context.Context, string) (int64, error) {
	return 1, nil
}
func (f *shadowManagementFake) RemoveAllShadowBans(context.Context) (int64, error) { return 0, nil }

var _ repository.ShadowBanCommandRepository = (*shadowManagementFake)(nil)

func TestShadowBanOfflineSelectorUsesTypedPersistence(t *testing.T) {
	fake := &shadowManagementFake{}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: fake}}}
	d, _ := commandDefinitionFor("sban")
	status, err := d.New(e, moderationMessage("!sban offline", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(fake.selectors, []string{"offline"}) || !equalStrings(e.chats, []string{"mod|banned: offline|false"}) {
		t.Fatalf("status=%v err=%v selectors=%v chats=%v", status, err, fake.selectors, e.chats)
	}
}

func TestShadowBanListRendersSourceShape(t *testing.T) {
	fake := &shadowManagementFake{rows: []repository.ShadowBanRecord{{Hash: "raw", Trip: "", Name: "offline"}}}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: fake}}}
	d, _ := commandDefinitionFor("banlist")
	status, err := d.New(e, moderationMessage("!banlist", true)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.chats, []string{"mod|Banned hashes, trips, names: \\nraw - ------ - offline\\n|true"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestUnshadowBanRemovesTypedSelector(t *testing.T) {
	fake := &shadowManagementFake{}
	e := &commandEngineStub{bundle: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: fake}}}
	d, _ := commandDefinitionFor("unblock")
	status, err := d.New(e, moderationMessage("!unblock target", false)).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(e.chats, []string{"mod|Unbanned shadow-ban records: 1|false"}) {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestModeratorCommandIsBlockedForUnauthorizedAuthor(t *testing.T) {
	e := &deniedModerationEngine{commandEngineStub: &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice"},
	}}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
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
