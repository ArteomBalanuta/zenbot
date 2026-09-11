package command

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type activeModerationEngine struct {
	*commandEngineStub
	operations []string
	err        error
	current    *model.User
}

func (e *activeModerationEngine) GetActiveUserByName(name string) *model.User {
	if e.current != nil {
		return e.current
	}
	for _, user := range e.users {
		if user != nil && strings.EqualFold(user.Name, name) {
			return user
		}
	}
	return nil
}

func (e *activeModerationEngine) MuteNick(_ context.Context, target common.NickTarget) error {
	e.operations = append(e.operations, "mute:"+string(target))
	return e.err
}
func (e *activeModerationEngine) UnmuteHash(_ context.Context, hash common.BanHash) error {
	e.operations = append(e.operations, "unmute:"+string(hash))
	return e.err
}
func (e *activeModerationEngine) ForceFlair(_ context.Context, target common.NickTarget, flair common.Flair) error {
	e.operations = append(e.operations, "flair:"+string(target)+":"+string(flair))
	return e.err
}
func (e *activeModerationEngine) ForceColor(_ context.Context, target common.NickTarget, color common.Color) error {
	e.operations = append(e.operations, "color:"+string(target)+":"+string(color))
	return e.err
}

func activeEngine(users map[string]*model.User) *activeModerationEngine {
	return &activeModerationEngine{commandEngineStub: &commandEngineStub{users: users}}
}

func executeActiveModeration(t *testing.T, engine *activeModerationEngine, text string, whisper bool) (model.Status, error) {
	t.Helper()
	return executeActiveModerationContext(t, context.Background(), engine, text, whisper)
}

func executeActiveModerationContext(t *testing.T, ctx context.Context, engine *activeModerationEngine, text string, whisper bool) (model.Status, error) {
	t.Helper()
	commandName := ""
	for _, r := range text[1:] {
		if r == ' ' {
			break
		}
		commandName += string(r)
	}
	var command common.SaturnCommand
	base := commandBase{engine: engine, message: &model.ChatMessage{Name: "mod", Text: text, IsWhisper: whisper}, role: model.MODERATOR, aliases: []string{commandName}, canonical: commandName}
	switch commandName {
	case "mute", "dumb":
		command = &muteCommand{base}
	case "unmute", "undumb":
		command = &unmuteCommand{base}
	case "color":
		command = &colorCommand{base}
	case "flair":
		command = &flairCommand{base}
	default:
		t.Fatalf("unsupported S3 command %q", commandName)
	}
	return command.Execute(ctx)
}

func TestMuteRequiresActiveCaseInsensitiveTargetAndIncludesResolvedHash(t *testing.T) {
	t.Run("missing target uses source example", func(t *testing.T) {
		engine := activeEngine(nil)
		status, err := executeActiveModeration(t, engine, "!dumb", true)
		if err != nil || status != model.FAILED || !reflect.DeepEqual(engine.chats, []string{"mod|Example: !mute merc|true"}) || len(engine.operations) != 0 {
			t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
		}
	})
	t.Run("absent target has no operation", func(t *testing.T) {
		engine := activeEngine(nil)
		status, err := executeActiveModeration(t, engine, "!mute merc", false)
		if err != nil || status != model.FAILED || !reflect.DeepEqual(engine.chats, []string{"mod|merc is not in the room|false"}) || len(engine.operations) != 0 {
			t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
		}
	})
	t.Run("active target resolves case and sends canonical active name", func(t *testing.T) {
		engine := activeEngine(map[string]*model.User{"Merc": {Name: "Merc", Hash: "hash-a"}})
		status, err := executeActiveModeration(t, engine, "!mute @mErC", false)
		if err != nil || status != model.SUCCESSFUL || !reflect.DeepEqual(engine.operations, []string{"mute:Merc"}) || !reflect.DeepEqual(engine.chats, []string{"mod|Mute request sent for Merc hash-a; server application is unconfirmed.|false"}) {
			t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
		}
	})
}

func TestUnmuteUsesFirstHashAndSourceOutputs(t *testing.T) {
	engine := activeEngine(nil)
	status, err := executeActiveModeration(t, engine, "!undumb hash-a ignored", true)
	if err != nil || status != model.SUCCESSFUL || !reflect.DeepEqual(engine.operations, []string{"unmute:hash-a"}) || !reflect.DeepEqual(engine.chats, []string{"mod|Unmute request sent for hash hash-a; server application is unconfirmed.|true"}) {
		t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
	}
	engine = activeEngine(nil)
	status, err = executeActiveModeration(t, engine, "!unmute", false)
	if err != nil || status != model.FAILED || !reflect.DeepEqual(engine.chats, []string{"mod|Example: !unmute jJ4M4fsECSazzlj|false"}) || len(engine.operations) != 0 {
		t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
	}
}

func TestColorAndFlairRequireActiveCaseInsensitiveTarget(t *testing.T) {
	for _, tc := range []struct {
		name, command, operation, absent, success string
	}{
		{"color", "color", "color:Merc:00ff00", "User merc is not in the room, color was not applied.", ""},
		{"flair", "flair", "flair:Merc:trusted", "User merc is not in the room, flair was not applied.", "\\n Flair request sent; server application is unconfirmed."},
	} {
		t.Run(tc.name+" absent", func(t *testing.T) {
			engine := activeEngine(nil)
			status, err := executeActiveModeration(t, engine, "!"+tc.command+" merc "+map[string]string{"color": "00ff00", "flair": "trusted"}[tc.command], false)
			if err != nil || status != model.FAILED || !reflect.DeepEqual(engine.chats, []string{"mod|" + tc.absent + "|false"}) || len(engine.operations) != 0 {
				t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
			}
		})
		t.Run(tc.name+" active", func(t *testing.T) {
			engine := activeEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
			status, err := executeActiveModeration(t, engine, "!"+tc.command+" @mErC "+map[string]string{"color": "00ff00", "flair": "trusted"}[tc.command], true)
			var wantChats []string
			if tc.success != "" {
				wantChats = []string{"mod|" + tc.success + "|true"}
			}
			if err != nil || status != model.SUCCESSFUL || !reflect.DeepEqual(engine.operations, []string{tc.operation}) || !reflect.DeepEqual(engine.chats, wantChats) {
				t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
			}
		})
	}
}

func TestActiveTargetModerationPropagatesOperationFailureWithoutReply(t *testing.T) {
	engine := activeEngine(map[string]*model.User{"merc": {Name: "merc", Hash: "hash"}})
	engine.err = errors.New("outbound failed")
	status, err := executeActiveModeration(t, engine, "!mute merc", false)
	if status != model.FAILED || !errors.Is(err, engine.err) || !reflect.DeepEqual(engine.operations, []string{"mute:merc"}) || len(engine.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
	}
}

func TestActiveTargetModerationUsesCurrentAuthoritativeLookup(t *testing.T) {
	engine := activeEngine(nil)
	engine.current = &model.User{Name: "Merc", Hash: "current-hash"}
	status, err := executeActiveModeration(t, engine, "!mute @mErC", false)
	if err != nil || status != model.SUCCESSFUL || !reflect.DeepEqual(engine.operations, []string{"mute:Merc"}) || !reflect.DeepEqual(engine.chats, []string{"mod|Mute request sent for Merc current-hash; server application is unconfirmed.|false"}) {
		t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
	}
}

func TestRegisterUserUtilitiesRegistersConcreteS3ModeratorAliases(t *testing.T) {
	engine := activeEngine(nil)
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	wants := map[string]string{
		"mute": "mute", "dumb": "mute", "unmute": "unmute", "undumb": "unmute", "color": "color", "flair": "flair",
	}
	for alias, canonical := range wants {
		metadata, ok := engine.commands[alias]
		if !ok {
			t.Errorf("%s alias %q was not registered", canonical, alias)
			continue
		}
		instance := metadata.Command(&model.ChatMessage{Name: "mod", Text: "!" + alias})
		adapter, ok := instance.(*legacyAdapter)
		if !ok {
			t.Errorf("%s alias %q instance=%T, want legacy adapter", canonical, alias, instance)
			continue
		}
		if adapter.GetRole() == nil || *adapter.GetRole() != model.MODERATOR {
			t.Errorf("%s alias %q role=%v, want MODERATOR", canonical, alias, adapter.GetRole())
		}
		if _, generic := adapter.def.New(engine, &model.ChatMessage{}).(*saturnCommand); generic {
			t.Errorf("%s alias %q resolved to generic fallback", canonical, alias)
		}
	}
}

func TestActiveTargetModerationCancellationProducesNoPayloadOrReply(t *testing.T) {
	for _, text := range []string{"!mute merc", "!unmute hash-a", "!color merc 00ff00", "!flair merc trusted"} {
		t.Run(text, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			engine := activeEngine(map[string]*model.User{"merc": {Name: "merc", Hash: "hash-a"}})
			status, err := executeActiveModerationContext(t, ctx, engine, text, true)
			if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.operations) != 0 || len(engine.chats) != 0 {
				t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
			}
		})
	}
}

func TestActiveTargetModerationOperationFailureProducesNoSuccessReply(t *testing.T) {
	for _, text := range []string{"!mute merc", "!unmute hash-a", "!color merc 00ff00", "!flair merc trusted"} {
		t.Run(text, func(t *testing.T) {
			engine := activeEngine(map[string]*model.User{"merc": {Name: "merc", Hash: "hash-a"}})
			engine.err = errors.New("outbound failed")
			status, err := executeActiveModeration(t, engine, text, true)
			if status != model.FAILED || !errors.Is(err, engine.err) || len(engine.operations) != 1 || len(engine.chats) != 0 {
				t.Fatalf("status=%v err=%v chats=%v operations=%v", status, err, engine.chats, engine.operations)
			}
		})
	}
}
