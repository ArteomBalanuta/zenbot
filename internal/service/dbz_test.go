package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"zenbot/internal/repository"
)

type dbzRepoStub struct {
	mu         sync.Mutex
	stats      repository.DBZStats
	levelUps   int
	levelUpErr error
}

func (r *dbzRepoStub) RegisterCharacter(context.Context, string, func() int64) (int64, error) {
	return 1, nil
}
func (r *dbzRepoStub) LevelUp(context.Context, string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levelUps++
	return r.levelUpErr
}
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
		go func() {
			defer wg.Done()
			defeated, err := s.Fight(context.Background(), "goku", "x")
			if err != nil || !defeated {
				t.Errorf("Fight()=%v, %v", defeated, err)
			}
		}()
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
	if err == nil || free != 0 {
		t.Fatalf("FreeStats()=%d, %v; want 0 and repository error", free, err)
	}
}

type missingDBZRepo struct{ dbzRepoStub }

func (*missingDBZRepo) Stats(context.Context, string) (repository.DBZStats, bool, error) {
	return repository.DBZStats{}, false, nil
}
func (*missingDBZRepo) FreeStats(context.Context, string) (int, bool, error) {
	return 0, false, nil
}

func TestDBZMissingStatsReturnCharacterNotFound(t *testing.T) {
	s := &DBZService{Repo: &missingDBZRepo{}}
	if text, err := s.StatsText(context.Background(), "missing"); text != "" || !errors.Is(err, repository.ErrDBZCharacterNotFound) {
		t.Fatalf("StatsText()=%q, %v", text, err)
	}
	if free, err := s.FreeStats(context.Background(), "missing"); free != 0 || !errors.Is(err, repository.ErrDBZCharacterNotFound) {
		t.Fatalf("FreeStats()=%d, %v", free, err)
	}
}

func TestDBZFightAbsentOrRepeatedEnemyDoesNotReward(t *testing.T) {
	repo := &dbzRepoStub{}
	s := &DBZService{Repo: repo}
	s.SpawnEnemy("frieza")

	if defeated, err := s.Fight(context.Background(), "goku", "missing"); err != nil || defeated {
		t.Fatalf("missing Fight()=%v, %v", defeated, err)
	}
	if defeated, err := s.Fight(context.Background(), "goku", "frieza"); err != nil || !defeated {
		t.Fatalf("first Fight()=%v, %v", defeated, err)
	}
	if defeated, err := s.Fight(context.Background(), "goku", "frieza"); err != nil || defeated {
		t.Fatalf("repeated Fight()=%v, %v", defeated, err)
	}
	if repo.levelUps != 1 {
		t.Fatalf("level ups=%d, want 1", repo.levelUps)
	}
}

func TestDBZConcurrentFightRewardsOneAttempt(t *testing.T) {
	repo := &dbzRepoStub{}
	s := &DBZService{Repo: repo}
	s.SpawnEnemy("frieza")
	start := make(chan struct{})
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			defeated, err := s.Fight(context.Background(), "goku", "frieza")
			if err != nil {
				t.Errorf("Fight error=%v", err)
			}
			results <- defeated
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var defeated int
	for result := range results {
		if result {
			defeated++
		}
	}
	if defeated != 1 || repo.levelUps != 1 || len(s.Enemies()) != 0 {
		t.Fatalf("defeated=%d levelUps=%d enemies=%q", defeated, repo.levelUps, s.Enemies())
	}
}

func TestDBZFightDefinitePersistenceFailureRetainsEnemy(t *testing.T) {
	want := errors.New("level write rejected")
	repo := &dbzRepoStub{levelUpErr: want}
	s := &DBZService{Repo: repo}
	s.SpawnEnemy("frieza")

	defeated, err := s.Fight(context.Background(), "goku", "frieza")
	if defeated || !errors.Is(err, want) || !reflect.DeepEqual(s.Enemies(), []string{"frieza"}) {
		t.Fatalf("Fight()=%v, %v enemies=%q", defeated, err, s.Enemies())
	}
}

func TestDBZFightUnknownCommitQuarantinesEnemyWithoutConfirmedReward(t *testing.T) {
	driverErr := errors.New("commit response lost")
	repo := &dbzRepoStub{levelUpErr: fmt.Errorf("%w: %w", repository.ErrCommitOutcomeUnknown, driverErr)}
	s := &DBZService{Repo: repo}
	s.SpawnEnemy("frieza")

	defeated, err := s.Fight(context.Background(), "goku", "frieza")
	if defeated || !errors.Is(err, repository.ErrCommitOutcomeUnknown) || !errors.Is(err, driverErr) || len(s.Enemies()) != 0 {
		t.Fatalf("Fight()=%v, %v enemies=%q", defeated, err, s.Enemies())
	}
	defeated, err = s.Fight(context.Background(), "goku", "frieza")
	if defeated || err != nil || repo.levelUps != 1 {
		t.Fatalf("replay Fight()=%v, %v levelUps=%d", defeated, err, repo.levelUps)
	}
}

func TestDBZFightCancelledContextRetainsEnemyWithoutReward(t *testing.T) {
	repo := &dbzRepoStub{}
	s := &DBZService{Repo: repo}
	s.SpawnEnemy("frieza")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	defeated, err := s.Fight(ctx, "goku", "frieza")
	if defeated || !errors.Is(err, context.Canceled) || repo.levelUps != 0 || !reflect.DeepEqual(s.Enemies(), []string{"frieza"}) {
		t.Fatalf("Fight()=%v, %v levelUps=%d enemies=%q", defeated, err, repo.levelUps, s.Enemies())
	}
}
