package h2

import (
	"context"
	"strings"

	"zenbot/internal/repository"
)

// activityStatsSQL is ActivityCommandImpl.SQL_STATS_PER_HOUR_OF_WEEK ported
// verbatim to PostgreSQL-wire parameter syntax.
const activityStatsSQL = `
WITH MessagesPerTrip AS (
    SELECT
        trip,
        DAY_OF_WEEK(DATEADD(MILLISECOND, created_on, TIMESTAMP '1970-01-01 00:00:00')) AS day_number,
        EXTRACT(HOUR FROM DATEADD(MILLISECOND, created_on, TIMESTAMP '1970-01-01 00:00:00')) AS "hour",
        COUNT(*) AS message_count
    FROM messages
    GROUP BY trip, day_number, "hour"
),
TotalMessages AS (
    SELECT
        trip,
        COUNT(*) AS total_message_count
    FROM messages
    GROUP BY trip
),
Probability AS (
    SELECT
        m.trip,
        m.day_number,
        m."hour",
        (m.message_count * 1.0 / t.total_message_count) * 100 AS probability_percentage,
        CASE m.day_number
            WHEN 1 THEN 'Sunday'
            WHEN 2 THEN 'Monday'
            WHEN 3 THEN 'Tuesday'
            WHEN 4 THEN 'Wednesday'
            WHEN 5 THEN 'Thursday'
            WHEN 6 THEN 'Friday'
            WHEN 7 THEN 'Saturday'
        END AS day_full
    FROM MessagesPerTrip m
    JOIN TotalMessages t ON m.trip = t.trip
)
SELECT
    trip,
    day_full AS day_of_week,
    "hour",
    probability_percentage
FROM Probability WHERE LOWER(trip) = LOWER($1) ORDER BY trip, day_number, "hour"`

func (d *Database) ActivityStats(ctx context.Context, trip string) ([]repository.ActivityStat, error) {
	rows, err := d.DB.QueryContext(ctx, activityStatsSQL, strings.TrimSpace(trip))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []repository.ActivityStat{}
	for rows.Next() {
		var stat repository.ActivityStat
		if err := rows.Scan(&stat.Trip, &stat.DayOfWeek, &stat.Hour, &stat.ProbabilityPercentage); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}
	return stats, rows.Err()
}
