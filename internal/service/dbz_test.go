package service

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"zenbot/internal/repository"
)

type dbzRepoStub struct {
	mu      sync.Mutex
	stats   repository.DBZStats
	enemies int
}

func (r *dbzRepoStub) RegisterCharacter(context.Context, string, func() int64) (int64, error) {
	return 1, nil
}
func (r *dbzRepoStub) LevelUp(context.Context, string) error          { return nil }
func (r *dbzRepoStub) AddStrength(context.Context, string, int) error { return nil }
func (r *dbzRepoStub) AddAgility(context.Context, string, int) error  { return nil }
func (r *dbzRepoStub) AddVitality(context.Context, string, int) error { return nil }
func (r *dbzRepoStub) AddEnergy(context.Context, string, int) error   { return nil }
func (r *dbzRepoStub) Stats(context.Context, string) (repository.DBZStats, bool, error) {
	return r.stats, true, nil
}
func (r *dbzRepoStub) FreeStats(context.Context, string) (int, bool, error) {
	return r.stats.FreeStats, true, nil
}
func TestDBZStatsTextExactAndEnemyStateConcurrent(t *testing.T) {
	r := &dbzRepoStub{stats: repository.DBZStats{Name: "goku", Level: 2, FreeStats: 5, Strength: 3, Agility: 1, Vitality: 1, Energy: 1}}
	s := &DBZService{Repo: r}
	got, err := s.StatsText(context.Background(), "goku")
	if err != nil {
		t.Fatal(err)
	}
	want := "character: goku\nlevel: 2\nfree stats: 5\nstr: 3\nagi: 1\nvit: 1\nene: 1\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := 0; i < 100; i++ {
		s.SpawnEnemy("x")
	}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.Fight("x") }()
	}
	wg.Wait()
	if len(s.Enemies()) != 0 {
		t.Fatalf("enemies=%v", s.Enemies())
	}
}

func TestDBZSpawnEnemyStateIsInstanceLocalAndRetainsInsertionOrder(t *testing.T) {
	first := &DBZService{}
	first.SpawnEnemy("a")
	first.SpawnEnemy("a")
	first.SpawnEnemy("b")
	if got, want := first.Enemies(), []string{"a", "a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first enemies=%q want=%q", got, want)
	}
	second := &DBZService{}
	if enemies := second.Enemies(); len(enemies) != 0 {
		t.Fatalf("second enemies=%q", enemies)
	}
}

func TestDBZStatsTextRendersExactSevenLineSnapshot(t *testing.T) {
	s := &DBZService{Repo: &dbzRepoStub{stats: repository.DBZStats{Name: "goku", Level: 2, FreeStats: 5, Strength: 3, Agility: 4, Vitality: 5, Energy: 6}}}
	got, err := s.StatsText(context.Background(), "goku")
	if err != nil {
		t.Fatal(err)
	}
	want := "character: goku\nlevel: 2\nfree stats: 5\nstr: 3\nagi: 4\nvit: 5\nene: 6\n"
	if got != want {
		t.Fatalf("StatsText()=%q, want %q", got, want)
	}
}

type failingFreeStatsDBZRepo struct{ dbzRepoStub }

func (*failingFreeStatsDBZRepo) FreeStats(context.Context, string) (int, bool, error) {
	return 0, false, errors.New("free stats query failed")
}

func TestDBZFreeStatsConvertsRepositoryReadErrorToSourceEquivalentNoFreeValue(t *testing.T) {
	free, err := (&DBZService{Repo: &failingFreeStatsDBZRepo{}}).FreeStats(context.Background(), "goku")
	if err != nil || free != -1 {
		t.Fatalf("FreeStats()=%d, %v; want -1, nil", free, err)
	}
}
