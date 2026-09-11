package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"zenbot/internal/repository"
)

type DBZService struct {
	Repo    repository.DBZRepository
	Now     func() int64
	mu      sync.Mutex
	enemies []string
}

func (s *DBZService) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UnixMilli()
}
func (s *DBZService) Register(ctx context.Context, name string) error {
	_, err := s.Repo.RegisterCharacter(ctx, name, s.now)
	return err
}
func (s *DBZService) AddStrength(ctx context.Context, n string, a int) error {
	return s.Repo.AddStrength(ctx, n, a)
}
func (s *DBZService) AddAgility(ctx context.Context, n string, a int) error {
	return s.Repo.AddAgility(ctx, n, a)
}
func (s *DBZService) AddVitality(ctx context.Context, n string, a int) error {
	return s.Repo.AddVitality(ctx, n, a)
}
func (s *DBZService) AddEnergy(ctx context.Context, n string, a int) error {
	return s.Repo.AddEnergy(ctx, n, a)
}
func (s *DBZService) StatsText(ctx context.Context, name string) (string, error) {
	v, ok, err := s.Repo.Stats(ctx, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", repository.ErrDBZCharacterNotFound
	}
	return fmt.Sprintf("character: %s\nlevel: %d\nfree stats: %d\nstr: %d\nagi: %d\nvit: %d\nene: %d\n", v.Name, v.Level, v.FreeStats, v.Strength, v.Agility, v.Vitality, v.Energy), nil
}
func (s *DBZService) FreeStats(ctx context.Context, name string) (int, error) {
	n, ok, err := s.Repo.FreeStats(ctx, name)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, repository.ErrDBZCharacterNotFound
	}
	return n, nil
}
func (s *DBZService) SpawnEnemy(name string) {
	s.mu.Lock()
	s.enemies = append(s.enemies, name)
	s.mu.Unlock()
}

// Fight rewards and consumes the first matching enemy as one service-owned
// operation. Its boolean reports only a definitely committed reward.
func (s *DBZService) Fight(ctx context.Context, fighter, enemy string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	for i, e := range s.enemies {
		if e != enemy {
			continue
		}
		err := s.Repo.LevelUp(ctx, fighter)
		if err != nil {
			if errors.Is(err, repository.ErrCommitOutcomeUnknown) {
				s.enemies = append(s.enemies[:i], s.enemies[i+1:]...)
			}
			return false, err
		}
		s.enemies = append(s.enemies[:i], s.enemies[i+1:]...)
		return true, nil
	}
	return false, nil
}
func (s *DBZService) Enemies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.enemies...)
}
