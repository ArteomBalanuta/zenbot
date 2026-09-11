package repository

import (
	"context"
	"errors"
)

var (
	// ErrCommitOutcomeUnknown means a transaction's commit result could not be
	// established. Callers must not blindly replay its mutation.
	ErrCommitOutcomeUnknown     = errors.New("commit outcome unknown")
	ErrDBZCharacterNotFound     = errors.New("DBZ character not found")
	ErrDBZInvalidStatAmount     = errors.New("DBZ stat amount must be a positive 32-bit integer")
	ErrDBZInsufficientFreeStats = errors.New("insufficient DBZ free stats")
	ErrDBZStatOverflow          = errors.New("DBZ stat allocation overflows 32-bit integer")
)

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
