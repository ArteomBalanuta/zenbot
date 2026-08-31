package h2

import (
	"context"
	"testing"
	"zenbot/internal/repository"
)

func TestDBZRealH2RegisterAndQuirkyMutations(t *testing.T) {
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
	if err = d.AddAgility(ctx, "goku", 3); err != nil {
		t.Fatal(err)
	}
	if err = d.AddVitality(ctx, "goku", 4); err != nil {
		t.Fatal(err)
	}
	if err = d.AddEnergy(ctx, "goku", 5); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT free_stats,str,agi,vit,ene FROM dbz_stats").Scan(&free, &str, &agi, &vit, &ene); err != nil {
		t.Fatal(err)
	}
	if free != 3 || str != 3 || agi != 4 || vit != 5 || ene != 6 {
		t.Fatalf("quirks free=%d str=%d agi=%d vit=%d ene=%d", free, str, agi, vit, ene)
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
	var characters, stats int
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM dbz_characters WHERE name=$1", "vegeta").Scan(&characters); err != nil {
		t.Fatal(err)
	}
	if err := d.DB.QueryRow("SELECT COUNT(*) FROM dbz_stats").Scan(&stats); err != nil {
		t.Fatal(err)
	}
	if characters != 2 || stats != 2 {
		t.Fatalf("duplicate registration counts characters=%d stats=%d", characters, stats)
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
