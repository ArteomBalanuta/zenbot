package listener

import (
	"context"
	"encoding/json"
	"testing"

	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type subscriptionQueryStub struct {
	data   string
	calls  int
	events *[]string
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
func (r *recordingPresenceRepository) LogMessage(string, string, string, string, string) (int64, error) {
	*r.events = append(*r.events, "log")
	return 0, nil
}
func (r *recordingPresenceRepository) LogPresence(string, string, string, string, string) (int64, error) {
	return 0, nil
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

func TestUserJoinedListenerWhispersExactDataToAllCaseInsensitiveSubscribersOnly(t *testing.T) {
	q := &subscriptionQueryStub{data: `Hashes: \nhash-joined \nNicks: \nnick-joined \n`}
	e := &core.EngineImpl{
		ActiveUsers:     map[*model.User]struct{}{},
		OutMessageQueue: make(chan string, 4),
		Repository:      &repository.DummyImpl{},
		Services:        &service.Bundle{Users: &service.UserService{Queries: q}},
	}
	e.SubscribeTrip("TRIP-A")
	e.AddActiveUser(&model.User{Name: "sub-one", Trip: "trip-a"})
	e.AddActiveUser(&model.User{Name: "sub-two", Trip: "TrIp-A"})
	e.AddActiveUser(&model.User{Name: "other", Trip: "trip-b"})

	joined, _ := json.Marshal(&model.User{Name: "joined", Hash: "hash-joined", Trip: "Trip-A"})
	NewUserJoinedListener(e).Notify(string(joined))
	if q.calls != 1 {
		t.Fatalf("BasicUserData calls=%d, want 1", q.calls)
	}
	got := []string{<-e.OutMessageQueue, <-e.OutMessageQueue, <-e.OutMessageQueue}
	want := map[string]bool{
		`{ "cmd": "chat", "text": "/whisper @sub-one  -\n\nHashes: \nhash-joined \nNicks: \nnick-joined \n"}`: true,
		`{ "cmd": "chat", "text": "/whisper @sub-two  -\n\nHashes: \nhash-joined \nNicks: \nnick-joined \n"}`: true,
		`{ "cmd": "chat", "text": "/whisper @joined  -\n\nHashes: \nhash-joined \nNicks: \nnick-joined \n"}`:  true,
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

	e.UnsubscribeTrip("trip-a")
	NewUserJoinedListener(e).Notify(string(joined))
	select {
	case extra := <-e.OutMessageQueue:
		t.Fatalf("notification after unsubscribe=%q", extra)
	default:
	}
}
