package core

import (
	"fmt"
	"sync"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type registryAuditCommand struct {
	common.Command
	aliases []string
	build   func(common.Engine)
}

func (c *registryAuditCommand) GetAliases() []string { return c.aliases }
func (c *registryAuditCommand) NewInstance(e common.Engine, _ *model.ChatMessage) common.Command {
	if c.build != nil {
		c.build(e)
	}
	return c
}

func TestRegistryOwnershipAuditConcurrentRegistrationAndReentrantConstruction(t *testing.T) {
	engine := &EngineImpl{}
	first := &registryAuditCommand{aliases: []string{"abc"}, build: func(e common.Engine) { e.GetEnabledCommands() }}
	if err := engine.RegisterCommand(first); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := engine.RegisterCommand(&registryAuditCommand{aliases: []string{fmt.Sprintf("other%d", i)}}); err != nil {
				t.Error(err)
			}
			for n := 0; n < 20; n++ {
				if common.BuildCommand("cab", engine, &model.ChatMessage{}) != first {
					t.Error("concurrent lookup lost owner")
				}
				view := engine.GetEnabledCommands()
				delete(*view, "abc")
			}
		}(i)
	}
	wg.Wait()
}

func TestRegistryOwnershipAuditDuplicateRegistrationPreservesFirstOwner(t *testing.T) {
	engine := &EngineImpl{}
	first := &registryAuditCommand{aliases: []string{"abc"}}
	second := &registryAuditCommand{aliases: []string{"new", "abc"}}
	if err := engine.RegisterCommand(first); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterCommand(second); err == nil {
		t.Error("duplicate registration returned no error")
	}
	if got := common.BuildCommand("abc", engine, &model.ChatMessage{}); got != first {
		t.Errorf("duplicate alias replaced valid owner: got %p want %p", got, first)
	}
	if _, ok := (*engine.GetEnabledCommands())["new"]; ok {
		t.Error("failed registration partially published a new alias")
	}
}

func TestRegistryOwnershipAuditCrossOwnerAnagramsFailClosed(t *testing.T) {
	engine := &EngineImpl{}
	first := &registryAuditCommand{aliases: []string{"abc"}}
	if err := engine.RegisterCommand(first); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterCommand(&registryAuditCommand{aliases: []string{"bca"}}); err == nil {
		t.Error("anagram collision returned no error")
	}
	if _, ok := (*engine.GetEnabledCommands())["bca"]; ok {
		t.Error("cross-owner anagram was published")
	}
	if got := common.BuildCommand("cab", engine, &model.ChatMessage{}); got != first {
		t.Errorf("unambiguous compatibility lookup lost original owner: got %p want %p", got, first)
	}
}

func TestRegistryOwnershipAuditReturnedMapCannotRewriteCommands(t *testing.T) {
	engine := &EngineImpl{}
	first := &registryAuditCommand{aliases: []string{"abc"}}
	if err := engine.RegisterCommand(first); err != nil {
		t.Fatal(err)
	}
	view := engine.GetEnabledCommands()
	delete(*view, "abc")
	(*view)["injected"] = common.CommandMetadata{Command: func(*model.ChatMessage) common.Command { return first }}
	if got := common.BuildCommand("abc", engine, &model.ChatMessage{}); got != first {
		t.Error("deleting from inventory snapshot erased registered command")
	}
	if _, ok := (*engine.GetEnabledCommands())["injected"]; ok {
		t.Error("inventory snapshot injected a command into live registry")
	}
}

func TestRegistryOwnershipAuditSameOwnerAliasesAndCallerMutation(t *testing.T) {
	engine := &EngineImpl{}
	first := &registryAuditCommand{aliases: []string{" ABC ", "bca", "abc"}}
	if err := engine.RegisterCommand(first); err != nil {
		t.Fatal(err)
	}
	first.aliases[0] = "outside"
	for _, alias := range []string{"abc", "bca", "cab", " ABC "} {
		if got := common.BuildCommand(alias, engine, &model.ChatMessage{}); got != first {
			t.Errorf("alias %q lost owner", alias)
		}
	}
	if got := common.BuildCommand("outside", engine, &model.ChatMessage{}); got != nil {
		t.Fatal("caller added an alias after registration")
	}
	if got := common.BuildCommand("missing", engine, &model.ChatMessage{}); got != nil {
		t.Fatal("unknown command resolved")
	}
	if err := engine.RegisterCommand(&registryAuditCommand{aliases: []string{"valid", " "}}); err == nil {
		t.Fatal("blank alias accepted")
	}
	if _, ok := (*engine.GetEnabledCommands())["valid"]; ok {
		t.Fatal("invalid batch partially published")
	}
}
