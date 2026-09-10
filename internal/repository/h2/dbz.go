package h2

import (
	"context"
	"database/sql"
	"strconv"
	"zenbot/internal/repository"
)

// Register intentionally remains two-step and non-atomic for Saturn parity.
func (d *Database) RegisterCharacter(ctx context.Context, name string, now func() int64) (int64, error) {
	if now == nil {
		now = func() int64 { return 0 }
	}
	if _, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_characters(name,level,created_on) VALUES($1,1,$2)", name, now()); err != nil {
		return 0, err
	}
	var id int64
	if err := d.DB.QueryRowContext(ctx, "SELECT id FROM dbz_characters WHERE name=$1", name).Scan(&id); err != nil {
		return 0, err
	}
	_, err := d.DB.ExecContext(ctx, "INSERT INTO dbz_stats(char_id,free_stats,str,agi,vit,ene,created_on) VALUES($1,0,1,1,1,1,$2)", id, now())
	return id, err
}
func (d *Database) LevelUp(ctx context.Context, name string) error {
	if _, err := d.DB.ExecContext(ctx, "UPDATE dbz_characters SET level=level+1 WHERE name=$1", name); err != nil {
		return err
	}
	_, err := d.DB.ExecContext(ctx, "UPDATE dbz_stats SET free_stats=free_stats+5 WHERE char_id=(SELECT id FROM dbz_characters WHERE name=$1)", name)
	return err
}
func (d *Database) AddStrength(ctx context.Context, name string, amount int) error {
	a := strconv.Itoa(amount)
	_, err := d.DB.ExecContext(ctx, "UPDATE dbz_stats SET str=str+CAST($1 AS INTEGER), free_stats=free_stats-CAST($2 AS INTEGER) WHERE char_id=(SELECT id FROM dbz_characters WHERE name=$3)", a, a, name)
	return err
}
func (d *Database) AddAgility(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, "agi", name, amount)
}
func (d *Database) AddVitality(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, "vit", name, amount)
}
func (d *Database) AddEnergy(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, "ene", name, amount)
}
func (d *Database) addStat(ctx context.Context, col, name string, amount int) error {
	_, err := d.DB.ExecContext(ctx, "UPDATE dbz_stats SET "+col+"="+col+"+CAST($1 AS INTEGER) WHERE char_id=(SELECT id FROM dbz_characters WHERE name=$2)", strconv.Itoa(amount), name)
	return err
}
func (d *Database) Stats(ctx context.Context, name string) (repository.DBZStats, bool, error) {
	var s repository.DBZStats
	err := d.DB.QueryRowContext(ctx, "SELECT c.name,c.level,s.free_stats,s.str,s.agi,s.vit,s.ene FROM dbz_stats s INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=$1", name).Scan(&s.Name, &s.Level, &s.FreeStats, &s.Strength, &s.Agility, &s.Vitality, &s.Energy)
	if err == sql.ErrNoRows {
		return s, false, nil
	}
	return s, err == nil, err
}
func (d *Database) FreeStats(ctx context.Context, name string) (int, bool, error) {
	var n int
	err := d.DB.QueryRowContext(ctx, "SELECT s.free_stats FROM dbz_stats s INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=$1", name).Scan(&n)
	if err == sql.ErrNoRows {
		return -1, false, nil
	}
	return n, err == nil, err
}
