package repository

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DatabaseRepositoryImpl struct {
	DB *sql.DB
}

func (r *DatabaseRepositoryImpl) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	timestampMillis := time.Now().UnixNano() / int64(time.Millisecond)

	result, err := r.DB.Exec(
		`INSERT INTO messages ('trip', 'name', 'hash', 'message', 'created_on', 'channel') VALUES (?, ?, ?, ?, ?, ?)`,
		trip, name, hash, message, timestampMillis, channel,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (r *DatabaseRepositoryImpl) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	timestampMillis := time.Now().UnixNano() / int64(time.Millisecond)

	result, err := r.DB.Exec(
		`INSERT INTO user_presence_log ('trip', 'name', 'hash', 'event_type', 'created_on', 'channel') VALUES (?, ?, ?, ?, ?, ?)`,
		trip, name, hash, eventType, timestampMillis, channel,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func NewRepository(dbPath string) (Repository, error) {
	//TODO: file doesnt exist yet sqlite succeeds
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	return &DatabaseRepositoryImpl{DB: db}, nil
}
