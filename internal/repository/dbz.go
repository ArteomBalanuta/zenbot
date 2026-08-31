package repository

import "context"

// DBZStats is the persisted character snapshot used by the DBZ service.
type DBZStats struct {
	Name                                           string
	Level                                          int
	FreeStats, Strength, Agility, Vitality, Energy int
}

// DBZRepository deliberately mirrors Saturn's small, quirky DBZ persistence surface.
type DBZRepository interface {
	RegisterCharacter(context.Context, string, func() int64) (int64, error)
	LevelUp(context.Context, string) error
	AddStrength(context.Context, string, int) error
	AddAgility(context.Context, string, int) error
	AddVitality(context.Context, string, int) error
	AddEnergy(context.Context, string, int) error
	Stats(context.Context, string) (DBZStats, bool, error)
	FreeStats(context.Context, string) (int, bool, error)
}
