package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"

	"zenbot/internal/repository"
)

type dbzStatColumn string

const (
	dbzStrength dbzStatColumn = "str"
	dbzAgility  dbzStatColumn = "agi"
	dbzVitality dbzStatColumn = "vit"
	dbzEnergy   dbzStatColumn = "ene"
)

func (d *Database) RegisterCharacter(ctx context.Context, name string, now func() int64) (int64, error) {
	if now == nil {
		now = func() int64 { return 0 }
	}
	var id int64
	err := d.WithTx(ctx, func(tx *sql.Tx) error {
		createdOn := now()
		if err := tx.QueryRowContext(ctx, "INSERT INTO dbz_characters(name,level,created_on) VALUES(?1,1,?2) RETURNING id", name, createdOn).Scan(&id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO dbz_stats(char_id,free_stats,str,agi,vit,ene,created_on) VALUES(?1,0,1,1,1,1,?2)", id, now())
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (d *Database) LevelUp(ctx context.Context, name string) error {
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		var id int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM dbz_characters WHERE name=?1 ORDER BY id DESC LIMIT 1", name).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return repository.ErrDBZCharacterNotFound
			}
			return err
		}
		result, err := tx.ExecContext(ctx, "UPDATE dbz_characters SET level=level+1 WHERE id=?1", id)
		if err != nil {
			return err
		}
		if err := requireDBZRow(result); err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, "UPDATE dbz_stats SET free_stats=free_stats+5 WHERE char_id=?1", id)
		if err != nil {
			return err
		}
		return requireDBZRow(result)
	})
}

func (d *Database) AddStrength(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, dbzStrength, name, amount)
}

func (d *Database) AddAgility(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, dbzAgility, name, amount)
}

func (d *Database) AddVitality(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, dbzVitality, name, amount)
}

func (d *Database) AddEnergy(ctx context.Context, name string, amount int) error {
	return d.addStat(ctx, dbzEnergy, name, amount)
}

func (d *Database) addStat(ctx context.Context, column dbzStatColumn, name string, amount int) error {
	if amount <= 0 || int64(amount) > math.MaxInt32 {
		return repository.ErrDBZInvalidStatAmount
	}
	columnName, err := dbzColumnName(column)
	if err != nil {
		return err
	}
	query := "UPDATE dbz_stats SET " + columnName + "=" + columnName + "+CAST(?1 AS INTEGER), free_stats=free_stats-CAST(?1 AS INTEGER) " +
		"WHERE id=(SELECT s.id FROM dbz_stats s INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=?2 ORDER BY c.id DESC,s.id DESC LIMIT 1) " +
		"AND free_stats >= CAST(?1 AS INTEGER) AND " + columnName + " <= 2147483647-CAST(?1 AS INTEGER)"
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, query, strconv.Itoa(amount), name)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 1 {
			return nil
		}

		var freeStats, current int64
		read := "SELECT s.free_stats,s." + columnName + " FROM dbz_stats s " +
			"INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=?1 ORDER BY c.id DESC,s.id DESC LIMIT 1"
		if err := tx.QueryRowContext(ctx, read, name).Scan(&freeStats, &current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return repository.ErrDBZCharacterNotFound
			}
			return err
		}
		if freeStats < int64(amount) {
			return repository.ErrDBZInsufficientFreeStats
		}
		if current > math.MaxInt32-int64(amount) {
			return repository.ErrDBZStatOverflow
		}
		return fmt.Errorf("DBZ stat update affected %d rows", rows)
	})
}

func dbzColumnName(column dbzStatColumn) (string, error) {
	switch column {
	case dbzStrength, dbzAgility, dbzVitality, dbzEnergy:
		return string(column), nil
	default:
		return "", fmt.Errorf("invalid DBZ stat column %q", column)
	}
}

func requireDBZRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return repository.ErrDBZCharacterNotFound
	}
	return nil
}

func (d *Database) Stats(ctx context.Context, name string) (repository.DBZStats, bool, error) {
	var s repository.DBZStats
	err := d.DB.QueryRowContext(ctx, "SELECT c.name,c.level,s.free_stats,s.str,s.agi,s.vit,s.ene FROM dbz_stats s INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=?1 ORDER BY c.id DESC,s.id DESC LIMIT 1", name).Scan(&s.Name, &s.Level, &s.FreeStats, &s.Strength, &s.Agility, &s.Vitality, &s.Energy)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}

func (d *Database) FreeStats(ctx context.Context, name string) (int, bool, error) {
	var n int
	err := d.DB.QueryRowContext(ctx, "SELECT s.free_stats FROM dbz_stats s INNER JOIN dbz_characters c ON s.char_id=c.id WHERE c.name=?1 ORDER BY c.id DESC,s.id DESC LIMIT 1", name).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, false, nil
	}
	return n, err == nil, err
}
