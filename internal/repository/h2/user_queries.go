package h2

import (
	"context"
	"database/sql"
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
	selectLastOnline          = `SELECT message,created_on FROM messages WHERE (name = $1 or trip = $2) and visibility = 'PUBLIC' and (message not in ('LEFT','JOINED')) order by created_on desc limit 1`
	selectSessionJoined       = `SELECT created_on FROM (
		SELECT created_on FROM user_presence_log WHERE (name = $1 OR trip = $2) AND LOWER(event_type) = 'joined'
		UNION ALL
		SELECT created_on FROM messages WHERE (name = $1 OR trip = $2) AND message = 'JOINED'
	) session_joins ORDER BY created_on DESC LIMIT 1`
	selectUserTrips           = `SELECT trip FROM trips WHERE type = 'USER';`
	selectRecentPresenceNames = `SELECT name, MAX(created_on) AS last_seen FROM (
		SELECT name, created_on FROM user_presence_log
		WHERE (hash = $1 OR (trip = $2 AND trip IS NOT NULL AND trip <> '' AND trip <> 'null'))
		  AND LOWER(event_type) IN ('joined', 'left') AND created_on >= $3
		UNION ALL
		SELECT name, created_on FROM messages
		WHERE (hash = $1 OR (trip = $2 AND trip IS NOT NULL AND trip <> '' AND trip <> 'null'))
		  AND message IN ('JOINED', 'LEFT') AND created_on >= $3
	) recent_presence GROUP BY name ORDER BY last_seen DESC LIMIT $4`
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

func (d *Database) UserTrips(ctx context.Context) ([]string, error) {
	rows, err := d.DB.QueryContext(ctx, selectUserTrips)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trips []string
	for rows.Next() {
		var trip string
		if err := rows.Scan(&trip); err != nil {
			return nil, err
		}
		trips = append(trips, trip)
	}
	return trips, rows.Err()
}

func (d *Database) RecentPresenceNames(ctx context.Context, hash, trip string, afterMillis int64, limit int) ([]string, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("recent presence database is unavailable")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("recent presence limit must be positive")
	}
	rows, err := d.DB.QueryContext(ctx, selectRecentPresenceNames, hash, trip, afterMillis, fmt.Sprint(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0, limit)
	for rows.Next() {
		var name string
		var lastSeen int64
		if err := rows.Scan(&name, &lastSeen); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (d *Database) LastOnline(ctx context.Context, target string) (repository.LastOnlineRecord, error) {
	var record repository.LastOnlineRecord
	if err := d.DB.QueryRowContext(ctx, selectLastOnline, target, target).Scan(&record.LastMessage, &record.LastSeenMillis); err == sql.ErrNoRows {
		return record, nil
	} else if err != nil {
		return repository.LastOnlineRecord{}, err
	}
	record.Found = true
	// Saturn looks up the current session only after finding a last-seen row.
	if !record.LastSeenMillis.Valid {
		return record, nil
	}
	if err := d.DB.QueryRowContext(ctx, selectSessionJoined, target, target).Scan(&record.JoinedMillis); err != nil && err != sql.ErrNoRows {
		return repository.LastOnlineRecord{}, err
	}
	return record, nil
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
