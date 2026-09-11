package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func accessParityEngine(auth *authFake) *commandEngineStub {
	return &commandEngineStub{
		bundle: &service.Bundle{
			Users:    &service.UserService{GroupB: &groupBHistoryFake{}},
			Security: service.NewSecurityService(&config.Config{}, auth),
		},
		users: map[string]*model.User{"admin": {Name: "admin", Trip: "admin-trip"}},
	}
}

func TestAccessParityTracer1CatalogDispatchesGrantThroughAdminGate(t *testing.T) {
	auth := &authFake{}
	e := accessParityEngine(auth)
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	registered, ok := (*e.GetEnabledCommands())["grant"]
	if !ok {
		t.Fatal("grant alias was not registered")
	}
	command := registered.Command(&model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!grant target ADMIN"})
	if command.GetRole() == nil || *command.GetRole() != model.ADMIN {
		t.Fatalf("grant role=%v, want ADMIN", command.GetRole())
	}

	payload, err := json.Marshal(model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!grant target ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	listenerNotify(t, e, payload)
	if e.authorizationCalls != 1 || len(auth.granted) != 1 || auth.granted[0] != "target:Admin" {
		t.Fatalf("authorization calls=%d grants=%v", e.authorizationCalls, auth.granted)
	}
}

func TestAccessParityTracer2AliasesGrantAndWhisperExactReply(t *testing.T) {
	for _, alias := range []string{"grant", "access"} {
		t.Run(alias, func(t *testing.T) {
			auth := &authFake{}
			e := accessParityEngine(auth)
			if err := RegisterUserUtilities(e); err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!" + alias + " target ADMIN", Type: "whisper"})
			if err != nil {
				t.Fatal(err)
			}
			listenerNotify(t, e, payload)
			if len(auth.granted) != 1 || auth.granted[0] != "target:Admin" {
				t.Fatalf("grants=%v", auth.granted)
			}
			want := "admin|\\n Granted new Role: ADMIN to trip: target|true"
			if len(e.chats) != 1 || e.chats[0] != want {
				t.Fatalf("chats=%v want=%q", e.chats, want)
			}
		})
	}
}

func TestAccessParityTracer3InputFailuresMatchSaturn(t *testing.T) {
	for _, message := range []*model.ChatMessage{
		{Name: "admin", Text: "!grant target ADMIN"},
		{Name: "admin", Trip: "admin-trip", Text: "!grant target"},
	} {
		auth := &authFake{}
		e := accessParityEngine(auth)
		status, err := (&accessCommand{commandBase: commandBase{engine: e, message: message}}).Execute(context.Background())
		want := "admin|\\n Set your trip first. Example: !grant 8Wotmg ADMIN|false"
		if status != model.FAILED || err != nil || len(auth.granted) != 0 || len(e.chats) != 1 || e.chats[0] != want {
			t.Fatalf("message=%+v status=%v err=%v grants=%v chats=%v", message, status, err, auth.granted, e.chats)
		}
	}
	auth := &authFake{}
	e := accessParityEngine(auth)
	status, err := (&accessCommand{commandBase: commandBase{engine: e, message: &model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!grant target admin"}}}).Execute(context.Background())
	if status != model.FAILED || err != nil || len(auth.granted) != 0 || len(e.chats) != 0 {
		t.Fatalf("lowercase status=%v err=%v grants=%v chats=%v", status, err, auth.granted, e.chats)
	}
}

func TestAccessParityTracer4CommaTargetsPersistRequestedRole(t *testing.T) {
	auth := &authFake{}
	e := accessParityEngine(auth)
	status, err := (&accessCommand{commandBase: commandBase{engine: e, message: &model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!access first,second, ADMIN"}}}).Execute(context.Background())
	wantGrants := []string{"first:Admin", "second:Admin"}
	wantReply := "admin|\\n Granted new Roles: ADMIN to trips: [first second]|false"
	if status != model.SUCCESSFUL || err != nil || len(auth.granted) != len(wantGrants) || len(e.chats) != 1 || e.chats[0] != wantReply {
		t.Fatalf("status=%v err=%v grants=%v chats=%v", status, err, auth.granted, e.chats)
	}
	for i, want := range wantGrants {
		if auth.granted[i] != want {
			t.Fatalf("grants=%v want=%v", auth.granted, wantGrants)
		}
	}
}

func TestAccessParityTracer5NormalGrantErrorDoesNotReply(t *testing.T) {
	errWant := errors.New("grant failed")
	auth := &authFake{err: errWant}
	e := accessParityEngine(auth)
	status, err := (&accessCommand{commandBase: commandBase{engine: e, message: &model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!grant target ADMIN"}}}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, errWant) || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestAccessParityTracer6DoesNotExposeWithoutWritableAuthorization(t *testing.T) {
	e := &commandEngineStub{
		bundle: &service.Bundle{
			Users:    &service.UserService{GroupB: &groupBHistoryFake{}},
			Security: service.NewSecurityService(&config.Config{}),
		},
		users: map[string]*model.User{"admin": {Name: "admin", Trip: "admin-trip"}},
	}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"grant", "access"} {
		if _, ok := (*e.GetEnabledCommands())[alias]; ok {
			t.Fatalf("%s must not be registered without Security.Authorization", alias)
		}
	}
	payload, err := json.Marshal(model.ChatMessage{Name: "admin", Trip: "admin-trip", Text: "!grant target ADMIN"})
	if err != nil {
		t.Fatal(err)
	}
	listenerNotify(t, e, payload)
	if e.authorizationCalls != 0 || len(e.chats) != 0 {
		t.Fatalf("unavailable command dispatched: authorizationCalls=%d chats=%v", e.authorizationCalls, e.chats)
	}
}

func TestAccessAuthorizationGateDoesNotSuppressOtherGroupBCommands(t *testing.T) {
	e := &commandEngineStub{
		bundle: &service.Bundle{
			Users:    &service.UserService{GroupB: &groupBHistoryFake{}, Identity: &identityFake{}},
			Security: service.NewSecurityService(&config.Config{}),
		},
	}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"reg", "register", "messages", "lastmessages"} {
		if _, ok := (*e.GetEnabledCommands())[alias]; !ok {
			t.Fatalf("%s must retain its Group B registration without Security.Authorization", alias)
		}
	}
}

func listenerNotify(t *testing.T, e *commandEngineStub, payload []byte) {
	t.Helper()
	listener.NewUserChatListener(e).Notify(string(payload))
}
