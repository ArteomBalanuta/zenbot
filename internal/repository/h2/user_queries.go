package h2

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"zenbot/internal/repository"
)

const (
	selectRegisteredUsers     = `select distinct t.trip,n.name from trip_names tn inner join names n on tn.name_id=n.id inner join trips t on tn.trip_id=t.id order by n.name desc`
	selectNicksByTrip         = `SELECT DISTINCT name FROM messages WHERE LOWER(trip)=$1`
	selectBasicUserDataByHash = `select distinct hash,name from messages where hash=$1 limit 30`
	selectBasicUserDataByTrip = `select distinct hash,name from messages where trip=$1 limit 30`
)

func (d *Database) RegisteredUsers(ctx context.Context) ([]repository.RegisteredUser, error) {
	rows, err := d.DB.QueryContext(ctx, selectRegisteredUsers)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []repository.RegisteredUser
	for rows.Next() {
		var user repository.RegisteredUser
		if err := rows.Scan(&user.Trip, &user.Name); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (d *Database) NicksByTrip(ctx context.Context, trip string) ([]string, error) {
	rows, err := d.DB.QueryContext(ctx, selectNicksByTrip, strings.ToLower(trip))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nicks []string
	for rows.Next() {
		var nick string
		if err := rows.Scan(&nick); err != nil {
			return nil, err
		}
		nicks = append(nicks, nick)
	}
	return nicks, rows.Err()
}

func (d *Database) BasicUserData(ctx context.Context, hash, trip string) (string, error) {
	query, arg := selectBasicUserDataByTrip, trip
	if strings.TrimSpace(trip) == "" {
		query, arg = selectBasicUserDataByHash, hash
	}
	rows, err := d.DB.QueryContext(ctx, query, arg)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	hashes, nicks := map[string]struct{}{}, map[string]struct{}{}
	for rows.Next() {
		var h, n string
		if err := rows.Scan(&h, &n); err != nil {
			return "", err
		}
		hashes[h] = struct{}{}
		nicks[n] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	join := func(values map[string]struct{}) string {
		out := make([]string, 0, len(values))
		for value := range values {
			out = append(out, value)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	return fmt.Sprintf("Hashes: \\n%s \\nNicks: \\n%s \\n", join(hashes), join(nicks)), nil
}
