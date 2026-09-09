package repository

import "context"

// ActivityStat is one source-query row used by the moderator activity command.
type ActivityStat struct {
	Trip                  string
	DayOfWeek             string
	Hour                  string
	ProbabilityPercentage string
}

// ActivityRepository is the bounded persistence seam for Saturn's activity
// statistics query. Implementations must retain the source query's exact
// case-insensitive trip filter and trip/day/hour grouping semantics.
type ActivityRepository interface {
	ActivityStats(context.Context, string) ([]ActivityStat, error)
}
