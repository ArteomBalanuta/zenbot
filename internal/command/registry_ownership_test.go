package command

import (
	"errors"
	"testing"

	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

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
