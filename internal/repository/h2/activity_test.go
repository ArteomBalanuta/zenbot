package h2

import (
	"context"
	"testing"
)

func TestActivityStatsUsesSourceCaseInsensitiveTripQueryAndOrdersWeekHour(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for _, statement := range []string{
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('Trip-A','one','first',3600000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','one','second',3600000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('TRIP-A','one','third',7200000,'PUBLIC')",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('other','two','other',3600000,'PUBLIC')",
	} {
		if _, err := d.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := d.ActivityStats(ctx, " trip-a ")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 {
		t.Fatalf("stats=%+v", stats)
	}
	// Source grouping is by exact stored trip while its final target filter is
	// case-insensitive, so three case variants remain independently normalized.
	want := []struct{ trip, day, hour, probability string }{
		{"TRIP-A", "Wednesday", "2", "100.000000000000000000000000000000000000000"},
		{"Trip-A", "Wednesday", "1", "100.000000000000000000000000000000000000000"},
		{"trip-a", "Wednesday", "1", "100.000000000000000000000000000000000000000"},
	}
	for i, row := range want {
		if stats[i].Trip != row.trip || stats[i].DayOfWeek != row.day || stats[i].Hour != row.hour || stats[i].ProbabilityPercentage != row.probability {
			t.Fatalf("stats[%d]=%+v, want=%+v", i, stats[i], row)
		}
	}
}

func TestActivityStatsReturnsNoRowsForAbsentTrip(t *testing.T) {
	d := openTestDB(t)
	stats, err := d.ActivityStats(context.Background(), "absent")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("stats=%+v", stats)
	}
}
