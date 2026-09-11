package h2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func (d *Database) IsNameRegistered(ctx context.Context, name string) (bool, error) {
	var n int
	err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM names WHERE LOWER(name)=LOWER($1)", strings.TrimSpace(name)).Scan(&n)
	if err == nil && n > 1 {
		return false, fmt.Errorf("ambiguous registered name %q", name)
	}
	return n > 0, err
}
func (d *Database) IsTripRegistered(ctx context.Context, trip string) (bool, error) {
	var n int
	err := d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM trips WHERE trip=$1", strings.TrimSpace(trip)).Scan(&n)
	return n > 0, err
}

func (d *Database) Register(ctx context.Context, name, trip string, role model.Role) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	roleName, err := roleDatabaseName(role)
	if err != nil {
		return err
	}
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		var nameID, tripID int64
		if _, err := tx.ExecContext(ctx, "INSERT INTO names(name,created_on) VALUES($1,$2)", name, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT id FROM names WHERE name=$1", name).Scan(&nameID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO trips(type,trip,created_on) VALUES($1,$2,$3)", roleName, trip, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT id FROM trips WHERE trip=$1", trip).Scan(&tripID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}

func (d *Database) RegisterNameByTrip(ctx context.Context, name, trip string) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		var tripID int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM trips WHERE trip=$1", trip).Scan(&tripID); err != nil {
			return err
		}
		var nameID int64
		if _, err := tx.ExecContext(ctx, "INSERT INTO names(name,created_on) VALUES($1,$2)", name, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT id FROM names WHERE name=$1", name).Scan(&nameID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}
func (d *Database) RegisterTripByName(ctx context.Context, name, trip string) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(ctx, func(tx *sql.Tx) error {
		var nameID int64
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(MIN(id),0) FROM names WHERE LOWER(name)=LOWER($1)", name).Scan(&count, &nameID); err != nil {
			return err
		}
		if count == 0 {
			return sql.ErrNoRows
		}
		if count != 1 {
			return fmt.Errorf("ambiguous registered name %q", name)
		}
		var tripID int64
		if _, err := tx.ExecContext(ctx, "INSERT INTO trips(type,trip,created_on) VALUES('REGULAR',$1,$2)", trip, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT id FROM trips WHERE trip=$1", trip).Scan(&tripID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}

func (d *Database) LastSeen(ctx context.Context, target string) (repository.LastSeen, error) {
	var out repository.LastSeen
	if err := d.DB.QueryRowContext(ctx, "SELECT message,created_on FROM messages WHERE (name=$1 OR trip=$2) AND visibility='PUBLIC' AND message NOT IN ('LEFT','JOINED') ORDER BY created_on DESC,id DESC LIMIT 1", target, target).Scan(&out.Message, &out.SeenAt); err != nil && err != sql.ErrNoRows {
		return out, err
	}
	if err := d.DB.QueryRowContext(ctx, "SELECT created_on FROM messages WHERE (name=$1 OR trip=$2) AND message='JOINED' ORDER BY created_on DESC,id DESC LIMIT 1", target, target).Scan(&out.JoinedAt); err != nil && err != sql.ErrNoRows {
		return out, err
	}
	return out, nil
}

func (d *Database) LastMessages(ctx context.Context, name, trip string, count int) ([]model.Message, error) {
	if count <= 0 {
		count = 5
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT id,trip,name,hash,message,created_on,channel FROM messages WHERE (name=$1 OR trip=$2) AND visibility='PUBLIC' AND message NOT IN ('LEFT','JOINED') ORDER BY created_on DESC,id DESC LIMIT $3", name, trip, fmt.Sprint(count))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Message
	for rows.Next() {
		var m model.Message
		var trip, name, hash, text, channel sql.NullString
		if err := rows.Scan(&m.ID, &trip, &name, &hash, &text, &m.CreatedOn, &channel); err != nil {
			return nil, err
		}
		m.Trip, m.Name, m.Hash, m.Message, m.Channel = trip.String, name.String, hash.String, text.String, channel.String
		out = append(out, m)
	}
	return out, rows.Err()
}
