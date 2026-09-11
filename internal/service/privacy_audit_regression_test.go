package service_test

import (
	"context"
	"strings"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestUtilityAuditPrivateMailCannotBeClaimedByNickname(t *testing.T) {
	db := h2fixture.Open(t, "audit-private-mail")
	if _, err := db.DB.Exec(`INSERT INTO mail(owner,receiver,message,status,created_on,is_whisper) VALUES('sender#trip','AbC123','secret','PENDING',1,'true')`); err != nil {
		t.Fatal(err)
	}
	mail := &service.MailService{DB: db.DB}
	for _, claimant := range []struct{ name, trip string }{
		{"AbC123", "attacker"}, {"someone", "abc123"}, {"AbC123", ""}, {"AbC123", "   "}, {"someone", "AbC12"}, {"someone", "AbC123,other"},
	} {
		pending, err := mail.Pending(context.Background(), claimant.name, claimant.trip)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 0 {
			t.Errorf("private mail leaked to name=%q trip=%q: %+v", claimant.name, claimant.trip, pending)
		}
	}
	pending, err := mail.Pending(context.Background(), "new-nickname", "AbC123")
	if err != nil || len(pending) != 1 || pending[0].Message != "secret" || !pending[0].IsWhisper {
		t.Fatalf("exact owner pending=%+v err=%v", pending, err)
	}
	if err := mail.DeliverPending(context.Background(), "new-nickname", "AbC123", func(context.Context, model.Mail) error { return nil }); err != nil {
		t.Fatal(err)
	}
	pending, err = mail.Pending(context.Background(), "new-nickname", "AbC123")
	if err != nil || len(pending) != 0 {
		t.Fatalf("delivered mail pending=%+v err=%v", pending, err)
	}
}

func TestPrivateMailQueueResolvesExactTripBeforeNickname(t *testing.T) {
	db := h2fixture.Open(t, "audit-mail-resolution")
	for _, query := range []string{
		`INSERT INTO trips(type,trip,created_on) VALUES('USER','AbC123',1),('USER','abc123',1),('USER','other',1)`,
		`INSERT INTO names(name,created_on) VALUES('Owner',1),('OwnerAlias',1),('LowerOwner',1),('AbC123',1)`,
		`INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE (t.trip='AbC123' AND n.name IN ('Owner','OwnerAlias')) OR (t.trip='abc123' AND n.name='LowerOwner') OR (t.trip='other' AND n.name='AbC123')`,
	} {
		if _, err := db.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	mail := &service.MailService{DB: db.DB}
	for _, tc := range []struct{ receiver, want string }{
		{"AbC123", "AbC123"}, {"abc123", "abc123"}, {" @oWnEr ", "AbC123"},
	} {
		got, err := mail.QueueResolved(context.Background(), "secret", "sender#trip", tc.receiver, true)
		if err != nil || got != tc.want {
			t.Errorf("receiver=%q resolved=%q want=%q err=%v", tc.receiver, got, tc.want, err)
		}
	}
	if _, err := mail.QueueResolved(context.Background(), "secret", "sender#trip", "LOWEROWNERTRIP", true); err == nil {
		t.Fatal("unknown recipient queued mail")
	}
}

func TestMailPreservesExplicitPublicVisibility(t *testing.T) {
	db := openMailGroupCDB(t)
	seedMailRecipient(t, db)
	mail := &service.MailService{DB: db}
	if err := mail.Queue(context.Background(), "public greeting", "sender#trip", "merc", false); err != nil {
		t.Fatal(err)
	}
	pending, err := mail.Pending(context.Background(), "nickname", "trip-a")
	if err != nil || len(pending) != 1 || pending[0].IsWhisper || pending[0].Message != "public greeting " {
		t.Fatalf("public pending=%+v err=%v", pending, err)
	}
}

func TestUtilityAuditLastOnlineDoesNotRevealWhispers(t *testing.T) {
	db := h2fixture.Open(t, "audit-last-online")
	if _, err := db.DB.Exec(`INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('target-trip','target','public hello',1,'PUBLIC'),('target-trip','target','!note private secret',2,'WHISPER'),('private-trip','private-only','only private secret',3,'WHISPER')`); err != nil {
		t.Fatal(err)
	}
	for _, svc := range []*service.UserService{{Queries: db}, {LastSeen: db}} {
		for _, target := range []string{"target", "target-trip"} {
			got, err := svc.LastOnline(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "private secret") || !strings.Contains(got, "public hello") {
				t.Errorf("lastonline reply: %s", got)
			}
		}
		got, _ := svc.LastOnline(context.Background(), "private-only")
		if strings.Contains(got, "private secret") {
			t.Errorf("private-only lastonline reply: %s", got)
		}
	}
	public, err := db.LastMessages(context.Background(), "target", "target-trip", 10)
	if err != nil || len(public) != 1 || public[0].Message != "public hello" {
		t.Fatalf("public history=%+v err=%v", public, err)
	}
}
