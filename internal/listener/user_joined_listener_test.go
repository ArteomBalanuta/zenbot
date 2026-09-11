package listener

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type subscriptionQueryStub struct {
	data        string
	calls       int
	events      *[]string
	recentNames []string
}
type recordingJoinAutomation struct {
	engine       *core.EngineImpl
	calls        int
	activeAtCall bool
	events       *[]string
	event        string
}
type recordingPresenceRepository struct {
	events *[]string
}

func (a *recordingJoinAutomation) OnJoin(_ context.Context, u *model.User) {
	a.calls++
	a.activeAtCall = a.engine.GetActiveUserByName(u.Name) != nil
	if a.events != nil {
		*a.events = append(*a.events, a.event)
	}
}

func (s *subscriptionQueryStub) RegisteredUsers(context.Context) ([]repository.RegisteredUser, error) {
	return nil, nil
}
func (s *subscriptionQueryStub) NicksByTrip(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *subscriptionQueryStub) BasicUserData(context.Context, string, string) (string, error) {
	s.calls++
	if s.events != nil {
		*s.events = append(*s.events, "share")
	}
	return s.data, nil
}
func (s *subscriptionQueryStub) LastOnline(context.Context, string) (repository.LastOnlineRecord, error) {
	return repository.LastOnlineRecord{}, nil
}
func (s *subscriptionQueryStub) RecentPresenceNames(context.Context, string, string, int64, int) ([]string, error) {
	return append([]string(nil), s.recentNames...), nil
}
func (r *recordingPresenceRepository) LogMessage(string, string, string, string, string) (int64, error) {
	*r.events = append(*r.events, "wrong-message-log")
	return 0, nil
}
func (r *recordingPresenceRepository) LogPresence(string, string, string, string, string) (int64, error) {
	*r.events = append(*r.events, "log")
	return 0, nil
}

func TestUserJoinedListenerNotifiesSubscribersAboutUnrelatedJoiningIdentity(t *testing.T) {
	q := &subscriptionQueryStub{data: "Hashes: \nhash-joined \nNicks: \nnick-joined \n"}
	e := &core.EngineImpl{
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 2),
		Repository:      &repository.DummyImpl{},
		Services:        &service.Bundle{Users: &service.UserService{Queries: q}},
	}
	e.SubscribeTrip("subscriber-trip")
	e.AddActiveUser(&model.User{Name: "subscriber", Trip: "subscriber-trip"})

	NewUserJoinedListener(e).Notify(`{"nick":"joined","trip":"different-trip","hash":"hash-joined"}`)

	if q.calls != 1 {
		t.Fatalf("BasicUserData calls=%d, want 1", q.calls)
	}
	if payload := <-e.OutMessageQueue; !strings.Contains(payload, `/whisper @subscriber`) || !strings.Contains(payload, `hash-joined`) {
		t.Fatalf("subscriber payload=%q", payload)
	}
}

type shadowBanJoinRepository struct{ records []repository.ShadowBanRecord }

func (r *shadowBanJoinRepository) PersistShadowBanRecord(context.Context, repository.ShadowBanRecord) error {
	return nil
}
func (r *shadowBanJoinRepository) ListShadowBans(context.Context) ([]repository.ShadowBanRecord, error) {
	return append([]repository.ShadowBanRecord(nil), r.records...), nil
}
func (r *shadowBanJoinRepository) HasShadowBanMatch(ctx context.Context, trip, name, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for _, record := range r.records {
		if (strings.TrimSpace(trip) != "" && trip == record.Trip) || (strings.TrimSpace(name) != "" && name == record.Name) || (strings.TrimSpace(hash) != "" && hash == record.Hash) {
			return true, nil
		}
	}
	return false, nil
}
func (r *shadowBanJoinRepository) RemoveShadowBanBySourceTarget(context.Context, string) (int64, error) {
	return 0, nil
}
func (r *shadowBanJoinRepository) RemoveAllShadowBans(context.Context) (int64, error) { return 0, nil }

func TestUserJoinedListenerKicksMatchingShadowBannedIdentity(t *testing.T) {
	e := &core.EngineImpl{
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 1),
		Repository:      &repository.DummyImpl{},
		Services: &service.Bundle{ShadowBans: &service.ShadowBanService{Repo: &shadowBanJoinRepository{records: []repository.ShadowBanRecord{
			{Name: "raider", Hash: "known-hash"},
		}}}},
	}

	NewUserJoinedListener(e).Notify(`{"nick":"raider","hash":"known-hash","trip":"new-trip"}`)

	if payload := <-e.OutMessageQueue; payload != `{"cmd":"kick","nick":"raider"}` {
		t.Fatalf("kick payload=%q", payload)
	}
}

func TestUserJoinedListenerReportsRecentAliasesForHumanUser(t *testing.T) {
	q := &subscriptionQueryStub{recentNames: []string{"old-name", "joined", "Older-Name"}}
	e := &core.EngineImpl{
		Name:            "zenbot",
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 1),
		Repository:      &repository.DummyImpl{},
		Services:        &service.Bundle{Users: &service.UserService{Queries: q}},
	}

	NewUserJoinedListener(e).Notify(`{"nick":"joined","hash":"hash","trip":"trip"}`)

	if payload := <-e.OutMessageQueue; !strings.Contains(payload, `@joined, has been seen as: _old-name, Older-Name_ recently.`) {
		t.Fatalf("recent-alias payload=%q", payload)
	}
}
func (r *recordingPresenceRepository) LogCommand(context.Context, model.CommandAuditRecord) (int64, error) {
	return 0, nil
}
func (r *recordingPresenceRepository) Close() error { return nil }

func TestUserJoinedListenerInvokesTrustedAutomationAfterRegistrationAndIgnoresMalformed(t *testing.T) {
	e := &core.EngineImpl{ActiveUsers: map[*model.User]struct{}{}, Repository: &repository.DummyImpl{}}
	a := &recordingJoinAutomation{engine: e}
	l := NewUserJoinedListenerWithAutomation(e, a)
	l.Notify(`{"nick":"joined","trip":"trip","hash":"hash"}`)
	if a.calls != 1 || !a.activeAtCall {
		t.Fatalf("automation calls=%d active=%v", a.calls, a.activeAtCall)
	}
	l.Notify(`{`)
	if a.calls != 1 {
		t.Fatalf("malformed join invoked automation %d times", a.calls)
	}
}

func TestUserJoinedListenerRunsAutoMoveLastAfterExistingJoinEffects(t *testing.T) {
	events := []string{}
	q := &subscriptionQueryStub{events: &events}
	e := &core.EngineImpl{
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 1),
		Repository:      &recordingPresenceRepository{events: &events},
		Services:        &service.Bundle{Users: &service.UserService{Queries: q}},
	}
	e.SubscribeTrip("trip")
	semantic := &recordingJoinAutomation{engine: e, events: &events, event: "semantic"}
	autoMove := &recordingJoinAutomation{engine: e, events: &events, event: "automove"}

	NewUserJoinedListenerWithAutomations(e, semantic, autoMove).Notify(`{"nick":"joined","trip":"trip","hash":"hash"}`)

	if !semantic.activeAtCall || !autoMove.activeAtCall {
		t.Fatalf("automations must observe the registered user: semantic=%v automove=%v", semantic.activeAtCall, autoMove.activeAtCall)
	}
	want := []string{"semantic", "share", "log", "automove"}
	if len(events) != len(want) {
		t.Fatalf("join events=%v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("join events=%v, want %v", events, want)
		}
	}
}

func TestUserJoinedListenerWhispersExactDataToExactTripSubscribersOnly(t *testing.T) {
	q := &subscriptionQueryStub{data: "Hashes: \nhash-joined \nNicks: \nnick-joined \n"}
	e := &core.EngineImpl{
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 4),
		Repository:      &repository.DummyImpl{},
		Services:        &service.Bundle{Users: &service.UserService{Queries: q}},
	}
	e.SubscribeTrip("TRIP-A")
	e.AddActiveUser(&model.User{Name: "case-variant", Trip: "TrIp-A"})
	e.AddActiveUser(&model.User{Name: "other", Trip: "trip-b"})

	joined, _ := json.Marshal(&model.User{Name: "joined", Hash: "hash-joined", Trip: "TRIP-A"})
	NewUserJoinedListener(e).Notify(string(joined))
	if q.calls != 1 {
		t.Fatalf("BasicUserData calls=%d, want 1", q.calls)
	}
	got := []string{<-e.OutMessageQueue}
	want := map[string]bool{
		`{ "cmd": "chat", "text": "/whisper @joined  -\n\nHashes: \nhash-joined \nNicks: \nnick-joined \n"}`: true,
	}
	for _, payload := range got {
		if !want[payload] {
			t.Fatalf("unexpected payload=%q", payload)
		}
	}
	select {
	case extra := <-e.OutMessageQueue:
		t.Fatalf("unexpected recipient payload=%q", extra)
	default:
	}

	if e.UnsubscribeTrip("trip-a") {
		t.Fatal("case-variant trip removed another credential's subscription")
	}
	if !e.UnsubscribeTrip("TRIP-A") {
		t.Fatal("exact trip failed to unsubscribe")
	}
	NewUserJoinedListener(e).Notify(string(joined))
	select {
	case extra := <-e.OutMessageQueue:
		t.Fatalf("notification after unsubscribe=%q", extra)
	default:
	}
}
