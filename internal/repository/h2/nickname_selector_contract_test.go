package h2

import (
	"context"
	"testing"
)

func TestLastOnlineRawNicknameSelectorPreservesOpaqueTrips(t *testing.T) {
	d := openTestDB(t)
	_, err := d.DB.Exec(`INSERT INTO messages(name,trip,message,created_on) VALUES('merc','@Trip','normal',1),('@merc','other','at-name',2)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ raw, want string }{{"merc", "normal"}, {"@merc", "normal"}, {"@@merc", "at-name"}, {"@Trip", "normal"}} {
		got, err := d.LastOnline(context.Background(), tc.raw)
		if err != nil || !got.Found || got.LastMessage.String != tc.want {
			t.Errorf("raw=%q got=%+v err=%v", tc.raw, got, err)
		}
	}
}

func TestRemoveIdentityAcceptsMentionWithoutChangingOpaqueTrip(t *testing.T) {
	for _, raw := range []string{"merc", "@merc", "@Trip"} {
		t.Run(raw, func(t *testing.T) {
			d := openTestDB(t)
			for _, q := range []string{`INSERT INTO names(name,created_on) VALUES('merc',1)`, `INSERT INTO trips(type,trip,created_on) VALUES('USER','@Trip',1)`, `INSERT INTO trip_names(name_id,trip_id) SELECT n.id,t.id FROM names n,trips t`} {
				if _, err := d.DB.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			got, err := d.DeleteIdentityAuthorized(context.Background(), raw)
			if err != nil || got.TripNamesRows != 1 || got.NameRows != 1 || got.TripRows != 1 {
				t.Fatalf("removed=%+v err=%v", got, err)
			}
		})
	}
}
