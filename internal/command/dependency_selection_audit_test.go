package command

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/tool"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

var errSelectedNilDependency = errors.New("typed-nil repository was selected")

type selectionGroupB struct {
	repository.SqlUtilGroupBRepository
	err error
}

func (r *selectionGroupB) SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error) {
	if r == nil {
		return nil, errSelectedNilDependency
	}
	return []repository.SaturnRegisteredUser{{Name: "preferred-user", Trip: "trip"}}, r.err
}
func (r *selectionGroupB) SaturnLastMessages(context.Context, *string, string, int) ([]repository.SaturnLastMessage, error) {
	if r == nil {
		return nil, errSelectedNilDependency
	}
	return []repository.SaturnLastMessage{{Name: "preferred-user", Trip: "trip", Message: "preferred-message"}}, r.err
}

type selectionQueries struct {
	repository.UserQueryRepository
	calls int
	err   error
}

func (r *selectionQueries) RegisteredUsers(context.Context) ([]repository.RegisteredUser, error) {
	if r == nil {
		return nil, errSelectedNilDependency
	}
	r.calls++
	return []repository.RegisteredUser{{Name: "fallback-user", Trip: "trip"}}, r.err
}
func (r *selectionQueries) LastOnline(context.Context, string) (repository.LastOnlineRecord, error) {
	if r == nil {
		return repository.LastOnlineRecord{}, errSelectedNilDependency
	}
	r.calls++
	return repository.LastOnlineRecord{Found: true}, r.err
}

type selectionLastSeen struct{ calls int }

func (r *selectionLastSeen) LastSeen(context.Context, string) (repository.LastOnlineRecord, error) {
	if r == nil {
		return repository.LastOnlineRecord{}, errSelectedNilDependency
	}
	r.calls++
	return repository.LastOnlineRecord{Found: true, LastMessage: sql.NullString{String: "fallback-message", Valid: true}, LastMessageMillis: sql.NullInt64{Int64: 0, Valid: true}}, nil
}

func selectionFixture(canonical, mode string, preferredErr error) (*service.UserService, func() int, string, string) {
	var group *selectionGroupB
	if mode == "preferred" {
		group = &selectionGroupB{err: preferredErr}
	}
	s := &service.UserService{GroupB: group}
	switch canonical {
	case "users":
		var fallback *selectionQueries
		if mode != "unavailable" {
			fallback = &selectionQueries{}
		}
		s.Queries = fallback
		return s, func() int {
			if fallback == nil {
				return 0
			}
			return fallback.calls
		}, "", "fallback-user"
	case "messages":
		var fallback *identityFake
		if mode != "unavailable" {
			fallback = &identityFake{messages: []model.Message{{Name: "fallback-user", Trip: "trip", Message: "fallback-message"}}}
		}
		s.Identity = fallback
		return s, func() int {
			if fallback == nil {
				return 0
			}
			return fallback.lastCalls
		}, "trip 1", "fallback-message"
	default:
		var queries *selectionQueries
		if mode == "preferred" {
			queries = &selectionQueries{err: preferredErr}
		}
		var fallback *selectionLastSeen
		if mode != "unavailable" {
			fallback = &selectionLastSeen{}
		}
		s.Queries, s.LastSeen = queries, fallback
		return s, func() int {
			if fallback == nil {
				return 0
			}
			return fallback.calls
		}, "target", "fallback-message"
	}
}

func TestDependencySelectionAuditTypedNilPreferredUsesUsableFallback(t *testing.T) {
	for _, canonical := range []string{"users", "messages", "lastonline"} {
		t.Run(canonical, func(t *testing.T) {
			s, calls, arguments, observation := selectionFixture(canonical, "fallback", nil)
			e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: s}}}
			caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", true, []string{}, []api.Capability{api.ModerationCommands, api.AdminCommands})
			entry, _ := commandcatalog.AgentEntry(canonical)
			g := NewAgentCommandGateway(e)
			if !(tool.SaturnCommand{Definition: entry, Gateway: g}).Authorized(caller) {
				t.Fatal("usable fallback hidden")
			}
			if err := RegisterUserUtilities(e); err != nil {
				t.Fatal(err)
			}
			if _, ok := (*e.GetEnabledCommands())[canonical]; !ok {
				t.Fatal("usable fallback not registered")
			}
			result, err := g.Execute(context.Background(), caller, canonical, arguments)
			if err != nil || result.Status != commandgateway.OutcomeSucceeded || calls() != 1 || e.sends != 1 || len(e.chats) != 1 || !strings.Contains(e.chats[0], observation) || !strings.HasSuffix(e.chats[0], "|true") {
				t.Fatalf("fallback result=%+v err=%v calls=%d chats=%v", result, err, calls(), e.chats)
			}
		})
	}
}

func TestDependencySelectionAuditBothUnavailableRejectBeforeOutput(t *testing.T) {
	for _, canonical := range []string{"users", "messages", "lastonline"} {
		for _, nilKind := range []string{"typed nil", "nil"} {
			t.Run(canonical+"/"+nilKind, func(t *testing.T) {
				s, _, arguments, _ := selectionFixture(canonical, "unavailable", nil)
				if nilKind == "nil" {
					s = &service.UserService{}
				}
				if canonical == "messages" {
					arguments = "trip 31"
				}
				e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: s}}}
				caller, _ := api.NewContextWithCapabilities("room", "caller", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands, api.AdminCommands})
				g := NewAgentCommandGateway(e)
				entry, _ := commandcatalog.AgentEntry(canonical)
				if (tool.SaturnCommand{Definition: entry, Gateway: g}).Authorized(caller) {
					t.Fatal("unavailable tool visible")
				}
				if err := RegisterUserUtilities(e); err != nil {
					t.Fatal(err)
				}
				if _, ok := (*e.GetEnabledCommands())[canonical]; ok {
					t.Fatal("unavailable command registered")
				}
				result, err := g.Execute(context.Background(), caller, canonical, arguments)
				if err == nil || result.Status != commandgateway.OutcomeRejected || e.sends != 0 {
					t.Fatalf("gateway result=%+v err=%v sends=%d", result, err, e.sends)
				}
				definition, _ := commandDefinitionFor(canonical)
				status, err := definition.New(e, &model.ChatMessage{Name: "caller", Text: "!" + canonical + " " + arguments}).Execute(context.Background())
				if status != model.FAILED || err == nil || errors.Is(err, errSelectedNilDependency) || e.sends != 0 {
					t.Fatalf("source status=%v err=%v sends=%d", status, err, e.sends)
				}
			})
		}
	}
}

func TestDependencySelectionAuditCanceledFallbackHasNoEffects(t *testing.T) {
	for _, canonical := range []string{"users", "messages", "lastonline"} {
		t.Run(canonical, func(t *testing.T) {
			s, calls, arguments, _ := selectionFixture(canonical, "fallback", nil)
			e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: s}}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(e, &model.ChatMessage{Name: "caller", Text: "!" + canonical + " " + arguments}).Execute(ctx)
			if status != model.FAILED || !errors.Is(err, context.Canceled) || calls() != 0 || e.sends != 0 {
				t.Fatalf("status=%v err=%v fallbackCalls=%d sends=%d", status, err, calls(), e.sends)
			}
		})
	}
}

func TestDependencySelectionAuditPreferredErrorNeverQueriesFallback(t *testing.T) {
	want := errors.New("preferred database failed")
	for _, canonical := range []string{"users", "messages", "lastonline"} {
		t.Run(canonical, func(t *testing.T) {
			s, calls, arguments, _ := selectionFixture(canonical, "preferred", want)
			e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: s}}}
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(e, &model.ChatMessage{Name: "caller", Text: "!" + canonical + " " + arguments}).Execute(context.Background())
			if status != model.FAILED || !errors.Is(err, want) || calls() != 0 || e.sends != 0 {
				t.Fatalf("status=%v err=%v fallbackCalls=%d sends=%d", status, err, calls(), e.sends)
			}
		})
	}
}
