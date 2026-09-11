package h2

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"zenbot/internal/repository"
	"zenbot/internal/util"
)

const (
	selectRegisteredUsers = `select distinct t.trip,n.name from trip_names tn inner join names n on tn.name_id=n.id inner join trips t on tn.trip_id=t.id order by n.name desc`
	selectNicksByTrip     = `SELECT name FROM (
		SELECT name, created_on FROM messages WHERE trip = $1 AND visibility = 'PUBLIC'
		UNION ALL
		SELECT name, created_on FROM user_presence_log WHERE trip = $1 AND LOWER(event_type) IN ('joined', 'left')
	) observations WHERE name IS NOT NULL AND TRIM(name) <> ''
	GROUP BY name ORDER BY MAX(created_on) DESC, name ASC`
	selectBasicUserDataByHash = `select distinct hash,name from messages where hash=$1 limit 30`
	selectBasicUserDataByTrip = `select distinct hash,name from messages where trip=$1 limit 30`
	// One statement keeps ambiguity detection and both facts on the same read.
	// Source rank breaks equal timestamps deterministically, not causally; ids
	// only order rows within the same source table.
	selectLastOnline = `SELECT message, created_on, event_type, name_match, trip_match, differing FROM (
		SELECT observations.*,
			MAX(CASE WHEN name = $1 THEN 1 ELSE 0 END) OVER () AS name_match,
			MAX(CASE WHEN trip = $2 THEN 1 ELSE 0 END) OVER () AS trip_match,
			MAX(CASE WHEN name = $1 AND trip = $2 THEN 0 ELSE 1 END) OVER () AS differing,
			ROW_NUMBER() OVER (
				PARTITION BY CASE WHEN event_type IS NOT NULL THEN 1 WHEN message IS NOT NULL THEN 0 ELSE 2 END
				ORDER BY created_on DESC, source_rank DESC, id DESC, event_type DESC
			) AS fact_rank
		FROM (
			SELECT id, name, trip, message, created_on, 0 AS source_rank,
				CASE WHEN message IN ('JOINED', 'LEFT') THEN message ELSE NULL END AS event_type
			FROM messages WHERE visibility = 'PUBLIC'
			UNION ALL
			SELECT id, name, trip, CAST(NULL AS VARCHAR), created_on, 1 AS source_rank, UPPER(event_type)
			FROM user_presence_log WHERE LOWER(event_type) IN ('joined', 'left')
		) observations WHERE name = $1 OR trip = $2
	) ranked WHERE fact_rank = 1`
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
	rows, err := d.DB.QueryContext(ctx, selectNicksByTrip, trip)
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
		if strings.TrimSpace(nick) == "" {
			continue
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
	target = strings.TrimSpace(target)
	nick, err := util.NormalizeNickTarget(&target)
	if err != nil {
		return record, err
	}
	rows, err := d.DB.QueryContext(ctx, selectLastOnline, nick, target)
	if err != nil {
		return repository.LastOnlineRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var message, event sql.NullString
		var timestamp sql.NullInt64
		var nameMatch, tripMatch, differing int
		if err := rows.Scan(&message, &timestamp, &event, &nameMatch, &tripMatch, &differing); err != nil {
			return repository.LastOnlineRecord{}, err
		}
		if nameMatch != 0 && tripMatch != 0 && differing != 0 {
			return repository.LastOnlineRecord{}, fmt.Errorf("%w target %q", repository.ErrAmbiguousHistory, target)
		}
		if event.Valid {
			record.LastPresenceEvent, record.LastPresenceMillis = event, timestamp
		} else if message.Valid {
			record.LastMessage, record.LastMessageMillis = message, timestamp
		}
	}
	if err := rows.Err(); err != nil {
		return repository.LastOnlineRecord{}, err
	}
	record.Found = record.LastMessageMillis.Valid || record.LastPresenceMillis.Valid
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
	return fmt.Sprintf("Hashes: \n%s \nNicks: \n%s \n", join(hashes), join(nicks)), nil
}
