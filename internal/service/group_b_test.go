package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"zenbot/internal/repository"
)

type groupBFake struct {
	registered []repository.SaturnRegisteredUser
	messages   []repository.SaturnLastMessage
	err        error
	gotName    *string
	gotTrip    string
	gotCount   int
}

func (f *groupBFake) DeleteIdentity(context.Context, string, string) (repository.DeleteResult, error) {
	return repository.DeleteResult{}, nil
}
func (f *groupBFake) SaturnRegisteredUsers(context.Context) ([]repository.SaturnRegisteredUser, error) {
	return f.registered, f.err
}
func (f *groupBFake) SaturnLastMessages(_ context.Context, name *string, trip string, count int) ([]repository.SaturnLastMessage, error) {
	f.gotName, f.gotTrip, f.gotCount = name, trip, count
	return f.messages, f.err
}

func TestUserServiceSaturnLastMessagesDelegatesTypedNullableInput(t *testing.T) {
	name := "Alice"
	want := []repository.SaturnLastMessage{{Name: "Alice", Message: "hello", CreatedOn: 42}}
	fake := &groupBFake{messages: want}
	got, err := (&UserService{GroupB: fake}).SaturnLastMessages(context.Background(), &name, "trip-a", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || fake.gotName != &name || fake.gotTrip != "trip-a" || fake.gotCount != 7 {
		t.Fatalf("got=%v name=%v trip=%q count=%d", got, fake.gotName, fake.gotTrip, fake.gotCount)
	}
}

func TestUserServiceSaturnLastMessagesForwardsNilNameAndNonPositiveCount(t *testing.T) {
	fake := &groupBFake{}
	_, err := (&UserService{GroupB: fake}).SaturnLastMessages(context.Background(), nil, "trip-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if fake.gotName != nil || fake.gotTrip != "trip-a" || fake.gotCount != 0 {
		t.Fatalf("name=%v trip=%q count=%d", fake.gotName, fake.gotTrip, fake.gotCount)
	}
}

func TestUserServiceSaturnLastMessagesPropagatesError(t *testing.T) {
	wantErr := errors.New("read failed")
	got, err := (&UserService{GroupB: &groupBFake{err: wantErr}}).SaturnLastMessages(context.Background(), nil, "trip-a", 1)
	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestMailServiceSaturnRegisteredUsersPreservesTypedRowsAndErrors(t *testing.T) {
	want := []repository.SaturnRegisteredUser{{Name: "Alice", Trip: "trip-a"}, {Name: "Bob", Trip: "trip-b"}}
	fake := &groupBFake{registered: want}
	got, err := (&MailService{GroupB: fake}).SaturnRegisteredUsers(context.Background())
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v err=%v", got, err)
	}
	wantErr := errors.New("directory failed")
	got, err = (&MailService{GroupB: &groupBFake{err: wantErr}}).SaturnRegisteredUsers(context.Background())
	if !errors.Is(err, wantErr) || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestLegacyServiceMethodsRemainSeparate(t *testing.T) {
	if (&UserService{}).GroupB != nil || (&MailService{}).GroupB != nil {
		t.Fatal("legacy-only services must be safe without Group B")
	}
}
