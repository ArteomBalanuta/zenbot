package command

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

// finalCatalogIntegrationEngine keeps registration and dispatched instances on
// the capability-bearing engine, so this test exercises the same catalog seam
// used by inbound chat rather than direct concrete-command construction only.
type finalCatalogIntegrationEngine struct {
	*commandEngineStub
	controller common.HostLifecycleController
	authorized bool
}

func (e *finalCatalogIntegrationEngine) HostLifecycleController() common.HostLifecycleController {
	return e.controller
}

func (e *finalCatalogIntegrationEngine) IsUserAuthorized(*model.User, *model.Role) bool {
	return e.authorized
}

func (e *finalCatalogIntegrationEngine) RegisterCommand(command common.Command) {
	if e.commands == nil {
		e.commands = map[string]common.CommandMetadata{}
	}
	for _, alias := range command.GetAliases() {
		registered := command
		e.commands[alias] = common.CommandMetadata{
			Alias: alias,
			Command: func(message *model.ChatMessage) common.Command {
				return registered.NewInstance(e, message)
			},
		}
	}
}

func TestSQLRestartShutdownFinalCatalogIntegration(t *testing.T) {
	aliases := []struct {
		alias string
		kind  string
	}{
		{"sql", "sql"},
		{"restart", "restart"}, {"reload", "restart"}, {"re", "restart"},
		{"exit", "shutdown"}, {"quit", "shutdown"}, {"shutdown", "shutdown"},
	}

	t.Run("dependency gating", func(t *testing.T) {
		without := &commandEngineStub{users: map[string]*model.User{}}
		if err := RegisterUserUtilities(without); err != nil {
			t.Fatal(err)
		}
		for _, entry := range aliases {
			if _, registered := (*without.GetEnabledCommands())[entry.alias]; registered {
				t.Errorf("%q registered without its concrete capability", entry.alias)
			}
		}
	})

	query := &recordingRawSQLQuery{}
	controller := &recordingLifecycleController{}
	engine := &finalCatalogIntegrationEngine{
		commandEngineStub: &commandEngineStub{
			bundle: &service.Bundle{SQLCommand: query},
			users:  map[string]*model.User{"alice": {Name: "alice", Trip: "admin"}},
		},
		controller: controller,
		authorized: true,
	}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	t.Run("exact aliases select concrete handlers", func(t *testing.T) {
		for _, entry := range aliases {
			metadata, registered := (*engine.GetEnabledCommands())[entry.alias]
			if !registered {
				t.Errorf("concrete capability omitted %q", entry.alias)
				continue
			}
			command := metadata.Command(&model.ChatMessage{Name: "alice", Text: "!" + entry.alias})
			adapter, ok := command.(*legacyAdapter)
			if !ok {
				t.Errorf("%q registration type=%T, want legacy adapter", entry.alias, command)
				continue
			}
			concrete := adapter.def.New(engine, &model.ChatMessage{Name: "alice", Text: "!" + entry.alias})
			switch entry.kind {
			case "sql":
				if _, ok := concrete.(*sqlCommand); !ok {
					t.Errorf("%q concrete type=%T, want *sqlCommand", entry.alias, concrete)
				}
			case "restart":
				if _, ok := concrete.(*restartCommand); !ok {
					t.Errorf("%q concrete type=%T, want *restartCommand", entry.alias, concrete)
				}
			case "shutdown":
				if _, ok := concrete.(*shutdownCommand); !ok {
					t.Errorf("%q concrete type=%T, want *shutdownCommand", entry.alias, concrete)
				}
			}
			if command.GetRole() == nil || *command.GetRole() != model.ADMIN {
				t.Errorf("%q role=%v, want ADMIN", entry.alias, command.GetRole())
			}
		}
	})

	t.Run("unauthorized dispatch precedes every side effect", func(t *testing.T) {
		engine.authorized = false
		defer func() { engine.authorized = true }()
		for _, entry := range aliases {
			beforeQueries := len(query.queries)
			beforeRestart, beforeShutdown := controller.restartCalls, controller.shutdownCalls
			payload, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "admin", Text: "!" + entry.alias + " SELECT 1"})
			if err != nil {
				t.Fatal(err)
			}
			listener.NewUserChatListener(engine).Notify(string(payload))
			if len(query.queries) != beforeQueries || controller.restartCalls != beforeRestart || controller.shutdownCalls != beforeShutdown {
				t.Errorf("%q side effects query=%d->%d restart=%d->%d shutdown=%d->%d", entry.alias, beforeQueries, len(query.queries), beforeRestart, controller.restartCalls, beforeShutdown, controller.shutdownCalls)
			}
		}
	})

	t.Run("agent gateway excludes every privileged alias", func(t *testing.T) {
		caller, err := api.NewContext("room", "alice", "admin", "", false, []string{})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range aliases {
			beforeQueries := len(query.queries)
			beforeRestart, beforeShutdown := controller.restartCalls, controller.shutdownCalls
			result, err := NewAgentCommandGateway(engine).Execute(context.Background(), caller, entry.alias, "SELECT 1")
			if err == nil || result.Status == commandgateway.OutcomeSucceeded {
				t.Errorf("agent gateway accepted %q: result=%+v err=%v", entry.alias, result, err)
			}
			if len(query.queries) != beforeQueries || controller.restartCalls != beforeRestart || controller.shutdownCalls != beforeShutdown {
				t.Errorf("agent gateway side effect for %q", entry.alias)
			}
		}
	})

	t.Run("generic fallback allowlist is fully finalized", func(t *testing.T) {
		for _, canonical := range []string{"sql", "restart", "shutdown"} {
			if _, allowed := allowedScopedGenericFallbacks[canonical]; allowed {
				t.Errorf("%q remains an allowed generic fallback after concrete integration", canonical)
			}
		}
	})
}
