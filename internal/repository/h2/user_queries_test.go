package h2

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/repository"
)

func TestUserQueriesPreserveSaturnRowsAndTripNormalization(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-a',1)",
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-b',2)",
		"INSERT INTO names(name,created_on) VALUES('zeta',1)",
		"INSERT INTO names(name,created_on) VALUES('alpha',2)",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-a' AND n.name='zeta'",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-b' AND n.name='alpha'",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('Trip-A','zeta','one',1,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','zeta','two',2,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','alpha','three',3,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-b','other','four',4,'PUBLIC')",
	} {
		if _, err := d.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	users, err := d.RegisteredUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantUsers := []repository.RegisteredUser{{Trip: "trip-a", Name: "zeta"}, {Trip: "trip-b", Name: "alpha"}}
	if len(users) != len(wantUsers) || users[0] != wantUsers[0] || users[1] != wantUsers[1] {
		t.Fatalf("users=%v, want %v", users, wantUsers)
	}
	nicks, err := d.NicksByTrip(ctx, "TRIP-A")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, nick := range nicks {
		seen[nick] = true
	}
	if len(nicks) != 2 || !seen["zeta"] || !seen["alpha"] {
		t.Fatalf("nicks=%v, want distinct zeta and alpha", nicks)
	}
}

func TestUserTripsUsesSaturnLoungeTripQuery(t *testing.T) {
	const want = "SELECT trip FROM trips WHERE type = 'USER';"
	if selectUserTrips != want {
		t.Fatalf("selectUserTrips = %q, want source-exact %q", selectUserTrips, want)
	}
}

func TestUserTripsReturnsOnlyUserTripsWithOriginalCase(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','Trip-Alpha',1)",
		"INSERT INTO trips(type,trip,created_on) VALUES('MODERATOR','Trip-Moderator',2)",
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-beta',3)",
		"INSERT INTO trips(type,trip,created_on) VALUES('PEST','trip-pest',4)",
	} {
		if _, err := d.DB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	trips, err := d.UserTrips(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Trip-Alpha", "trip-beta"}
	if len(trips) != len(want) {
		t.Fatalf("trips=%v, want %v", trips, want)
	}
	for i := range want {
		if trips[i] != want[i] {
			t.Fatalf("trips=%v, want %v", trips, want)
		}
	}
}

func TestUserTripsPropagatesCanceledContext(t *testing.T) {
	d := openTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := d.UserTrips(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("UserTrips error = %v, want context.Canceled", err)
	}
}
