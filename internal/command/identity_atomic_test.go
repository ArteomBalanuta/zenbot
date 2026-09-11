package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/sqlitefixture"
)

func TestAccessAtomicInvalidTargetAndSecondWriteFailure(t *testing.T) {
	for _, targets := range []string{"first,,second", ",first", ",,,", "first,blocked"} {
		t.Run(targets, func(t *testing.T) {
			d := sqlitefixture.Open(t, "access-atomic")
			if _, err := d.DB.Exec(`INSERT INTO trips(type,trip,created_on) VALUES('USER','first',1)`); err != nil {
				t.Fatal(err)
			}
			if _, err := d.DB.Exec(`CREATE TRIGGER rejected_grant BEFORE INSERT ON trips WHEN NEW.trip = 'blocked' BEGIN SELECT RAISE(ABORT, 'rejected grant'); END`); err != nil {
				t.Fatal(err)
			}
			e := &commandEngineStub{bundle: &service.Bundle{Security: service.NewSecurityService(&config.Config{}, d)}}
			status, err := (&accessCommand{commandBase{engine: e, message: &model.ChatMessage{Name: "admin", Trip: "admin", Text: "!access " + targets + " ADMIN"}}}).Execute(context.Background())
			if status != model.FAILED || err == nil {
				t.Errorf("status=%v err=%v", status, err)
			}
			role, err := d.ResolveRole(context.Background(), "first")
			if err != nil || role != model.USER {
				t.Fatalf("partial write: role=%v err=%v", role, err)
			}
		})
	}
}

func TestAccessAtomicExactTargetsDeduplicateAndPreserveTrailingSeparator(t *testing.T) {
	d := sqlitefixture.Open(t, "access-exact")
	e := &commandEngineStub{bundle: &service.Bundle{Security: service.NewSecurityService(&config.Config{}, d)}}
	status, err := (&accessCommand{commandBase{engine: e, message: &model.ChatMessage{Name: "admin", Trip: "admin", Text: "!access AbC123,abc123,AbC123, USER"}}}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(e.chats) != 1 || !strings.Contains(e.chats[0], "[AbC123 abc123]") {
		t.Fatalf("targets=%v", e.chats)
	}
	var count int
	if err := d.DB.QueryRow(`SELECT COUNT(*) FROM trips WHERE type='USER'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("trips=%d err=%v", count, err)
	}
}

func TestIdentityCommandsMissingStoresFailWithoutPanic(t *testing.T) {
	e := &commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{}, Security: service.NewSecurityService(&config.Config{})}}
	for alias, text := range map[string]string{"register": "!register Alice Trip", "access": "!access Trip ADMIN", "messages": "!messages Trip 3"} {
		t.Run(alias, func(t *testing.T) {
			def, _ := commandDefinitionFor(alias)
			status, err := def.New(e, &model.ChatMessage{Name: "mod", Trip: "mod", Text: text}).Execute(context.Background())
			if status != model.FAILED || err == nil {
				t.Fatalf("status=%v err=%v", status, err)
			}
		})
	}
}

func TestIdentityCommittedMutationPropagatesAcknowledgmentFailure(t *testing.T) {
	d := sqlitefixture.Open(t, "identity-ack")
	want := errors.New("ack failed")
	e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Users: &service.UserService{Identity: d}, Security: service.NewSecurityService(&config.Config{}, d)}}, sendErr: want}
	for _, text := range []string{"!register Alice Trip", "!access Trip ADMIN"} {
		def, _ := commandDefinitionFor(map[string]string{"!register Alice Trip": "register", "!access Trip ADMIN": "access"}[text])
		status, err := def.New(e, &model.ChatMessage{Name: "mod", Trip: "mod", Text: text}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, want) {
			t.Errorf("%s status=%v err=%v", text, status, err)
		}
	}
	role, err := d.ResolveRole(context.Background(), "Trip")
	if err != nil || role != model.ADMIN {
		t.Fatalf("committed role=%v err=%v", role, err)
	}
}
