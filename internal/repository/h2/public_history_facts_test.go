package h2

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

func TestPublicHistoryNicksExactPublicObservations(t *testing.T) {
	d := openTestDB(t)
	if err := d.Register(context.Background(), "registered-only", "Trip-A", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO messages(trip,name,message,created_on,visibility) VALUES
		('Trip-A','old','public',1000,'PUBLIC'),('Trip-A','old','public again',2000,'PUBLIC'),
		('Trip-A','Alpha','public',3000,'PUBLIC'),('Trip-A','alpha','public',3000,'PUBLIC'),
		('Trip-A','secret','private',9000,'WHISPER'),('Trip-A',' ','blank',9000,'PUBLIC'),
		('trip-a','lower','public',8000,'PUBLIC'),('TRIP-A','upper','public',8000,'PUBLIC')`,
		`INSERT INTO user_presence_log(trip,name,event_type,created_on,channel) VALUES
		('Trip-A','presence-only','left',4000,'elsewhere'),('Trip-A',NULL,'joined',9000,'elsewhere'),('Trip-A','ignored','typing',9000,'elsewhere')`,
	} {
		if _, err := d.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		trip string
		want []string
	}{
		{"Trip-A", []string{"presence-only", "Alpha", "alpha", "old"}},
		{"trip-a", []string{"lower"}}, {"TRIP-A", []string{"upper"}}, {"absent", nil},
	} {
		t.Run(tc.trip, func(t *testing.T) {
			got, err := d.NicksByTrip(context.Background(), tc.trip)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("nicks=%v want=%v err=%v", got, tc.want, err)
			}
		})
	}
}

func TestPublicHistoryNicksExcludeWhitespaceOnlyNames(t *testing.T) {
	d := openTestDB(t)
	for _, name := range []string{"\t\n", "\u2003", ""} {
		if _, err := d.DB.Exec(`INSERT INTO messages(trip,name,message,created_on) VALUES('trip',$1,'public',1000)`, name); err != nil {
			t.Fatal(err)
		}
	}
	got, err := d.NicksByTrip(context.Background(), "trip")
	if err != nil || len(got) != 0 {
		t.Fatalf("blank nicknames=%q err=%v", got, err)
	}
}

func TestPublicHistoryIndependentFactsBothServicePaths(t *testing.T) {
	for _, tc := range []struct{ name, rows, action, message, observed string }{
		{"join-only", `INSERT INTO user_presence_log(name,event_type,created_on) VALUES('target','JoInEd',2000)`, "joining", "No public messages found.", "00:00:02"},
		{"leave-only", `INSERT INTO user_presence_log(name,event_type,created_on) VALUES('target','left',3000)`, "leaving", "No public messages found.", "00:00:03"},
		{"leave-after-join", `INSERT INTO user_presence_log(name,event_type,created_on) VALUES('target','joined',2000),('target','left',3000)`, "leaving", "No public messages found.", "00:00:03"},
		{"legacy-presence", `INSERT INTO messages(name,message,created_on,visibility) VALUES('target','JOINED',2000,'PUBLIC'),('target','LEFT',3000,'PUBLIC'),('target','JOINED',9000,'WHISPER')`, "leaving", "No public messages found.", "00:00:03"},
		{"legacy-presence-tie", `INSERT INTO messages(name,message,created_on,visibility) VALUES('target','JOINED',3000,'PUBLIC'),('target','LEFT',3000,'PUBLIC')`, "leaving", "No public messages found.", "00:00:03"},
		{"message-tie", `INSERT INTO messages(name,message,created_on,visibility) VALUES('target','older id',4000,'PUBLIC'),('target','latest id',4000,'PUBLIC'),('target','secret',9000,'WHISPER')`, "messaging", "Last message: latest id", "00:00:04"},
		{"empty-message", `INSERT INTO messages(name,message,created_on,visibility) VALUES('target','',4000,'PUBLIC')`, "messaging", "Last message: ", "00:00:04"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := openTestDB(t)
			if _, err := d.DB.Exec(tc.rows); err != nil {
				t.Fatal(err)
			}
			primary, err := d.LastOnline(context.Background(), "target")
			if err != nil || !primary.Found {
				t.Fatalf("public facts=%+v err=%v", primary, err)
			}
			fallback, err := d.LastSeen(context.Background(), "target")
			if err != nil || primary != fallback {
				t.Fatalf("primary=%+v fallback=%+v err=%v", primary, fallback, err)
			}
			observed := primary.LastMessageMillis.Int64
			if primary.LastPresenceMillis.Valid && primary.LastPresenceMillis.Int64 > observed {
				observed = primary.LastPresenceMillis.Int64
			}
			if got := time.UnixMilli(observed).UTC().Format("15:04:05"); got != tc.observed {
				t.Fatalf("observed=%s want=%s", got, tc.observed)
			}
			for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
				svc.Now = func() time.Time { return time.Date(1970, 1, 3, 0, 0, 0, 0, time.UTC) }
				got, err := svc.LastOnline(context.Background(), "target")
				if err != nil {
					t.Errorf("history unavailable: %v", err)
					continue
				}
				want := "target — last seen " + tc.action + " · 1 Jan 00:00 UTC\n" + tc.message
				if got != want {
					t.Errorf("reply=%q want=%q", got, want)
				}
				for _, forbidden := range []string{"Session duration", "Seen active", "secret"} {
					if strings.Contains(got, forbidden) {
						t.Errorf("unsupported/private output: %q", got)
					}
				}
			}
		})
	}
}

func TestPublicHistoryIndependentTimestampsAndStablePresenceTies(t *testing.T) {
	d := openTestDB(t)
	for _, query := range []string{
		`INSERT INTO messages(name,message,created_on,visibility,channel) VALUES('target','earlier public',1000,'PUBLIC','room-a'),('target','LEFT',3000,'PUBLIC','room-a'),('target','JOINED',9000,'WHISPER','room-a')`,
		`INSERT INTO user_presence_log(name,event_type,created_on,channel) VALUES('target','left',3000,'room-b'),('target','joined',3000,'room-b'),('target','typing',9000,'room-b')`,
	} {
		if _, err := d.DB.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	assertTimes := func(messageMillis int64) {
		t.Helper()
		for _, lookup := range []func(context.Context, string) (repository.LastOnlineRecord, error){d.LastOnline, d.LastSeen} {
			record, err := lookup(context.Background(), "target")
			if err != nil || !record.LastPresenceMillis.Valid || record.LastPresenceMillis.Int64 != 3000 || !record.LastPresenceEvent.Valid || record.LastPresenceEvent.String != "JOINED" || !record.LastMessageMillis.Valid || record.LastMessageMillis.Int64 != messageMillis {
				t.Fatalf("independent timestamps=%+v err=%v", record, err)
			}
		}
	}
	assertTimes(1000)
	for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
		svc.Now = func() time.Time { return time.Date(1970, 1, 3, 0, 0, 0, 0, time.UTC) }
		got, err := svc.LastOnline(context.Background(), "target")
		if err != nil || got != "target — last seen joining · 1 Jan 00:00 UTC\nLast message · 1 Jan 00:00 UTC: earlier public" {
			t.Errorf("independent observations=%q err=%v", got, err)
		}
	}
	if _, err := d.DB.Exec(`INSERT INTO messages(name,message,created_on,visibility) VALUES('target','later public',5000,'PUBLIC')`); err != nil {
		t.Fatal(err)
	}
	assertTimes(5000)
	for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
		svc.Now = func() time.Time { return time.Date(1970, 1, 3, 0, 0, 0, 0, time.UTC) }
		got, err := svc.LastOnline(context.Background(), "target")
		if err != nil || got != "target — last seen messaging · 1 Jan 00:00 UTC\nLast presence: joined · 1 Jan 00:00 UTC\nLast message: later public" {
			t.Errorf("independent observations=%q err=%v", got, err)
		}
	}
}

func TestPublicHistoryAmbiguityUsesPublicMatchingRowSets(t *testing.T) {
	for _, tc := range []struct {
		name, rows string
		ambiguous  bool
	}{
		{"cross-kind", `INSERT INTO messages(name,trip,message,created_on) VALUES('target','other','by name',1000),('elsewhere','target','by trip',2000)`, true},
		{"same-row", `INSERT INTO messages(name,trip,message,created_on) VALUES('target','target','same identity',1000)`, false},
		{"same-presence-row", `INSERT INTO user_presence_log(name,trip,event_type,created_on) VALUES('target','target','joined',1000)`, false},
		{"overlap-different-sets", `INSERT INTO messages(name,trip,message,created_on) VALUES('target','target','overlap',1000),('elsewhere','target','by trip',2000)`, true},
		{"private-collision", `INSERT INTO messages(name,trip,message,created_on,visibility) VALUES('target','other','public',1000,'PUBLIC'),('elsewhere','target','secret',2000,'WHISPER')`, false},
		{"current-presence-collision", `INSERT INTO user_presence_log(name,trip,event_type,created_on) VALUES('target','other','joined',1000),('elsewhere','target','left',2000)`, true},
		{"null-trip-collision", `INSERT INTO messages(name,trip,message,created_on) VALUES('target',NULL,'by name',1000),('elsewhere','target','by trip',2000)`, true},
		{"unknown-event-collision", `INSERT INTO user_presence_log(name,trip,event_type,created_on) VALUES('target','other','joined',1000),('elsewhere','target','typing',2000)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := openTestDB(t)
			if _, err := d.DB.Exec(tc.rows); err != nil {
				t.Fatal(err)
			}
			for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
				got, err := svc.LastOnline(context.Background(), "target")
				if tc.ambiguous {
					if !errors.Is(err, repository.ErrAmbiguousHistory) || err.Error() != `ambiguous public history target "target"` || got != "" {
						t.Errorf("ambiguous history=%q err=%v", got, err)
					}
				} else if err != nil {
					t.Errorf("valid history error=%v", err)
				}
			}
		})
	}
}

func TestPublicHistoryMissingAndCanceledBothPaths(t *testing.T) {
	d := openTestDB(t)
	for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
		got, err := svc.LastOnline(context.Background(), "absent")
		if got != "" || !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("missing reply=%q err=%v", got, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := svc.LastOnline(ctx, "absent"); !errors.Is(err, context.Canceled) {
			t.Errorf("canceled err=%v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.NicksByTrip(ctx, "absent"); !errors.Is(err, context.Canceled) {
		t.Errorf("nicks canceled err=%v", err)
	}
	if err := d.DB.Close(); err != nil {
		t.Fatal(err)
	}
	for _, svc := range []*service.UserService{{Queries: d}, {LastSeen: d}} {
		if got, err := svc.LastOnline(context.Background(), "absent"); err == nil || err.Error() != "sql: database is closed" || got != "" {
			t.Errorf("closed source reply=%q err=%v", got, err)
		}
	}
}
