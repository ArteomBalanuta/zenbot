package h2

import (
	"context"
	"testing"
)

func TestLastOnlineSelectsIndependentPublicMessageAndPresence(t *testing.T) {
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
	if !record.Found || !record.LastMessage.Valid || record.LastMessage.String != "latest" || !record.LastMessageMillis.Valid || record.LastMessageMillis.Int64 != 4000 {
		t.Fatalf("last message=%+v last seen=%+v", record.LastMessage, record.LastMessageMillis)
	}
	if !record.LastPresenceMillis.Valid || record.LastPresenceMillis.Int64 != 5000 || !record.LastPresenceEvent.Valid || record.LastPresenceEvent.String != "JOINED" {
		t.Fatalf("joined=%+v", record.LastPresenceMillis)
	}
}

func TestLastOnlineUsesCurrentPresenceTableForIndependentEvent(t *testing.T) {
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
	if !record.Found || !record.LastPresenceMillis.Valid || record.LastPresenceMillis.Int64 != 3000 || !record.LastPresenceEvent.Valid || record.LastPresenceEvent.String != "JOINED" {
		t.Fatalf("joined=%+v", record.LastPresenceMillis)
	}
}

func TestLastOnlineMatchesExactNameOrTripAcrossRepeatedLookups(t *testing.T) {
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
	if err != nil || !byName.Found || !byName.LastMessage.Valid || byName.LastMessage.String != "by-name" {
		t.Fatalf("name record=%+v err=%v", byName, err)
	}
	byTrip, err := d.LastOnline(ctx, "Trip-B")
	if err != nil || !byTrip.Found || !byTrip.LastMessage.Valid || byTrip.LastMessage.String != "by-trip" {
		t.Fatalf("trip record=%+v err=%v", byTrip, err)
	}
	missing, err := d.LastOnline(ctx, "merc")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Found || missing.LastMessage.Valid || missing.LastMessageMillis.Valid || missing.LastPresenceMillis.Valid {
		t.Fatalf("case-folded unexpectedly: %+v", missing)
	}
}

func TestLastOnlineReturnsEmptyRecordForAbsentTarget(t *testing.T) {
	d := openTestDB(t)
	record, err := d.LastOnline(context.Background(), "absent")
	if err != nil {
		t.Fatal(err)
	}
	if record.Found || record.LastMessage.Valid || record.LastMessageMillis.Valid || record.LastPresenceMillis.Valid {
		t.Fatalf("record=%+v", record)
	}
}
