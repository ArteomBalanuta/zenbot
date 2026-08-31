package h2

import (
	"testing"

	"zenbot/internal/model"
)

func TestIdentityRegistrationFourCasesAndAtomicDuplicateBehavior(t *testing.T) {
	d := openTestDB(t)
	if err := d.Register("Alice", "trip-a", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	name, _ := d.IsNameRegistered("alice")
	trip, _ := d.IsTripRegistered("TRIP-A")
	if !name || !trip {
		t.Fatalf("registered identity missing: name=%v trip=%v", name, trip)
	}
	if err := d.RegisterNameByTrip("Bob", "trip-a"); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterTripByName("Alice", "trip-b"); err != nil {
		t.Fatal(err)
	}
	if err := d.Register("Alice", "trip-c", model.REGULAR); err == nil {
		t.Fatal("duplicate name unexpectedly succeeded")
	}
	if err := d.Register("Carol", "trip-a", model.REGULAR); err == nil {
		t.Fatal("duplicate trip unexpectedly succeeded")
	}
	var links int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trip_names").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 3 {
		t.Fatalf("links=%d, want 3", links)
	}
}

func TestLastMessagesOrderingFilteringLimitAndEscapingData(t *testing.T) {
	d := openTestDB(t)
	for _, row := range []struct {
		trip, name, text string
		ts               int64
	}{
		{"trip", "alice", "old", 1}, {"trip", "bob", "new \\\"quoted\\\"", 4},
		{"trip", "alice", "JOINED", 5}, {"other", "alice", "name-match", 3},
	} {
		if _, err := d.DB.Exec("INSERT INTO messages(trip,name,message,created_on,visibility) VALUES($1,$2,$3,$4,'PUBLIC')", row.trip, row.name, row.text, row.ts); err != nil {
			t.Fatal(err)
		}
	}
	got, err := d.LastMessages("", "trip", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Message != `new \"quoted\"` || got[1].Message != "old" {
		t.Fatalf("messages=%v", got)
	}
	got, err = d.LastMessages("alice", "", 5)
	if err != nil || len(got) != 2 || got[0].Message != "name-match" || got[1].Message != "old" {
		t.Fatalf("name filter=%v err=%v", got, err)
	}
}

func TestLastMessagesExcludesWhispersAndUsesIDAsTieBreaker(t *testing.T) {
	d := openTestDB(t)
	for _, row := range []struct{ text, visibility string }{
		{"public-first", "PUBLIC"}, {"whisper-secret", "WHISPER"}, {"public-second", "PUBLIC"},
	} {
		if _, err := d.DB.Exec("INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip','alice',$1,10,$2)", row.text, row.visibility); err != nil {
			t.Fatal(err)
		}
	}
	got, err := d.LastMessages("", "trip", 2)
	if err != nil || len(got) != 2 || got[0].Message != "public-second" || got[1].Message != "public-first" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestRegistrationRollsBackPartialInsert(t *testing.T) {
	d := openTestDB(t)
	if err := d.Register("Alice", "trip", model.REGULAR); err != nil {
		t.Fatal(err)
	}
	if err := d.Register("Bob", "trip", model.REGULAR); err == nil {
		t.Fatal("duplicate trip unexpectedly succeeded")
	}
	var names, trips, links int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM names").Scan(&names); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trips").Scan(&trips); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM trip_names").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if names != 1 || trips != 1 || links != 1 {
		t.Fatalf("partial registration remained: names=%d trips=%d links=%d", names, trips, links)
	}
}
