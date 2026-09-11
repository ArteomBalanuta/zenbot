package h2

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"zenbot/internal/repository"
)

func TestDBZRealH2RegisterLevelAndAllocationsSpendExactBalance(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	id, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 })
	if err != nil || id == 0 {
		t.Fatalf("register id=%d err=%v", id, err)
	}
	var level, free, str, agi, vit, ene int
	if err := d.DB.QueryRow("SELECT c.level,s.free_stats,s.str,s.agi FROM dbz_characters c JOIN dbz_stats s ON s.char_id=c.id WHERE c.name=$1", "goku").Scan(&level, &free, &str, &agi); err != nil {
		t.Fatal(err)
	}
	if level != 1 || free != 0 || str != 1 || agi != 1 {
		t.Fatalf("initial %d %d %d %d", level, free, str, agi)
	}
	if err = d.LevelUp(ctx, "goku"); err != nil {
		t.Fatal(err)
	}
	if err = d.AddStrength(ctx, "goku", 2); err != nil {
		t.Fatal(err)
	}
	if err = d.AddAgility(ctx, "goku", 1); err != nil {
		t.Fatal(err)
	}
	if err = d.AddVitality(ctx, "goku", 1); err != nil {
		t.Fatal(err)
	}
	if err = d.AddEnergy(ctx, "goku", 1); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT free_stats,str,agi,vit,ene FROM dbz_stats").Scan(&free, &str, &agi, &vit, &ene); err != nil {
		t.Fatal(err)
	}
	if free != 0 || str != 3 || agi != 2 || vit != 2 || ene != 2 {
		t.Fatalf("allocated free=%d str=%d agi=%d vit=%d ene=%d", free, str, agi, vit, ene)
	}
}

func TestDBZRegistrationDuplicateAndMissingReadSemantics(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "vegeta", func() int64 { return 11 }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RegisterCharacter(ctx, "vegeta", func() int64 { return 12 }); err != nil {
		t.Fatal(err)
	}
	var characters, stats, distinctStatsCharacters int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=$1", "vegeta").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM dbz_stats").Scan(&stats); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(DISTINCT char_id) FROM dbz_stats").Scan(&distinctStatsCharacters); err != nil {
		t.Fatal(err)
	}
	if characters != 2 || stats != 2 || distinctStatsCharacters != 2 {
		t.Fatalf("duplicate registration characters=%d stats=%d distinct stat owners=%d", characters, stats, distinctStatsCharacters)
	}
	got, ok, err := d.Stats(ctx, "missing")
	if err != nil || ok || got != (repository.DBZStats{}) {
		t.Fatalf("missing stats=%+v ok=%v err=%v", got, ok, err)
	}
	free, ok, err := d.FreeStats(ctx, "missing")
	if err != nil || ok || free != -1 {
		t.Fatalf("missing free stats=%d ok=%v err=%v", free, ok, err)
	}
}

func TestDBZStatsRealH2ReadsJoinedSnapshotByName(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_characters(name,level,created_on) VALUES($1,$2,$3)", "goku", 2, 1); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := d.DB.QueryRowContext(ctx, "SELECT id FROM dbz_characters WHERE name=$1", "goku").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_stats(char_id,free_stats,str,agi,vit,ene,created_on) VALUES($1,$2,$3,$4,$5,$6,$7)", id, 5, 3, 4, 5, 6, 1); err != nil {
		t.Fatal(err)
	}

	got, ok, err := d.Stats(ctx, "goku")
	want := repository.DBZStats{Name: "goku", Level: 2, FreeStats: 5, Strength: 3, Agility: 4, Vitality: 5, Energy: 6}
	if err != nil || !ok || got != want {
		t.Fatalf("stats=%+v ok=%v err=%v, want %+v true nil", got, ok, err, want)
	}
}

func TestDBZStrengthRealH2RejectsOverspendWithoutMutation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_characters(name,level,created_on) VALUES($1,$2,$3)", "goku", 2, 1); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := d.DB.QueryRowContext(ctx, "SELECT id FROM dbz_characters WHERE name=$1", "goku").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_stats(char_id,free_stats,str,agi,vit,ene,created_on) VALUES($1,$2,$3,$4,$5,$6,$7)", id, 1, 3, 1, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := d.AddStrength(ctx, "goku", 2); !errors.Is(err, repository.ErrDBZInsufficientFreeStats) {
		t.Fatalf("AddStrength error=%v", err)
	}
	var strength, free int
	if err := d.DB.QueryRowContext(ctx, "SELECT str,free_stats FROM dbz_stats WHERE char_id=$1", id).Scan(&strength, &free); err != nil {
		t.Fatal(err)
	}
	if strength != 3 || free != 1 {
		t.Fatalf("strength=%d free_stats=%d, want unchanged 3 1", strength, free)
	}
}

func TestDBZRegistrationRollsBackCharacterWhenStatsInsertFails(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.DB.ExecContext(ctx, `ALTER TABLE dbz_stats ADD CONSTRAINT reject_initial_dbz_stats CHECK (str > 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 }); err == nil {
		t.Fatal("registration succeeded despite rejected stats row")
	}
	var characters, stats int
	if err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM dbz_characters WHERE name=$1", "goku").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM dbz_stats").Scan(&stats); err != nil {
		t.Fatal(err)
	}
	if characters != 0 || stats != 0 {
		t.Fatalf("partial registration characters=%d stats=%d", characters, stats)
	}
}

func TestDBZLevelUpRollsBackLevelWhenRewardFails(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, `ALTER TABLE dbz_stats ADD CONSTRAINT reject_level_reward CHECK (free_stats <> 5)`); err != nil {
		t.Fatal(err)
	}
	if err := d.LevelUp(ctx, "goku"); err == nil {
		t.Fatal("level up succeeded despite rejected reward")
	}
	stats, found, err := d.Stats(ctx, "goku")
	if err != nil || !found || stats.Level != 1 || stats.FreeStats != 0 {
		t.Fatalf("partial level up stats=%+v found=%v err=%v", stats, found, err)
	}
}

func TestDBZMutationsRejectMissingCharacter(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	for name, mutate := range map[string]func() error{
		"level":    func() error { return d.LevelUp(ctx, "missing") },
		"strength": func() error { return d.AddStrength(ctx, "missing", 1) },
		"agility":  func() error { return d.AddAgility(ctx, "missing", 1) },
		"vitality": func() error { return d.AddVitality(ctx, "missing", 1) },
		"energy":   func() error { return d.AddEnergy(ctx, "missing", 1) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutate(); !errors.Is(err, repository.ErrDBZCharacterNotFound) {
				t.Fatalf("mutation error=%v", err)
			}
		})
	}
}

func TestDBZAllocationsRejectNonPositiveAndNonInt32Amounts(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 }); err != nil {
		t.Fatal(err)
	}
	if err := d.LevelUp(ctx, "goku"); err != nil {
		t.Fatal(err)
	}
	tooLarge := int(int64(math.MaxInt32) + 1)
	for _, amount := range []int{0, -1, tooLarge} {
		for name, mutate := range map[string]func() error{
			"strength": func() error { return d.AddStrength(ctx, "goku", amount) },
			"agility":  func() error { return d.AddAgility(ctx, "goku", amount) },
			"vitality": func() error { return d.AddVitality(ctx, "goku", amount) },
			"energy":   func() error { return d.AddEnergy(ctx, "goku", amount) },
		} {
			t.Run(name, func(t *testing.T) {
				if err := mutate(); !errors.Is(err, repository.ErrDBZInvalidStatAmount) {
					t.Fatalf("amount=%d error=%v", amount, err)
				}
			})
		}
	}
	stats, found, err := d.Stats(ctx, "goku")
	want := repository.DBZStats{Name: "goku", Level: 2, FreeStats: 5, Strength: 1, Agility: 1, Vitality: 1, Energy: 1}
	if err != nil || !found || stats != want {
		t.Fatalf("invalid allocation changed stats=%+v found=%v err=%v", stats, found, err)
	}
}

func TestDBZAllocationRejectsIntegerOverflowWithoutMutation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DB.ExecContext(ctx, "UPDATE dbz_stats SET free_stats=1,str=$1 WHERE char_id=(SELECT id FROM dbz_characters WHERE name=$2)", math.MaxInt32, "goku"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddStrength(ctx, "goku", 1); !errors.Is(err, repository.ErrDBZStatOverflow) {
		t.Fatalf("overflow error=%v", err)
	}
	stats, found, err := d.Stats(ctx, "goku")
	if err != nil || !found || stats.FreeStats != 1 || stats.Strength != math.MaxInt32 {
		t.Fatalf("overflow changed stats=%+v found=%v err=%v", stats, found, err)
	}
}

func TestDBZConcurrentAllocationsCannotOverspend(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 7 }); err != nil {
		t.Fatal(err)
	}
	if err := d.LevelUp(ctx, "goku"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- d.AddStrength(ctx, "goku", 5)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var succeeded, insufficient int
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, repository.ErrDBZInsufficientFreeStats):
			insufficient++
		default:
			t.Fatalf("unexpected allocation error=%v", err)
		}
	}
	stats, found, err := d.Stats(ctx, "goku")
	if succeeded != 1 || insufficient != 1 || err != nil || !found || stats.FreeStats != 0 || stats.Strength != 6 {
		t.Fatalf("success=%d insufficient=%d stats=%+v found=%v err=%v", succeeded, insufficient, stats, found, err)
	}
}

func TestDBZCancelledMutationsDoNotWrite(t *testing.T) {
	d := openTestDB(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.RegisterCharacter(cancelled, "cancelled", func() int64 { return 7 }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled registration error=%v", err)
	}
	var cancelledCharacters int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=$1", "cancelled").Scan(&cancelledCharacters); err != nil {
		t.Fatal(err)
	}
	if cancelledCharacters != 0 {
		t.Fatalf("cancelled registration wrote %d characters", cancelledCharacters)
	}

	ctx := context.Background()
	if _, err := d.RegisterCharacter(ctx, "goku", func() int64 { return 8 }); err != nil {
		t.Fatal(err)
	}
	if err := d.LevelUp(ctx, "goku"); err != nil {
		t.Fatal(err)
	}
	if err := d.LevelUp(cancelled, "goku"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled level error=%v", err)
	}
	if err := d.AddStrength(cancelled, "goku", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled allocation error=%v", err)
	}
	stats, found, err := d.Stats(ctx, "goku")
	want := repository.DBZStats{Name: "goku", Level: 2, FreeStats: 5, Strength: 1, Agility: 1, Vitality: 1, Energy: 1}
	if err != nil || !found || stats != want {
		t.Fatalf("cancelled mutation changed stats=%+v found=%v err=%v", stats, found, err)
	}
}

func TestDBZStorageErrorsRemainDistinctFromBusinessRejections(t *testing.T) {
	d := openTestDB(t)
	if err := d.DB.Close(); err != nil {
		t.Fatal(err)
	}
	err := d.AddStrength(context.Background(), "goku", 1)
	if err == nil || errors.Is(err, repository.ErrDBZCharacterNotFound) || errors.Is(err, repository.ErrDBZInsufficientFreeStats) || errors.Is(err, repository.ErrDBZInvalidStatAmount) || errors.Is(err, repository.ErrDBZStatOverflow) {
		t.Fatalf("closed storage error=%v", err)
	}
}
