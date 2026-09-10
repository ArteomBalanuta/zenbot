package h2

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/model"
)

// GrantTrip creates or updates the role associated with a trip.
func (d *Database) GrantTrip(ctx context.Context, trip string, role model.Role) error {
	trip = strings.TrimSpace(trip)
	roleName, err := roleDatabaseName(role)
	if err != nil {
		return err
	}
	if trip == "" {
		return fmt.Errorf("trip is required")
	}

	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM trips WHERE trip=$1", trip).Scan(&id)
	switch err {
	case nil:
		_, err = tx.ExecContext(ctx, "UPDATE trips SET type=$1 WHERE id=$2", roleName, id)
	case sql.ErrNoRows:
		_, err = tx.ExecContext(ctx, "INSERT INTO trips(type,trip,created_on) VALUES($1,$2,$3)", roleName, trip, time.Now().UnixMilli())
	}
	if err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

// ResolveRole returns REGULAR for an unrecognized or blank trip.
func (d *Database) ResolveRole(ctx context.Context, trip string) (model.Role, error) {
	trip = strings.TrimSpace(trip)
	if trip == "" {
		return model.REGULAR, nil
	}
	var roleName string
	if err := d.DB.QueryRowContext(ctx, "SELECT type FROM trips WHERE trip=$1", trip).Scan(&roleName); err != nil {
		if err == sql.ErrNoRows {
			return model.REGULAR, nil
		}
		return model.REGULAR, err
	}
	return roleFromDatabaseName(roleName)
}

// IsTripAuthorized applies application-configured trip allowlisting first,
// then the persisted role hierarchy. A configured "x" is the wildcard used
// by Saturn's application configuration.
func (d *Database) IsTripAuthorized(ctx context.Context, trip string, required model.Role, configuredTrips []string) (bool, error) {
	trip = strings.TrimSpace(trip)
	for _, configured := range configuredTrips {
		configured = strings.TrimSpace(configured)
		if strings.EqualFold(configured, "x") || strings.EqualFold(configured, trip) {
			return true, nil
		}
	}
	role, err := d.ResolveRole(ctx, trip)
	if err != nil {
		return false, err
	}
	return role <= required, nil
}

func roleDatabaseName(role model.Role) (string, error) {
	switch role {
	case model.ADMIN:
		return "ADMIN", nil
	case model.MODERATOR:
		return "MODERATOR", nil
	case model.TRUSTED:
		return "TRUSTED", nil
	case model.USER:
		return "USER", nil
	case model.REGULAR:
		return "REGULAR", nil
	case model.PEST:
		return "PEST", nil
	default:
		return "", fmt.Errorf("invalid role %d", role)
	}
}

func roleFromDatabaseName(name string) (model.Role, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "ADMIN":
		return model.ADMIN, nil
	case "MODERATOR":
		return model.MODERATOR, nil
	case "TRUSTED":
		return model.TRUSTED, nil
	case "USER":
		return model.USER, nil
	case "REGULAR":
		return model.REGULAR, nil
	case "PEST":
		return model.PEST, nil
	default:
		return model.REGULAR, fmt.Errorf("invalid persisted role %q", name)
	}
}
