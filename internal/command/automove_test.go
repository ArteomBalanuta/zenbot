package command

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type autoMoveControllerStub struct {
	*commandEngineStub
	snapshot       common.AutoMoveSnapshot
	enableCalls    int
	disableCalls   int
	configureCalls int
}

func (s *autoMoveControllerStub) AutoMoveSnapshot() common.AutoMoveSnapshot { return s.snapshot }
func (s *autoMoveControllerStub) ConfigureAutoMove(source, destination string) (common.AutoMoveSnapshot, error) {
	s.configureCalls++
	s.snapshot.Sources = append(s.snapshot.Sources, source)
	s.snapshot.Destination = destination
	return s.snapshot, nil
}
func (s *autoMoveControllerStub) EnableAutoMove(context.Context) (common.AutoMoveSnapshot, error) {
	s.enableCalls++
	s.snapshot.Enabled = true
	return s.snapshot, nil
}
func (s *autoMoveControllerStub) DisableAutoMove(context.Context) (common.AutoMoveSnapshot, error) {
	s.disableCalls++
	s.snapshot.Enabled = false
	return s.snapshot, nil
}

func TestAutoMoveDefaultUsageIsConcreteModeratorCommand(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{}}
	definition, ok := commandDefinitionFor("automove")
	if !ok {
		t.Fatal("automove definition missing")
	}
	command := definition.New(e, &model.ChatMessage{Text: "!automove", Name: "moderator"})
	if _, generic := command.(*saturnCommand); generic {
		t.Fatal("automove must be a concrete moderator command")
	}
	status, err := command.Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("status=%v err=%v", status, err)
	}
	want := []string{
		"moderator|!automove [on|off]|false",
		"moderator|Current status: false , Source rooms: [purgatory] , Destination room: lounge|false",
		"moderator|To set source: hell, destination: heaven - use: !automove hell heaven|false",
	}
	if len(e.chats) != len(want) {
		t.Fatalf("chats=%v, want %v", e.chats, want)
	}
	for i := range want {
		if e.chats[i] != want[i] {
			t.Fatalf("chat[%d]=%q want %q", i, e.chats[i], want[i])
		}
	}
}

func TestAutoMoveToggleUsesControllerBeforeArityAndIgnoresTrailingArguments(t *testing.T) {
	e := &autoMoveControllerStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		snapshot:          common.AutoMoveSnapshot{Sources: []string{"purgatory"}, Destination: "lounge"},
	}
	definition, ok := commandDefinitionFor("automove")
	if !ok {
		t.Fatal("automove definition missing")
	}
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove on ignored", Name: "moderator"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if e.enableCalls != 1 || e.disableCalls != 0 || e.configureCalls != 0 {
		t.Fatalf("enable=%d disable=%d configure=%d", e.enableCalls, e.disableCalls, e.configureCalls)
	}
	if len(e.chats) != 1 || e.chats[0] != "moderator| !automove is enabled|false" {
		t.Fatalf("chats=%v", e.chats)
	}
}

func TestAutoMoveOffUsesControllerAndRepliesDisabled(t *testing.T) {
	e := &autoMoveControllerStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		snapshot:          common.AutoMoveSnapshot{Enabled: true, Sources: []string{"purgatory"}, Destination: "lounge"},
	}
	definition, _ := commandDefinitionFor("automove")
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove OFF ignored", Name: "moderator"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if e.enableCalls != 0 || e.disableCalls != 1 || e.configureCalls != 0 {
		t.Fatalf("enable=%d disable=%d configure=%d", e.enableCalls, e.disableCalls, e.configureCalls)
	}
	if len(e.chats) != 1 || e.chats[0] != "moderator| !automove is disabled|false" {
		t.Fatalf("chats=%v", e.chats)
	}
}

func TestAutoMoveTwoArgumentsConfiguresAdditivelyAndRepliesSnapshot(t *testing.T) {
	e := &autoMoveControllerStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		snapshot:          common.AutoMoveSnapshot{Sources: []string{"purgatory"}, Destination: "lounge"},
	}
	definition, _ := commandDefinitionFor("automove")
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove hell heaven", Name: "moderator"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if e.enableCalls != 0 || e.disableCalls != 0 || e.configureCalls != 1 {
		t.Fatalf("enable=%d disable=%d configure=%d", e.enableCalls, e.disableCalls, e.configureCalls)
	}
	if len(e.chats) != 1 || e.chats[0] != "moderator|Set source channel: [purgatory hell] , destination channel: heaven. Make sure bot's REPLICA is serving source channels.|false" {
		t.Fatalf("chats=%v", e.chats)
	}
}

func TestAutoMoveBadArityUsesSnapshotShapedUsageReplies(t *testing.T) {
	e := &autoMoveControllerStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		snapshot:          common.AutoMoveSnapshot{Enabled: true, Sources: []string{"hell", "purgatory"}, Destination: "heaven"},
	}
	definition, _ := commandDefinitionFor("automove")
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove hell", Name: "moderator"}).Execute(context.Background())
	if err != nil || status != model.FAILED {
		t.Fatalf("status=%v err=%v", status, err)
	}
	want := []string{
		"moderator|!automove [on|off]|false",
		"moderator|Current status: true , Source rooms: [hell purgatory] , Destination room: heaven|false",
		"moderator|To set source: hell, destination: heaven - use: !automove hell heaven|false",
	}
	if e.enableCalls != 0 || e.disableCalls != 0 || e.configureCalls != 0 || len(e.chats) != len(want) {
		t.Fatalf("enable=%d disable=%d configure=%d chats=%v", e.enableCalls, e.disableCalls, e.configureCalls, e.chats)
	}
	for i := range want {
		if e.chats[i] != want[i] {
			t.Fatalf("chat[%d]=%q want %q", i, e.chats[i], want[i])
		}
	}
}

func TestAutoMovePreCancelledContextHasNoControllerCallOrReply(t *testing.T) {
	e := &autoMoveControllerStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		snapshot:          common.AutoMoveSnapshot{Sources: []string{"purgatory"}, Destination: "lounge"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	definition, _ := commandDefinitionFor("automove")
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove on", Name: "moderator"}).Execute(ctx)
	if err != context.Canceled || status != model.FAILED {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if e.enableCalls != 0 || e.disableCalls != 0 || e.configureCalls != 0 || len(e.chats) != 0 {
		t.Fatalf("enable=%d disable=%d configure=%d chats=%v", e.enableCalls, e.disableCalls, e.configureCalls, e.chats)
	}
}

func TestAutoMoveWithoutControllerFailsClosedWithoutReply(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{}}
	definition, _ := commandDefinitionFor("automove")
	status, err := definition.New(e, &model.ChatMessage{Text: "!automove on", Name: "moderator"}).Execute(context.Background())
	if err == nil || status != model.FAILED || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestAutoMoveRegistersOnlyWithControllerAsSoleModeratorAlias(t *testing.T) {
	without := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	if _, ok := without.commands["automove"]; ok {
		t.Fatal("automove registered without controller")
	}
	with := &autoMoveControllerStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}, snapshot: common.AutoMoveSnapshot{Sources: []string{"purgatory"}, Destination: "lounge"}}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	metadata, ok := with.commands["automove"]
	if !ok {
		t.Fatal("automove not registered with controller")
	}
	registered := metadata.Command(&model.ChatMessage{})
	if role := registered.GetRole(); role == nil || *role != model.MODERATOR {
		t.Fatalf("role=%v", role)
	}
	if aliases := registered.GetAliases(); len(aliases) != 1 || aliases[0] != "automove" {
		t.Fatalf("aliases=%v", aliases)
	}
	if len(with.commands) != len(without.commands)+1 {
		t.Fatalf("registered commands=%d baseline=%d", len(with.commands), len(without.commands))
	}
}
