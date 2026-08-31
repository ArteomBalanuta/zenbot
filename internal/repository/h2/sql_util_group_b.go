package h2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/repository"
)

const (
	deleteTripNames = `DELETE FROM trip_names WHERE trip_id IN (
        SELECT id FROM trips WHERE trip = ?
) OR name_id IN (
SELECT id FROM names WHERE name = ?
);`
	deleteTrip               = `DELETE FROM trips WHERE trip = ?;`
	deleteName               = `DELETE FROM names WHERE name = ?;`
	selectNameTripRegistered = `SELECT DISTINCT n.name,t.trip
FROM trip_names tn
INNER JOIN trips t on tn.trip_id = t.id
INNER JOIN names n on tn.name_id = n.id ORDER BY t.trip DESC;`
	selectLastNMessages = `SELECT name,message,created_on FROM messages WHERE (name = ? or trip = ?) and (message not
in ('LEFT','JOINED')) order by created_on desc limit ?;`
)

var errSaturnUnauthorized = errors.New("saturn compatibility operation requires authorization")

type saturnAuthorizationKey struct{}

func withSaturnAuthorization(ctx context.Context) context.Context {
	return context.WithValue(ctx, saturnAuthorizationKey{}, true)
}
func authorizedSaturnContext(ctx context.Context) bool {
	v, _ := ctx.Value(saturnAuthorizationKey{}).(bool)
	return v
}

// DeleteIdentityAuthorized resolves one Saturn name-or-trip selector while
// keeping the authorization context and two-field delete private to H2.
func (d *Database) DeleteIdentityAuthorized(ctx context.Context, nameOrTrip string) (repository.DeleteResult, error) {
	value := strings.TrimSpace(nameOrTrip)
	if value == "" {
		return repository.DeleteResult{}, fmt.Errorf("identity selector must match exactly one registered user")
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT DISTINCT n.name,t.trip FROM trip_names tn JOIN names n ON n.id=tn.name_id JOIN trips t ON t.id=tn.trip_id WHERE LOWER(n.name)=LOWER($1) OR LOWER(t.trip)=LOWER($2)`, value, value)
	if err != nil {
		return repository.DeleteResult{}, err
	}
	defer rows.Close()
	var name, trip string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return repository.DeleteResult{}, err
		}
		return repository.DeleteResult{}, fmt.Errorf("identity selector must match exactly one registered user")
	}
	if err := rows.Scan(&name, &trip); err != nil {
		return repository.DeleteResult{}, err
	}
	if rows.Next() {
		return repository.DeleteResult{}, fmt.Errorf("identity selector must match exactly one registered user")
	}
	if err := rows.Err(); err != nil {
		return repository.DeleteResult{}, err
	}
	return d.DeleteIdentity(withSaturnAuthorization(ctx), name, trip)
}

// DeleteIdentity is an unwired compatibility operation. Only an already
// authorized context created inside this repository package can invoke it.
func (d *Database) DeleteIdentity(ctx context.Context, name, trip string) (repository.DeleteResult, error) {
	if !authorizedSaturnContext(ctx) {
		return repository.DeleteResult{}, errSaturnUnauthorized
	}
	return d.deleteIdentity(ctx, name, trip, d.execDelete)
}

type deleteExecutor func(context.Context, *sql.Tx, string, ...any) (sql.Result, error)

func (d *Database) execDelete(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(ctx, query, args...)
}
func (d *Database) deleteIdentity(ctx context.Context, name, trip string, exec deleteExecutor) (repository.DeleteResult, error) {
	var result repository.DeleteResult
	err := d.WithTx(ctx, func(tx *sql.Tx) error {
		r, err := exec(ctx, tx, "DELETE FROM trip_names WHERE trip_id IN (SELECT id FROM trips WHERE trip = $1) OR name_id IN (SELECT id FROM names WHERE name = $2)", trip, name)
		if err != nil {
			return err
		}
		if result.TripNamesRows, err = r.RowsAffected(); err != nil {
			return err
		}
		r, err = exec(ctx, tx, "DELETE FROM trips WHERE trip = $1", trip)
		if err != nil {
			return err
		}
		if result.TripRows, err = r.RowsAffected(); err != nil {
			return err
		}
		r, err = exec(ctx, tx, "DELETE FROM names WHERE name = $1", name)
		if err != nil {
			return err
		}
		result.NameRows, err = r.RowsAffected()
		return err
	})
	if err != nil {
		return repository.DeleteResult{}, err
	}
	return result, nil
}

func (d *Database) SaturnRegisteredUsers(ctx context.Context) ([]repository.SaturnRegisteredUser, error) {
	rows, err := d.DB.QueryContext(ctx, selectNameTripRegistered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]repository.SaturnRegisteredUser, 0)
	for rows.Next() {
		var user repository.SaturnRegisteredUser
		if err := rows.Scan(&user.Name, &user.Trip); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (d *Database) SaturnLastMessages(ctx context.Context, name *string, trip string, count int) ([]repository.SaturnLastMessage, error) {
	if count <= 0 {
		count = 5
	}
	rows, err := d.DB.QueryContext(ctx, fmt.Sprintf("SELECT name,message,created_on FROM messages WHERE (name = $1 or trip = $2) and (message not in ('LEFT','JOINED')) order by created_on desc limit %d", count), name, trip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]repository.SaturnLastMessage, 0)
	for rows.Next() {
		var message repository.SaturnLastMessage
		if err := rows.Scan(&message.Name, &message.Message, &message.CreatedOn); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

var _ repository.SqlUtilGroupBRepository = (*Database)(nil)
var _ repository.SaturnAuthorizedDeleteRepository = (*Database)(nil)
