package command

import (
	"context"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type deniedConfiguredTripEngine struct{ *commandEngineStub }

func (e *deniedConfiguredTripEngine) IsUserAuthorized(*model.User, *model.Role) bool {
	return false
}

func TestLegacyAdapterUsesUserTripsOnlyForSourceWhitelistedCommands(t *testing.T) {
	base := &commandEngineStub{
		bundle: &service.Bundle{Security: &service.SecurityService{UserTrips: []string{"creator-trip"}}},
	}
	engine := &deniedConfiguredTripEngine{commandEngineStub: base}
	user := &model.User{Name: "creator", Trip: "creator-trip"}

	restart, _ := commandDefinitionFor("restart")
	if !(&legacyAdapter{engine: base, def: restart}).AuthorizeWithEngine(engine, user) {
		t.Fatal("configured user trip should be allowed to run source-whitelisted restart")
	}

	sql, _ := commandDefinitionFor("sql")
	if (&legacyAdapter{engine: base, def: sql}).AuthorizeWithEngine(engine, user) {
		t.Fatal("configured user trip must not bypass authorization for admin-only sql")
	}
}

func TestRegisteredTypedCommandWritesExecutionAudit(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"alice": {Name: "alice", Trip: "trip", Hash: "hash"},
	}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	metadata, ok := (*engine.GetEnabledCommands())["say"]
	if !ok {
		t.Fatal("say was not registered")
	}
	message := &model.ChatMessage{Name: "alice", Trip: "trip", Hash: "hash", Text: "!say hello world"}
	metadata.Command(message).(*legacyAdapter).ExecuteContext(context.Background())

	if len(engine.audits) != 1 {
		t.Fatalf("audits=%v, want one record", engine.audits)
	}
	audit := engine.audits[0]
	if audit.Trip != "trip" || audit.CommandName != "[say, echo]" || audit.Arguments != "[hello, world]" || audit.Status != "SUCCESSFUL" || audit.Channel != "programming" || audit.CreatedOnMillis <= 0 {
		t.Fatalf("audit=%+v", audit)
	}
}

var _ common.ContextualCommandAuthorizer = (*legacyAdapter)(nil)
