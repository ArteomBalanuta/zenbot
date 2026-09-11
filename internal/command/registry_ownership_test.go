package command

import (
	"context"
	"errors"
	"testing"

	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

func TestRegistryOwnershipAuditGhostUnlockDefinitionsFailClosed(t *testing.T) {
	for _, alias := range []string{"unlock", "unlockroom", "UNLOCK", "UNLOCKROOM"} {
		t.Run(alias, func(t *testing.T) {
			if definition, ok := commandDefinitionFor(alias); ok || definition.New != nil {
				t.Fatalf("unregistered alias %q exposed an executable definition", alias)
			}
		})
	}
}

type registryUnlockAuditEngine struct {
	*commandEngineStub
	registry       common.RuntimeCommandRegistry
	unlockContexts []context.Context
}

func (e *registryUnlockAuditEngine) RegisterCommand(c common.Command) error {
	return e.registry.Register(c, e)
}
func (e *registryUnlockAuditEngine) LookupCommand(alias string) (common.CommandMetadata, bool) {
	return e.registry.Lookup(alias)
}
func (e *registryUnlockAuditEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	snapshot := e.registry.Snapshot()
	return &snapshot
}
func (e *registryUnlockAuditEngine) UnlockRoom(ctx context.Context) error {
	e.unlockContexts = append(e.unlockContexts, ctx)
	return nil
}

func TestRegistryOwnershipAuditRegisteredLockOffUsesTypedUnlock(t *testing.T) {
	e := &registryUnlockAuditEngine{commandEngineStub: &commandEngineStub{}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"unlock", "unlockroom"} {
		if common.BuildCommand(alias, e, moderationMessage("!"+alias, false)) != nil {
			t.Fatalf("ghost alias %q was registered", alias)
		}
	}
	command := common.BuildCommand("lock", e, moderationMessage("!lock off", false))
	if command == nil {
		t.Fatal("registered lock command is missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status, err := common.InvokeCommand(ctx, e, command)
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("lock off status=%v err=%v", status, err)
	}
	if len(e.unlockContexts) != 1 || e.unlockContexts[0] != ctx || len(e.raws) != 0 {
		t.Fatalf("lock off did not use exactly one typed unlock with its context: contexts=%v raw=%v", e.unlockContexts, e.raws)
	}
}

func TestRegistryOwnershipAuditRegisterAllIsAtomic(t *testing.T) {
	r := common.NewSaturnCommandRegistry()
	if err := r.Register(def("held", []string{"ping"}, model.REGULAR)); err != nil {
		t.Fatal(err)
	}
	if err := RegisterAll(r); err == nil {
		t.Fatal("catalog collision accepted")
	}
	if got := r.Definitions(); len(got) != 1 || got[0].Canonical != "held" {
		t.Fatalf("failed catalog registration published partial inventory: %+v", got)
	}
}

type rejectingRegistrationEngine struct {
	*commandEngineStub
	err       error
	calls     int
	failAlias string
}

func (e *rejectingRegistrationEngine) RegisterCommand(c common.Command) error {
	e.calls++
	if e.failAlias != "" && c.GetAliases()[0] != e.failAlias {
		return e.commandEngineStub.RegisterCommand(c)
	}
	return e.err
}

func TestRegistryOwnershipAuditDirectLRegistrationError(t *testing.T) {
	want := errors.New("direct l rejected")
	e := &rejectingRegistrationEngine{commandEngineStub: &commandEngineStub{}, err: want, failAlias: "l"}
	if err := RegisterUserUtilitiesWithDirectAgent(e, &recordingDirectAgentSubmitter{}); !errors.Is(err, want) {
		t.Fatalf("got %v, want direct l registration error", err)
	}
	if _, ok := (*e.GetEnabledCommands())["l"]; ok {
		t.Fatal("failed direct l was published")
	}
	if _, ok := (*e.GetEnabledCommands())["help"]; !ok {
		t.Fatal("earlier valid command was erased")
	}
}

func TestRegistryOwnershipAuditInvalidMaterializationHasNoInventory(t *testing.T) {
	for _, aliases := range [][]string{{"abc"}, {"bca"}, {""}, nil} {
		entries := []commandcatalog.Entry{
			{Canonical: "abc", Aliases: []string{"abc"}},
			{Canonical: "other", Aliases: aliases},
		}
		if registry, err := materializeCatalog(entries); err == nil || registry != nil {
			t.Fatalf("invalid aliases %v exposed inventory or lost error: %v %v", aliases, registry, err)
		}
	}
}

func TestRegistryOwnershipAuditProductionPropagatesRegistrationError(t *testing.T) {
	want := errors.New("registration rejected")
	e := &rejectingRegistrationEngine{commandEngineStub: &commandEngineStub{}, err: want}
	if err := RegisterUserUtilities(e); !errors.Is(err, want) {
		t.Fatalf("got %v, want registration error", err)
	}
	if e.calls != 1 {
		t.Fatalf("continued publishing after failure: %d calls", e.calls)
	}
}

func TestRegistryOwnershipAuditDefinitionFactoryDoesNotShareAliases(t *testing.T) {
	aliases := []string{"ping"}
	d := def("ping", aliases, model.REGULAR)
	aliases[0] = "outside"
	d.Aliases[0] = "snapshot"
	c := d.New(&commandEngineStub{}, &model.ChatMessage{})
	if got := c.Aliases(); len(got) != 1 || got[0] != "ping" {
		t.Fatalf("mutable definition rewrote factory aliases: %v", got)
	}
}
