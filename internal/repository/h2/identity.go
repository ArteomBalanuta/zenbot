package h2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/model"
)

func (d *Database) IsNameRegistered(name string) (bool, error) {
	var n int
	err := d.DB.QueryRow("SELECT COUNT(*) FROM names WHERE LOWER(name)=LOWER($1)", strings.TrimSpace(name)).Scan(&n)
	return n > 0, err
}
func (d *Database) IsTripRegistered(trip string) (bool, error) {
	var n int
	err := d.DB.QueryRow("SELECT COUNT(*) FROM trips WHERE LOWER(trip)=LOWER($1)", strings.TrimSpace(trip)).Scan(&n)
	return n > 0, err
}

func (d *Database) Register(name, trip string, role model.Role) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	roleName, err := roleDatabaseName(role)
	if err != nil {
		return err
	}
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(context.Background(), func(tx *sql.Tx) error {
		var nameID, tripID int64
		if _, err := tx.Exec("INSERT INTO names(name,created_on) VALUES($1,$2)", name, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM names WHERE name=$1", name).Scan(&nameID); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO trips(type,trip,created_on) VALUES($1,$2,$3)", roleName, trip, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM trips WHERE trip=$1", trip).Scan(&tripID); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}

func (d *Database) RegisterNameByTrip(name, trip string) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(context.Background(), func(tx *sql.Tx) error {
		var tripID int64
		if err := tx.QueryRow("SELECT id FROM trips WHERE LOWER(trip)=LOWER($1)", trip).Scan(&tripID); err != nil {
			return err
		}
		var nameID int64
		if _, err := tx.Exec("INSERT INTO names(name,created_on) VALUES($1,$2)", name, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM names WHERE name=$1", name).Scan(&nameID); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}
func (d *Database) RegisterTripByName(name, trip string) error {
	name, trip = strings.TrimSpace(name), strings.TrimSpace(trip)
	if name == "" || trip == "" {
		return fmt.Errorf("name and trip are required")
	}
	return d.WithTx(context.Background(), func(tx *sql.Tx) error {
		var nameID int64
		if err := tx.QueryRow("SELECT id FROM names WHERE LOWER(name)=LOWER($1)", name).Scan(&nameID); err != nil {
			return err
		}
		var tripID int64
		if _, err := tx.Exec("INSERT INTO trips(type,trip,created_on) VALUES('REGULAR',$1,$2)", trip, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM trips WHERE trip=$1", trip).Scan(&tripID); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO trip_names(trip_id,name_id) VALUES($1,$2)", tripID, nameID)
		return err
	})
}

func (d *Database) LastMessages(name, trip string, count int) ([]model.Message, error) {
	if count <= 0 {
		count = 5
	}
	rows, err := d.DB.Query(fmt.Sprintf("SELECT id,trip,name,hash,message,created_on,channel FROM messages WHERE (name=$1 OR trip=$2) AND visibility='PUBLIC' AND message NOT IN ('LEFT','JOINED') ORDER BY created_on DESC,id DESC LIMIT %d", count), name, trip)
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
