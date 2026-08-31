package util

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// TimestampUnit is deliberately closed to avoid confusing epoch seconds and milliseconds.
type TimestampUnit uint8

const (
	UnitSeconds TimestampUnit = iota
	UnitMilliseconds
)

func TimestampNowMillis() int64 { return time.Now().UnixMilli() }

func UTCNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

var timestampPattern = regexp.MustCompile(`^(\d+)-(\d+)-(\d+)T(\d+):(\d+)`)

// Timestamp8601ToSeconds preserves Java SimpleDateFormat's null-on-parse-failure
// result and its accepted one-digit minute form. time.Date is intentionally used
// after validation so its overflow normalization retains SimpleDateFormat leniency.
func Timestamp8601ToSeconds(timestamp *string, zoneID *string) (*int64, error) {
	if timestamp == nil {
		return nil, nil
	}
	zone := "UTC"
	if zoneID != nil {
		zone = *zoneID
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, err
	}
	parts := timestampPattern.FindStringSubmatch(*timestamp)
	if parts == nil {
		return nil, nil
	}
	values := make([]int, 5)
	for i := range values {
		values[i], err = strconv.Atoi(parts[i+1])
		if err != nil {
			return nil, nil
		}
	}
	value := time.Date(values[0], time.Month(values[1]), values[2], values[3], values[4], 0, 0, location).Unix()
	return &value, nil
}

func Difference(first, second time.Time) string {
	delta := first.Sub(second)
	seconds := int64(delta / time.Second)
	if delta < 0 && delta%time.Second != 0 {
		// Java Duration.getSeconds() floors negative fractional seconds, while
		// its toDays/toHours/toMinutes component methods truncate toward zero.
		seconds--
	}
	days := seconds / (24 * 60 * 60)
	seconds -= days * 24 * 60 * 60
	hours := seconds / (60 * 60)
	seconds -= hours * 60 * 60
	minutes := seconds / 60
	seconds -= minutes * 60
	return fmt.Sprintf("%d days, %d hours, %d minutes, %d seconds", days, hours, minutes, seconds)
}

func FormatZoneUTC(timestampMillis int64) string {
	return FormatTime(ToZoneDateTimeUTC(timestampMillis))
}

func ToZoneDateTimeUTC(timestampMillis int64) time.Time {
	return time.UnixMilli(timestampMillis).UTC()
}

func FormatTime(value time.Time) string { return value.Format("2006-01-02T15:04:05 MST") }

func FormatRFC1123(epochTimestamp int64, unit TimestampUnit, zoneID string) (string, error) {
	var value time.Time
	switch unit {
	case UnitSeconds:
		value = time.Unix(epochTimestamp, 0)
	case UnitMilliseconds:
		value = time.UnixMilli(epochTimestamp)
	default:
		return "", errors.New("Timestamp should be in seconds or milliseconds.")
	}
	location, err := time.LoadLocation(zoneID)
	if err != nil {
		return "", err
	}
	value = value.In(location)
	zone := value.Format("MST")
	if zone == "UTC" {
		zone = "GMT"
	}
	return fmt.Sprintf("%s, %d %s %s %s %s", value.Format("Mon"), value.Day(), value.Format("Jan"), value.Format("2006"), value.Format("15:04:05"), zone), nil
}
