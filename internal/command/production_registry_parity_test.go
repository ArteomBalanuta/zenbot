package command

import (
	"testing"

	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestProductionRegistryIncludesEveryInScopeSaturnCommand(t *testing.T) {
	database := h2fixture.Open(t, "production-registry-availability")
	engine := &commandEngineStub{
		users: map[string]*model.User{},
		bundle: &service.Bundle{
			Mail:  &service.MailService{DB: database.DB},
			Notes: &service.NoteService{DB: database.DB},
		},
	}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"mail", "msg", "send", "note", "save", "notes", "kick", "k", "out"} {
		if _, ok := (*engine.GetEnabledCommands())[alias]; !ok {
			t.Errorf("production alias %q is not registered", alias)
		}
	}
}

func TestEveryInScopeSaturnCommandBuildsAConcreteHandler(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{}}
	excluded := map[string]bool{"mine": true, "whiskey": true, "ws": true, "wsa": true}
	for _, entry := range commandcatalog.Entries() {
		if excluded[entry.Canonical] {
			continue
		}
		var definition common.CommandDefinition
		var ok bool
		if entry.Canonical == "l" {
			definition, ok = directLDefinition(&recordingDirectAgentSubmitter{})
		} else {
			definition, ok = commandDefinitionFor(entry.Canonical)
		}
		if !ok {
			t.Errorf("%s has no production definition", entry.Canonical)
			continue
		}
		built := definition.New(engine, &model.ChatMessage{Name: "caller", Text: "!" + entry.Canonical})
		if _, placeholder := built.(*saturnCommand); placeholder {
			t.Errorf("%s still resolves to the generic placeholder", entry.Canonical)
		}
	}
}

func TestEveryPublicSaturnCommandRequiresRegularRole(t *testing.T) {
	public := map[string]struct{}{
		"afk": {}, "ape": {}, "coin": {}, "help": {}, "crashcourse": {}, "info": {},
		"l": {}, "lastonline": {}, "nicks": {}, "list": {}, "mail": {}, "msgchannel": {},
		"note": {}, "notes": {}, "ping": {}, "users": {}, "say": {}, "sub": {}, "time": {},
		"unsub": {}, "version": {}, "weather": {},
	}
	for _, definition := range catalog() {
		if _, ok := public[definition.Canonical]; ok && definition.Role != model.REGULAR {
			t.Errorf("%s role=%v, want REGULAR", definition.Canonical, definition.Role)
		}
	}

	engine := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"say", "afk", "list"} {
		metadata := (*engine.GetEnabledCommands())[alias]
		role := metadata.Command(&model.ChatMessage{}).GetRole()
		if role == nil || *role != model.REGULAR {
			t.Errorf("live %s role=%v, want REGULAR", alias, role)
		}
	}
}

func TestBuildCommandPreservesSaturnAnagramLookup(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{}}
	definition, ok := commandDefinitionFor("ping")
	if !ok {
		t.Fatal("missing ping definition")
	}
	engine.RegisterCommand(&legacyAdapter{engine: engine, def: definition})
	if command := common.BuildCommand("gnip", engine, &model.ChatMessage{Text: "!gnip"}); command == nil {
		t.Fatal("anagram alias did not resolve")
	}
}
