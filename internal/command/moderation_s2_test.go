package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type moderationOperationsEngineStub struct {
	*commandEngineStub
	err error
}

func (s *moderationOperationsEngineStub) operation(ctx context.Context, raw string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	s.raws = append(s.raws, raw)
	return nil
}
func (s *moderationOperationsEngineStub) BanNick(ctx context.Context, target common.NickTarget) error {
	return s.operation(ctx, `{"cmd":"ban","nick":"`+string(target)+`"}`)
}
func (s *moderationOperationsEngineStub) UnbanHash(ctx context.Context, hash common.BanHash) error {
	return s.operation(ctx, `{"cmd":"unban","hash":"`+string(hash)+`"}`)
}
func (s *moderationOperationsEngineStub) UnbanAllContext(ctx context.Context) error {
	return s.operation(ctx, `{"cmd":"unbanall"}`)
}
func (s *moderationOperationsEngineStub) LockRoom(ctx context.Context) error {
	return s.operation(ctx, `{"cmd":"lockroom"}`)
}
func (s *moderationOperationsEngineStub) UnlockRoom(ctx context.Context) error {
	return s.operation(ctx, `{"cmd":"unlockroom"}`)
}
func (s *moderationOperationsEngineStub) EnableCaptcha(ctx context.Context) error {
	return s.operation(ctx, `{"cmd":"enablecaptcha"}`)
}
func (s *moderationOperationsEngineStub) DisableCaptcha(ctx context.Context) error {
	return s.operation(ctx, `{"cmd":"disablecaptcha"}`)
}
func (s *moderationOperationsEngineStub) AuthorizeTrip(ctx context.Context, trip common.Trip) error {
	return s.operation(ctx, `{"cmd":"authtrip","trip":"`+string(trip)+`"}`)
}
func (s *moderationOperationsEngineStub) DeauthorizeTrip(ctx context.Context, trip common.Trip) error {
	return s.operation(ctx, `{"cmd":"deauthtrip","trip":"`+string(trip)+`"}`)
}
func (s *moderationOperationsEngineStub) MuteNick(context.Context, common.NickTarget) error {
	return nil
}
func (s *moderationOperationsEngineStub) UnmuteHash(context.Context, common.BanHash) error {
	return nil
}
func (s *moderationOperationsEngineStub) ForceFlair(context.Context, common.NickTarget, common.Flair) error {
	return nil
}
func (s *moderationOperationsEngineStub) ForceColor(context.Context, common.NickTarget, common.Color) error {
	return nil
}
func (s *moderationOperationsEngineStub) KickNick(context.Context, common.NickTarget) error {
	return nil
}
func (s *moderationOperationsEngineStub) KickNickTo(context.Context, common.NickTarget, common.Channel) error {
	return nil
}
func (s *moderationOperationsEngineStub) OverflowNick(ctx context.Context, target common.NickTarget) error {
	return s.operation(ctx, `{"cmd":"overflow","nick":"`+string(target)+`"}`)
}

func newModerationOperationsEngine() *moderationOperationsEngineStub {
	return &moderationOperationsEngineStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}}
}

func TestManualSimpleModerationCommandsUseTypedOperations(t *testing.T) {
	cases := []struct {
		name, command, raw, chat string
	}{
		{"captcha defaults on", "!captcha", `{"cmd":"enablecaptcha"}`, "mod| Captcha enabled!|true"},
		{"captcha off", "!captcha off", `{"cmd":"disablecaptcha"}`, "mod| Captcha disabled!|true"},
		{"authorize", "!auth trip", `{"cmd":"authtrip","trip":"trip"}`, "mod| authorized trip: trip|true"},
		{"deauthorize", "!deauth trip", `{"cmd":"deauthtrip","trip":"trip"}`, "mod| deauthorized trip: trip|true"},
		{"lock", "!lock on", `{"cmd":"lockroom"}`, "mod| Room locked!|true"},
		{"unlock", "!lock off", `{"cmd":"unlockroom"}`, "mod| Room unlocked!|true"},
		{"overflow", "!shoot @merc", `{"cmd":"overflow","nick":"merc"}`, ""},
		{"ban", "!ban @merc", `{"cmd":"ban","nick":"merc"}`, "mod|merc has been banned|true"},
		{"unban", "!unban hash", `{"cmd":"unban","hash":"hash"}`, "mod|hash has been unbanned|true"},
		{"unban all", "!pardonall", `{"cmd":"unbanall"}`, "mod|mercy.|true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newModerationOperationsEngine()
			canonical := strings.Fields(tc.command[1:])[0]
			if canonical == "auth" {
				canonical = "authorize"
			}
			if canonical == "deauth" {
				canonical = "deauthorize"
			}
			if canonical == "shoot" {
				canonical = "overflow"
			}
			if canonical == "pardonall" {
				canonical = "unbanall"
			}
			d, ok := commandDefinitionFor(canonical)
			if !ok {
				t.Fatalf("missing definition for %q", canonical)
			}
			status, err := d.New(e, moderationMessage(tc.command, true)).Execute(context.Background())
			if err != nil || status != model.SUCCESSFUL {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if got := e.raws; !equalStrings(got, []string{tc.raw}) {
				t.Fatalf("raw=%v", got)
			}
			if tc.chat == "" {
				if len(e.chats) != 0 {
					t.Fatalf("chat=%v", e.chats)
				}
			} else if !equalStrings(e.chats, []string{tc.chat}) {
				t.Fatalf("chat=%v", e.chats)
			}
		})
	}
}

func TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects(t *testing.T) {
	cases := []struct{ command, want string }{
		{"!captcha nope", "mod|!captcha [on|off]|false"},
		{"!auth", "mod| example: !auth cmdTV+|false"},
		{"!deauth", "mod| example: !deauth cmdTV+|false"},
		{"!lock no", "mod|!lock [on|off]|false"},
		{"!shoot", "mod|target nick isn't set! Example: !shoot @merc|false"},
		{"!ban", "mod|Example: !ban merc|false"},
		{"!unban", "mod|Example: !unban HjkUEWNlIRH35Xk|false"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			e := newModerationOperationsEngine()
			d, _ := commandDefinitionFor(strings.Fields(tc.command[1:])[0])
			status, err := d.New(e, moderationMessage(tc.command, false)).Execute(context.Background())
			if err != nil || status != model.FAILED || len(e.raws) != 0 || !equalStrings(e.chats, []string{tc.want}) {
				t.Fatalf("status=%v err=%v raw=%v chat=%v", status, err, e.raws, e.chats)
			}
		})
	}
	e := newModerationOperationsEngine()
	d, _ := commandDefinitionFor("captcha")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := d.New(e, moderationMessage("!captcha", false)).Execute(ctx)
	if !errors.Is(err, context.Canceled) || status != model.FAILED || len(e.raws) != 0 || len(e.chats) != 0 {
		t.Fatalf("cancel status=%v err=%v raw=%v chat=%v", status, err, e.raws, e.chats)
	}
}

func TestManualSimpleModerationOperationFailureHasNoSuccessReply(t *testing.T) {
	e := newModerationOperationsEngine()
	e.err = errors.New("transport down")
	d, _ := commandDefinitionFor("ban")
	status, err := d.New(e, moderationMessage("!ban merc", true)).Execute(context.Background())
	if err == nil || status != model.FAILED || len(e.raws) != 0 || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v raw=%v chat=%v", status, err, e.raws, e.chats)
	}
}

func TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases(t *testing.T) {
	e := newModerationOperationsEngine()
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"captcha", "authorize", "auth", "deauthorize", "deauth", "lock", "lockroom", "overflow", "shoot", "love", "hug", "kiss", "ban", "unban", "unbanall", "pardonall"} {
		metadata, ok := (*e.GetEnabledCommands())[alias]
		if !ok {
			t.Errorf("missing registered alias %q", alias)
			continue
		}
		command := metadata.Command(moderationMessage("!"+alias, false))
		if *command.GetRole() != model.MODERATOR {
			t.Errorf("alias %q role=%v, want moderator", alias, *command.GetRole())
		}
	}
}

// noModerationOperationsEngine has the identity persistence dependencies but
// deliberately lacks the typed raw moderation capability required by S2.
type noModerationOperationsEngine struct {
	common.Engine
	bundle   *service.Bundle
	commands map[string]common.CommandMetadata
}

func (s *noModerationOperationsEngine) ServiceBundle() *service.Bundle { return s.bundle }
func (s *noModerationOperationsEngine) RegisterCommand(c common.Command) {
	if s.commands == nil {
		s.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range c.GetAliases() {
		s.commands[alias] = common.CommandMetadata{Alias: alias}
	}
}

func TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability(t *testing.T) {
	e := &noModerationOperationsEngine{bundle: &service.Bundle{
		Security: &service.SecurityService{},
		Users:    &service.UserService{GroupB: &runtimeParityGroupB{}},
	}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"authorize", "auth", "deauthorize", "deauth", "captcha", "lock", "lockroom", "overflow", "shoot", "love", "hug", "kiss", "ban", "unban", "unbanall", "pardonall"} {
		if _, ok := e.commands[alias]; ok {
			t.Errorf("S2 alias %q registered without common.ModerationOperations", alias)
		}
	}
}
