package h2

import (
	"testing"

	"zenbot/internal/service"
)

func TestSqlUtilRow324GroupCNotesParity(t *testing.T) {
	d := openTestDB(t)
	notes := service.NoteService{DB: d.DB}

	tripANote := "quote: \"; backslash: \\; newline:\nsecond line"
	if err := notes.Save("trip-a", tripANote); err != nil {
		t.Fatalf("save trip-a note: %v", err)
	}
	if err := notes.Save("trip-b", "trip-b only"); err != nil {
		t.Fatalf("save trip-b note: %v", err)
	}

	list, err := notes.List("trip-a")
	if err != nil {
		t.Fatalf("list trip-a notes: %v", err)
	}
	if len(list) != 1 || list[0] != `quote: \"; backslash: \\; newline:\nsecond line` {
		t.Fatalf("trip-a notes = %q, want exactly one JSON-escaped note", list)
	}

	var count int
	var createdOn int64
	if err := d.DB.QueryRow("SELECT COUNT(*), MIN(created_on) FROM notes WHERE trip = ?", "trip-a").Scan(&count, &createdOn); err != nil {
		t.Fatalf("query persisted trip-a note: %v", err)
	}
	if count != 1 {
		t.Fatalf("trip-a persisted note count = %d, want 1", count)
	}
	var persistedNote string
	if err := d.DB.QueryRow("SELECT note FROM notes WHERE trip = ?", "trip-a").Scan(&persistedNote); err != nil {
		t.Fatalf("query exact persisted trip-a note: %v", err)
	}
	if persistedNote != tripANote {
		t.Fatalf("persisted trip-a note = %q, want exact payload %q", persistedNote, tripANote)
	}
	if createdOn <= 0 {
		t.Fatalf("trip-a note created_on = %d, want greater than zero", createdOn)
	}

	otherTrip, err := notes.List("trip-c")
	if err != nil {
		t.Fatalf("list different trip: %v", err)
	}
	if len(otherTrip) != 0 {
		t.Fatalf("different trip notes = %q, want no notes", otherTrip)
	}
	quoteTrip := `trip-a" OR "1"="1`
	quoteTripNote := `quote trip note`
	if err := notes.Save(quoteTrip, quoteTripNote); err != nil {
		t.Fatalf("save quote-bearing trip: %v", err)
	}
	quoteTripNotes, err := notes.List(quoteTrip)
	if err != nil {
		t.Fatalf("list quote-bearing trip: %v", err)
	}
	if len(quoteTripNotes) != 1 || quoteTripNotes[0] != quoteTripNote {
		t.Fatalf("quote-bearing trip notes = %q, want exactly [%s]", quoteTripNotes, quoteTripNote)
	}

	if err := notes.Clear("trip-a"); err != nil {
		t.Fatalf("clear trip-a notes: %v", err)
	}
	list, err = notes.List("trip-a")
	if err != nil {
		t.Fatalf("list cleared trip-a notes: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("cleared trip-a notes = %q, want no notes", list)
	}
	tripB, err := notes.List("trip-b")
	if err != nil {
		t.Fatalf("list trip-b notes after clearing trip-a: %v", err)
	}
	if len(tripB) != 1 || tripB[0] != "trip-b only" {
		t.Fatalf("trip-b notes after clearing trip-a = %q, want exactly [trip-b only]", tripB)
	}

	if err := notes.Clear(`trip-a" OR "1"="1`); err != nil {
		t.Fatalf("clear quote-bearing trip selector: %v", err)
	}
	tripB, err = notes.List("trip-b")
	if err != nil {
		t.Fatalf("list trip-b notes after quote-bearing clear: %v", err)
	}
	if len(tripB) != 1 || tripB[0] != "trip-b only" {
		t.Fatalf("trip-b notes after quote-bearing clear = %q, want exactly [trip-b only]", tripB)
	}
}
