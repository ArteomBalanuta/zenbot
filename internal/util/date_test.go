package util

import (
	"strings"
	"testing"
	"time"
)

func TestTimestampNowMillisIsCurrentEpochMilliseconds(t *testing.T) {
	before := time.Now().UnixMilli()
	got := TimestampNowMillis()
	after := time.Now().UnixMilli()
	if got < before || got > after {
		t.Fatalf("TimestampNowMillis() = %d, want value in [%d, %d]", got, before, after)
	}
}

func TestUTCNowIsParseableUTCISO8601(t *testing.T) {
	got := UTCNow()
	parsed, err := time.Parse(time.RFC3339Nano, got)
	if err != nil {
		t.Fatalf("UTCNow() = %q is not RFC3339: %v", got, err)
	}
	if parsed.Location() != time.UTC {
		t.Fatalf("UTCNow() location = %v, want UTC", parsed.Location())
	}
}

func TestTimestamp8601ToSeconds(t *testing.T) {
	valid := "2023-08-27T14:5"
	cases := []struct {
		name  string
		value *string
		zone  *string
		want  *int64
	}{
		{name: "lenient one digit minute", value: &valid, want: ptr(int64(1693145100))},
		{name: "nil timestamp", value: nil, want: nil},
	}
	invalid := "not a timestamp"
	cases = append(cases, struct {
		name  string
		value *string
		zone  *string
		want  *int64
	}{name: "invalid timestamp", value: &invalid, want: nil})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Timestamp8601ToSeconds(tc.value, tc.zone)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !sameInt64Ptr(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		value string
		want  int64
	}{
		{name: "lenient one digit date and hour fields", value: "2023-8-7T4:5", want: 1691381100},
		{name: "lenient overflow", value: "2023-02-30T14:05", want: 1677765900},
		{name: "SimpleDateFormat accepts trailing text", value: "2023-08-27T14:05suffix", want: 1693145100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Timestamp8601ToSeconds(&tc.value, nil)
			if err != nil || got == nil || *got != tc.want {
				t.Fatalf("Timestamp8601ToSeconds(%q) = %v, %v; want %d", tc.value, got, err, tc.want)
			}
		})
	}

	zoneInput := "2023-08-27T14:05"
	utc, err := Timestamp8601ToSeconds(&zoneInput, nil)
	if err != nil {
		t.Fatal(err)
	}
	newYork, err := Timestamp8601ToSeconds(&zoneInput, ptr("America/New_York"))
	if err != nil {
		t.Fatal(err)
	}
	if *newYork-*utc != 4*60*60 {
		t.Fatalf("New York - UTC = %d seconds, want 14400", *newYork-*utc)
	}
	if _, err := Timestamp8601ToSeconds(&zoneInput, ptr("Not/AZone")); err == nil {
		t.Fatal("invalid zone unexpectedly accepted")
	}
}

func TestDifferencePreservesSignedJavaComponents(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	cases := []struct {
		name  string
		first time.Time
		want  string
	}{
		{name: "zero", first: base, want: "0 days, 0 hours, 0 minutes, 0 seconds"},
		{name: "positive", first: base.Add(90061 * time.Second), want: "1 days, 1 hours, 1 minutes, 1 seconds"},
		{name: "negative one second", first: base.Add(-time.Second), want: "0 days, 0 hours, 0 minutes, -1 seconds"},
		{name: "negative multi component", first: base.Add(-90061 * time.Second), want: "-1 days, -1 hours, -1 minutes, -1 seconds"},
		{name: "negative fractional second", first: base.Add(-1500 * time.Millisecond), want: "0 days, 0 hours, 0 minutes, -2 seconds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Difference(tc.first, base); got != tc.want {
				t.Fatalf("Difference() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEpochMillisecondsAndTimeFormattingUseUTC(t *testing.T) {
	if got := FormatZoneUTC(0); got != "1970-01-01T00:00:00 UTC" {
		t.Fatalf("FormatZoneUTC(0) = %q", got)
	}
	if got := ToZoneDateTimeUTC(0); !got.Equal(time.Unix(0, 0).UTC()) || got.Location() != time.UTC {
		t.Fatalf("ToZoneDateTimeUTC(0) = %v, want UTC epoch", got)
	}
	if got := FormatTime(time.Unix(0, 0).UTC()); got != "1970-01-01T00:00:00 UTC" {
		t.Fatalf("FormatTime(epoch) = %q", got)
	}
}

func TestFormatRFC1123SupportsSecondsMillisecondsAndRejectsOtherUnits(t *testing.T) {
	for _, unit := range []TimestampUnit{UnitSeconds, UnitMilliseconds} {
		if got, err := FormatRFC1123(0, unit, "UTC"); err != nil || got != "Thu, 1 Jan 1970 00:00:00 GMT" {
			t.Fatalf("FormatRFC1123(0, %v) = %q, %v", unit, got, err)
		}
	}
	if got, err := FormatRFC1123(0, UnitSeconds, "America/New_York"); err != nil || !strings.Contains(got, "Wed, 31 Dec 1969 19:00:00 EST") {
		t.Fatalf("New York RFC1123 = %q, %v", got, err)
	}
	if _, err := FormatRFC1123(0, TimestampUnit(99), "UTC"); err == nil || !strings.Contains(err.Error(), "Timestamp should be in seconds or milliseconds.") {
		t.Fatalf("unsupported unit error = %v", err)
	}
	if _, err := FormatRFC1123(0, TimestampUnit(99), "Not/AZone"); err == nil || !strings.Contains(err.Error(), "Timestamp should be in seconds or milliseconds.") {
		t.Fatalf("unsupported unit with invalid zone error = %v", err)
	}
	if _, err := FormatRFC1123(0, UnitSeconds, "Not/AZone"); err == nil {
		t.Fatal("invalid zone unexpectedly accepted")
	}
}

func ptr[T any](v T) *T { return &v }

func sameInt64Ptr(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
