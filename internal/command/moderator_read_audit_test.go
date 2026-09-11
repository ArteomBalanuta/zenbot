package command

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestModeratorReadAuditActivityErrorIsNotSuccessfulData(t *testing.T) {
	want := errors.New("private-driver-diagnostic")
	repo := &activityCommandRepositoryStub{err: want}
	engine := &commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}
	def, _ := commandDefinitionFor("active")
	status, err := def.New(engine, &model.ChatMessage{Name: "mod", Text: "!active Trip"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, want) {
		t.Errorf("activity read failure became success: status=%v err=%v", status, err)
	}
	if strings.Contains(strings.Join(engine.chats, "\n"), want.Error()) {
		t.Error("raw database diagnostic leaked into chat result")
	}
}

func TestModeratorReadAuditActivityDeliveryFailureIsReturned(t *testing.T) {
	want := errors.New("activity delivery failed")
	repo := &activityCommandRepositoryStub{}
	engine := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Activity: &service.ActivityService{Repo: repo}}}, sendErr: want}
	def, _ := commandDefinitionFor("active")
	status, err := def.New(engine, &model.ChatMessage{Name: "mod", Text: "!active Trip"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, want) || repo.calls != 1 {
		t.Fatalf("activity delivery failure hidden or query repeated: status=%v err=%v queries=%d", status, err, repo.calls)
	}
}

func TestModeratorReadAuditMessagesRejectNonpositiveCountBeforeQuery(t *testing.T) {
	for _, count := range []string{"0", "-1"} {
		t.Run(count, func(t *testing.T) {
			identity := &identityFake{lastCount: -999}
			engine := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: identity}}}
			def, _ := commandDefinitionFor("messages")
			status, err := def.New(engine, &model.ChatMessage{Name: "mod", Text: "!messages Trip " + count}).Execute(context.Background())
			if status != model.FAILED || identity.lastCount != -999 {
				t.Fatalf("invalid count reached history query: status=%v err=%v count=%d", status, err, identity.lastCount)
			}
		})
	}
}

func TestModeratorReadAuditMessagesTruncateWithoutSplittingUTF8(t *testing.T) {
	identity := &identityFake{messages: []model.Message{{Name: "alice", Trip: "Trip", Message: strings.Repeat("a", 199) + "éTAIL"}}}
	engine := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: identity}}}
	def, _ := commandDefinitionFor("messages")
	status, err := def.New(engine, &model.ChatMessage{Name: "mod", Text: "!messages Trip 1"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(engine.chats) != 1 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
	}
	parts := strings.SplitN(engine.chats[0], "|", 3)
	body, err := strconv.Unquote("\"" + parts[1] + "\"")
	if err != nil || !utf8.ValidString(body) {
		t.Fatalf("history truncation corrupted UTF-8: body=%q err=%v", body, err)
	}
}

type moderatorReadCancelHistory struct {
	*identityFake
	cancel context.CancelFunc
}

func (r moderatorReadCancelHistory) LastMessages(context.Context, string, string, int) ([]model.Message, error) {
	r.cancel()
	return []model.Message{{Name: "alice", Trip: "Trip", Message: "observed"}}, nil
}

func TestModeratorReadAuditMessagesStopsAfterLookupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	identity := moderatorReadCancelHistory{identityFake: &identityFake{}, cancel: cancel}
	engine := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: identity}}}
	def, _ := commandDefinitionFor("messages")
	status, err := def.New(engine, &model.ChatMessage{Name: "mod", Text: "!messages Trip 1"}).Execute(ctx)
	if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.chats) != 0 {
		t.Fatalf("canceled history read sent output: status=%v err=%v chats=%v", status, err, engine.chats)
	}
}
