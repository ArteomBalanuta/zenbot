package h2

import (
	"context"
	"testing"
)

func TestLastOnlineSelectsLatestNonPresenceMessageAndJoinedRow(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','older',1000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','JOINED',2000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','LEFT',3000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','latest',4000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','JOINED',5000,'PUBLIC')",
	} {
		if _, err := d.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	record, err := d.LastOnline(ctx, "merc")
	if err != nil {
		t.Fatal(err)
	}
	if !record.LastMessage.Valid || record.LastMessage.String != "latest" || !record.LastSeenMillis.Valid || record.LastSeenMillis.Int64 != 4000 {
		t.Fatalf("last message=%+v last seen=%+v", record.LastMessage, record.LastSeenMillis)
	}
	if !record.JoinedMillis.Valid || record.JoinedMillis.Int64 != 5000 {
		t.Fatalf("joined=%+v", record.JoinedMillis)
	}
}

func TestLastOnlineUsesCurrentPresenceTableForSessionJoin(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.DB.Exec("INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip','alice','hello',2000,'PUBLIC')"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.Exec("INSERT INTO user_presence_log(trip,name,event_type,created_on,channel) VALUES('trip','alice','joined',3000,'programming')"); err != nil {
		t.Fatal(err)
	}

	record, err := d.LastOnline(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !record.JoinedMillis.Valid || record.JoinedMillis.Int64 != 3000 {
		t.Fatalf("joined=%+v", record.JoinedMillis)
	}
}

func TestLastOnlineMatchesNameOrTripWithSaturnCaseSemantics(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','Merc','by-name',1000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('Trip-B','other','by-trip',2000,'PUBLIC')",
	} {
		if _, err := d.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	byName, err := d.LastOnline(ctx, "Merc")
	if err != nil || !byName.LastMessage.Valid || byName.LastMessage.String != "by-name" {
		t.Fatalf("name record=%+v err=%v", byName, err)
	}
	byTrip, err := d.LastOnline(ctx, "Trip-B")
	if err != nil || !byTrip.LastMessage.Valid || byTrip.LastMessage.String != "by-trip" {
		t.Fatalf("trip record=%+v err=%v", byTrip, err)
	}
	missing, err := d.LastOnline(ctx, "merc")
	if err != nil {
		t.Fatal(err)
	}
	if missing.LastMessage.Valid || missing.LastSeenMillis.Valid || missing.JoinedMillis.Valid {
		t.Fatalf("case-folded unexpectedly: %+v", missing)
	}
}

func TestLastOnlineReturnsEmptyRecordForAbsentTarget(t *testing.T) {
	d := openTestDB(t)
	record, err := d.LastOnline(context.Background(), "absent")
	if err != nil {
		t.Fatal(err)
	}
	if record.LastMessage.Valid || record.LastSeenMillis.Valid || record.JoinedMillis.Valid {
		t.Fatalf("record=%+v", record)
	}
}
