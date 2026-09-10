package repository

import "context"

// AutoMoveTripRepository lists the USER trips eligible for automatic moves.
type AutoMoveTripRepository interface {
	UserTrips(context.Context) ([]string, error)
}
