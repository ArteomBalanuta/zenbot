package common

import (
	"testing"

	"zenbot/internal/model"
)

func TestRegistryOwnershipAuditDefinitionsDoNotShareAliasStorage(t *testing.T) {
	registry := NewSaturnCommandRegistry()
	aliases := []string{"alpha", "omega"}
	definition := CommandDefinition{Canonical: "alpha", Aliases: aliases, New: func(Engine, *model.ChatMessage) SaturnCommand { return nil }}
	if err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	aliases[1] = "outside"
	lookup, ok := registry.Lookup("alpha")
	if !ok || lookup.Aliases[1] != "omega" {
		t.Errorf("caller mutation changed registry alias ownership: %+v", lookup)
	}
	lookup.Aliases[1] = "lookup-write"
	definitions := registry.Definitions()
	definitions[0].Aliases[1] = "snapshot-write"
	again, _ := registry.Lookup("alpha")
	if again.Aliases[1] != "omega" {
		t.Errorf("returned definition shares registry alias storage: %+v", again)
	}
}

type registryMapEngine struct {
	Engine
	commands map[string]CommandMetadata
}

func (e *registryMapEngine) GetEnabledCommands() *map[string]CommandMetadata { return &e.commands }

func TestRegistryOwnershipAuditMapOnlyAmbiguityFailsClosed(t *testing.T) {
	calls := 0
	constructor := func(*model.ChatMessage) Command { calls++; return nil }
	e := &registryMapEngine{commands: map[string]CommandMetadata{
		"abc": {Command: constructor}, "bca": {Command: constructor},
	}}
	for i := 0; i < 30; i++ {
		BuildCommand("cab", e, &model.ChatMessage{})
	}
	if calls != 0 {
		t.Fatalf("ambiguous fallback executed %d constructors", calls)
	}
	BuildCommand("abc", e, &model.ChatMessage{})
	if calls != 1 {
		t.Fatal("exact map alias no longer resolves")
	}
	delete(e.commands, "bca")
	BuildCommand("cab", e, &model.ChatMessage{})
	if calls != 2 {
		t.Fatal("unambiguous legacy anagram no longer resolves")
	}
	BuildCommand("missing", e, &model.ChatMessage{})
	if calls != 2 {
		t.Fatal("unknown alias executed a constructor")
	}
}

func TestRegistryOwnershipAuditSameDefinitionAnagrams(t *testing.T) {
	r := NewSaturnCommandRegistry()
	d := CommandDefinition{Canonical: "abc", Aliases: []string{"abc", "bca"}, New: func(Engine, *model.ChatMessage) SaturnCommand { return nil }}
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(CommandDefinition{Canonical: "cab", Aliases: []string{"cab"}, New: d.New}); err == nil {
		t.Fatal("cross-owner collision accepted")
	}
	if len(r.Definitions()) != 1 {
		t.Fatal("collision changed inventory")
	}
}
