package command

import (
	"context"
	"errors"
	"strings"
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

func (f *identityFake) IsNameRegistered(_ context.Context, v string) (bool, error) {
	return f.names[v], f.err
}
func (f *identityFake) IsTripRegistered(_ context.Context, v string) (bool, error) {
	return f.trips[v], f.err
}
func (f *identityFake) Register(_ context.Context, n, t string, _ model.Role) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) RegisterNameByTrip(_ context.Context, n, t string) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) RegisterTripByName(_ context.Context, n, t string) error {
	f.registered = append(f.registered, n+":"+t)
	return f.mutationErr
}
func (f *identityFake) LastMessages(_ context.Context, _, _ string, count int) ([]model.Message, error) {
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
	failAt  int
}

func (f *authFake) IsTripAuthorized(context.Context, string, model.Role, []string) (bool, error) {
	return true, nil
}
func (f *authFake) GrantTrip(_ context.Context, t string, r model.Role) error {
	f.granted = append(f.granted, t+":"+r.String())
	if f.failAt > 0 && len(f.granted) != f.failAt {
		return nil
	}
	return f.err
}
func (f *authFake) ResolveRole(context.Context, string) (model.Role, error) {
	return model.REGULAR, nil
}

func (f *authFake) GrantTrips(ctx context.Context, trips []string, role model.Role) error {
	for _, trip := range trips {
		if err := f.GrantTrip(ctx, trip, role); err != nil {
			return err
		}
	}
	return nil
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
	status, err := (&authorizeCommand{commandBase: commandBase{engine: e, message: &model.ChatMessage{Name: "mod", Text: "!auth trip"}}}).Execute(context.Background())
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

func TestAccessCommandCommaTargetsUseRequestedRoleAndJavaSplitSemantics(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	auth := &authFake{}
	e := newIdentityEngine(ids, auth)
	d, _ := commandDefinitionFor("grant")
	status, err := d.New(e, &model.ChatMessage{Name: "mod", Trip: "invoker", Text: "!grant first,second, ADMIN"}).Execute(context.Background())
	wantGrants := []string{"first:Admin", "second:Admin"}
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

func TestAccessCommandStopsCommaTargetGrantOnPersistenceFailure(t *testing.T) {
	errWant := errors.New("second grant failed")
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	auth := &authFake{err: errWant, failAt: 2}
	e := newIdentityEngine(ids, auth)
	d, _ := commandDefinitionFor("grant")

	status, err := d.New(e, &model.ChatMessage{Name: "mod", Trip: "invoker", Text: "!grant first,second,third ADMIN"}).Execute(context.Background())

	if status != model.FAILED || !errors.Is(err, errWant) {
		t.Fatalf("status=%v err=%v, want FAILED and %v", status, err, errWant)
	}
	wantGrants := []string{"first:Admin", "second:Admin"}
	if len(auth.granted) != len(wantGrants) {
		t.Fatalf("grants=%v, want %v", auth.granted, wantGrants)
	}
	for index := range wantGrants {
		if auth.granted[index] != wantGrants[index] {
			t.Fatalf("grants=%v, want %v", auth.granted, wantGrants)
		}
	}
	if len(e.chats) != 0 {
		t.Fatalf("success reply sent after failed grant: %v", e.chats)
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

type lastSeenFake struct {
	record repository.LastSeen
}

func (f *lastSeenFake) LastSeen(context.Context, string) (repository.LastSeen, error) {
	return f.record, nil
}

func TestLastOnlineRendersPersistedLastSeen(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{})
	seen, joined := int64(0), int64(0)
	e.bundle.Users.LastSeen = &lastSeenFake{record: repository.LastSeen{Message: `hello "world"`, SeenAt: &seen, JoinedAt: &joined}}
	d, _ := commandDefinitionFor("seen")
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!seen @merc"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(e.chats) != 1 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
	if got := e.chats[0]; !strings.Contains(got, "Nick|Trip: merc") || !strings.Contains(got, "Last message: hello") || !strings.Contains(got, "world") || !strings.Contains(got, "Last seen: Thu, 01 Jan 1970 00:00:00 GMT") {
		t.Fatalf("unexpected last-online response %q", got)
	}
}

func TestLastOnlineUsesSourceAliasesAndUsageInsteadOfCatalogPlaceholder(t *testing.T) {
	ids := &identityFake{names: map[string]bool{}, trips: map[string]bool{}}
	e := newIdentityEngine(ids, &authFake{})
	for _, alias := range []string{"lastonline", "seen", "last", "online", "lastseen"} {
		d, ok := commandDefinitionFor(alias)
		if !ok || d.Canonical != "lastonline" || d.Role != model.REGULAR {
			t.Fatalf("%s definition=%+v ok=%v", alias, d, ok)
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias}).Execute(context.Background())
		want := "alice|\\n Example: !lastseen merc|false"
		if status != model.FAILED || err != nil || len(e.chats) != 1 || e.chats[0] != want {
			t.Fatalf("%s status=%v err=%v chats=%v want=%q", alias, status, err, e.chats, want)
		}
		e.chats = nil
	}
}
