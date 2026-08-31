package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type identityFake struct {
	names, trips map[string]bool
	registered   []string
	messages     []model.Message
	err          error
	mutationErr  error
	lastCount    int
}

func (f *identityFake) IsNameRegistered(v string) (bool, error) { return f.names[v], f.err }
func (f *identityFake) IsTripRegistered(v string) (bool, error) { return f.trips[v], f.err }
func (f *identityFake) Register(n, t string, _ model.Role) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) RegisterNameByTrip(n, t string) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) RegisterTripByName(n, t string) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) LastMessages(_, _ string, count int) ([]model.Message, error) {
	f.lastCount = count
	return f.messages, f.err
}

type groupBHistoryFake struct {
	messages []repository.SaturnLastMessage
}

func (f *groupBHistoryFake) DeleteIdentity(context.Context, string, string) (repository.DeleteResult, error) {
	return repository.DeleteResult{}, nil
}
func (f *groupBHistoryFake) SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error) {
	return nil, nil
}
func (f *groupBHistoryFake) SaturnLastMessages(context.Context, *string, string, int) ([]repository.SaturnLastMessage, error) {
	return f.messages, nil
}

type authFake struct {
	granted []string
	err     error
}

func (f *authFake) IsTripAuthorized(context.Context, string, model.Role, []string) (bool, error) {
	return true, nil
}
func (f *authFake) GrantTrip(_ context.Context, t string, r model.Role) error {
	f.granted = append(f.granted, t+":"+r.String())
	return f.err
}
func (f *authFake) ResolveRole(context.Context, string) (model.Role, error) {
	return model.REGULAR, nil
}

type identityCommandEngine struct {
	*commandEngineStub
	security *service.SecurityService
}

func (e *identityCommandEngine) ServiceBundle() *service.Bundle {
	return &service.Bundle{
		Users:    e.bundle.Users,
		Security: e.security,
	}
}

func newIdentityEngine(ids *identityFake, auth *authFake) *identityCommandEngine {
	base := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: ids}}}
	return &identityCommandEngine{commandEngineStub: base, security: service.NewSecurityService(&config.Config{}, auth)}
}

func TestIdentityCommandAliasesRolesAndConcreteDispatch(t *testing.T) {
	for _, tc := range []struct {
		alias, canonical string
		role             model.Role
	}{
		{"reg", "register", model.MODERATOR},
		{"REGISTER", "register", model.MODERATOR},
		{"auth", "authorize", model.MODERATOR},
		{"grant", "access", model.ADMIN},
		{"lastmessages", "messages", model.MODERATOR},
	} {
		d, ok := commandDefinitionFor(tc.alias)
		if !ok || d.Canonical != tc.canonical || d.Role != tc.role {
			t.Fatalf("%s definition=%+v ok=%v", tc.alias, d, ok)
		}
	}

	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{})
	d, _ := commandDefinitionFor("reg")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!reg Alice Trip"}).Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL || len(ids.registered) != 1 || ids.registered[0] != "Alice:Trip" {
		t.Fatalf("register status=%v err=%v registered=%v", status, err, ids.registered)
	}
}

func TestRegisterCommandTrimsAndReportsMutationErrors(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}, mutationErr: errors.New("insert failed")}
	e := newIdentityEngine(ids, &authFake{})
	d, _ := commandDefinitionFor("register")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!register  Alice   Trip "}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, ids.mutationErr) || len(e.chats) != 1 || e.chats[0] != "mod|Something went wrong|false" {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestAuthorizeCommandPropagatesPersistenceFailure(t *testing.T) {
	errWant := errors.New("grant failed")
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	auth := &authFake{err: errWant}
	e := newIdentityEngine(ids, auth)
	d, _ := commandDefinitionFor("auth")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!auth trip"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, errWant) || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestAccessCommandPropagatesGrantFailure(t *testing.T) {
	errWant := errors.New("grant failed")
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{err: errWant})
	d, _ := commandDefinitionFor("access")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Trip: "invoker", Text: "!grant target ADMIN"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, errWant) || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestAccessCommandUsesSaturnRawCaseSensitiveRoleParsing(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	auth := &authFake{}
	e := newIdentityEngine(ids, auth)
	d, _ := commandDefinitionFor("access")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Trip: "invoker", Text: "!access target admin"}).Execute(context.Background())
	if status != model.FAILED || err != nil || len(auth.granted) != 0 || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v grants=%v chats=%v", status, err, auth.granted, e.chats)
	}
}

func TestAccessCommandCommaTargetsUseUserAndJavaSplitSemantics(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	auth := &authFake{}
	e := newIdentityEngine(ids, auth)
	d, _ := commandDefinitionFor("grant")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Trip: "invoker", Text: "!grant first,second, ADMIN"}).Execute(context.Background())
	wantGrants := []string{"first:User", "second:User"}
	wantReply := "mod|\\n Granted new Roles: ADMIN to trips: [first second]|false"
	if status != model.SUCCESSFUL || err != nil || len(auth.granted) != len(wantGrants) || len(e.chats) != 1 || e.chats[0] != wantReply {
		t.Fatalf("status=%v err=%v grants=%v chats=%v", status, err, auth.granted, e.chats)
	}
	for i := range wantGrants {
		if auth.granted[i] != wantGrants[i] {
			t.Fatalf("grants=%v want=%v", auth.granted, wantGrants)
		}
	}
}

func TestMessagesCommandEscapesQuotesAndTruncatesBytes(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}, messages: []model.Message{{Name: "alice", Trip: "trip", Message: "quote \"x\""}}}
	e := newIdentityEngine(ids, &authFake{})
	d, _ := commandDefinitionFor("lastmessages")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!lastmessages trip 1"}).Execute(context.Background())
	want := `mod|\nalice#trip: quote \"x\"\n|false`
	if status != model.SUCCESSFUL || err != nil || len(e.chats) != 1 || e.chats[0] != want {
		t.Fatalf("status=%v err=%v chats=%q", status, err, e.chats)
	}
}

func TestMessagesCommandRendersReturnedGroupBRowTrip(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{})
	e.bundle.Users.GroupB = &groupBHistoryFake{messages: []repository.SaturnLastMessage{{Name: "alice", Trip: "stored-trip", Message: "message"}}}
	d, _ := commandDefinitionFor("lastmessages")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!lastmessages requested-trip 1"}).Execute(context.Background())
	want := `mod|\nalice#stored-trip: message\n|false`
	if status != model.SUCCESSFUL || err != nil || len(e.chats) != 1 || e.chats[0] != want {
		t.Fatalf("status=%v err=%v chats=%q", status, err, e.chats)
	}
}

func TestMessagesCommandClampsCountAndParsesEveryRole(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{})
	d, _ := commandDefinitionFor("messages")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Text: "!messages trip 99"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || ids.lastCount != 30 || len(e.chats) != 2 || e.chats[0] != "mod|Retrieving at max 30 messages! |false" {
		t.Fatalf("status=%v err=%v count=%d chats=%v", status, err, ids.lastCount, e.chats)
	}
	for _, name := range []string{"ADMIN", "MODERATOR", "TRUSTED", "USER", "REGULAR", "PEST"} {
		if _, ok := parseRole(name); !ok {
			t.Fatalf("role %q rejected", name)
		}
	}
}
