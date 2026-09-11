package service_test

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/sqlitefixture"
)

func TestShadowMatchAuditCorruptUnrelatedHashCannotDisableMatching(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-match-audit")
	ctx := context.Background()
	if err := db.PersistShadowBanRecord(ctx, repository.ShadowBanRecord{Name: "blocked", Trip: "ExactTrip", Hash: "raw-hash"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `INSERT INTO banned_users(name,hash,created_on) VALUES('unrelated','!bad-base64!',1)`); err != nil {
		t.Fatal(err)
	}
	svc := &service.ShadowBanService{Repo: db}
	for _, user := range []model.User{{Name: "blocked"}, {Trip: "ExactTrip"}, {Hash: "raw-hash"}} {
		matched, err := svc.Matches(ctx, &user)
		if err != nil || !matched {
			t.Errorf("unrelated corrupt record disabled known matching identity: user=%+v match=%v err=%v", user, matched, err)
		}
	}
	if _, err := svc.List(ctx); err == nil {
		t.Fatal("rich listing silently hid corrupt stored hash")
	}
}

func TestShadowMatchAuditUsesOnlyNonblankExactOwnColumnIdentities(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-match-exact-columns")
	ctx := context.Background()
	for _, record := range []repository.ShadowBanRecord{
		{Name: "Needle", Trip: "ExactTrip", Hash: "raw-hash"},
		{Name: "x' OR '1'='1"},
		{Name: " ", Trip: " ", Hash: " "},
	} {
		if err := db.PersistShadowBanRecord(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	svc := &service.ShadowBanService{Repo: db}
	for _, tc := range []struct {
		name string
		user *model.User
		want bool
	}{
		{"name", &model.User{Name: "Needle"}, true},
		{"trip", &model.User{Trip: "ExactTrip"}, true},
		{"hash", &model.User{Hash: "raw-hash"}, true},
		{"SQL-looking literal", &model.User{Name: "x' OR '1'='1"}, true},
		{"SQL-looking absent", &model.User{Name: "' OR 1=1 --"}, false},
		{"name case", &model.User{Name: "needle"}, false},
		{"trip case", &model.User{Trip: "exacttrip"}, false},
		{"hash case", &model.User{Hash: "RAW-HASH"}, false},
		{"name is not trip", &model.User{Name: "ExactTrip"}, false},
		{"trip is not name", &model.User{Trip: "Needle"}, false},
		{"hash is not name", &model.User{Hash: "Needle"}, false},
		{"name is not hash", &model.User{Name: "raw-hash"}, false},
		{"no wildcard", &model.User{Name: "%"}, false},
		{"no trimming", &model.User{Trip: " ExactTrip "}, false},
		{"missing", &model.User{Name: "other", Trip: "other", Hash: "other"}, false},
		{"blank", &model.User{Name: " ", Trip: " ", Hash: " "}, false},
		{"empty", &model.User{}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := svc.Matches(ctx, tc.user)
			if err != nil || matched != tc.want {
				t.Fatalf("matched=%v want=%v err=%v", matched, tc.want, err)
			}
		})
	}
}

func TestShadowMatchAuditPreservesCancellationDatabaseErrorsAndEmptyControls(t *testing.T) {
	db := sqlitefixture.Open(t, "shadow-match-source-errors")
	svc := &service.ShadowBanService{Repo: db}
	if matched, err := svc.Matches(context.Background(), &model.User{Name: "absent"}); err != nil || matched {
		t.Fatalf("empty database matched=%v err=%v", matched, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if matched, err := svc.Matches(ctx, &model.User{Name: "absent"}); !errors.Is(err, context.Canceled) || matched {
		t.Fatalf("canceled query matched=%v err=%v", matched, err)
	}
	if err := db.DB.Close(); err != nil {
		t.Fatal(err)
	}
	if matched, err := svc.Matches(context.Background(), &model.User{Name: "absent"}); err == nil || matched {
		t.Fatalf("closed database matched=%v err=%v", matched, err)
	}
	var unavailable *service.ShadowBanService
	if matched, err := unavailable.Matches(context.Background(), nil); err != nil || matched {
		t.Fatalf("nil user matched=%v err=%v", matched, err)
	}
	if matched, err := unavailable.Matches(context.Background(), &model.User{Name: "absent"}); err == nil || matched {
		t.Fatalf("missing repository matched=%v err=%v", matched, err)
	}
}
