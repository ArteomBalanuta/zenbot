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
	data  string
	calls int
}

func (s *subscriptionQueryStub) RegisteredUsers(context.Context) ([]repository.RegisteredUser, error) {
	return nil, nil
}
func (s *subscriptionQueryStub) NicksByTrip(context.Context, string) ([]string, error) {
	return nil, nil
}
func (s *subscriptionQueryStub) BasicUserData(context.Context, string, string) (string, error) {
	s.calls++
	return s.data, nil
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
