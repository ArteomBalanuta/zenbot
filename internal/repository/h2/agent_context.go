package h2

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"zenbot/internal/repository"
)

const recentPublicRoomMessagesSQL = `SELECT name, trip, hash, message, created_on, channel
FROM (
  SELECT id, name, trip, hash, message, created_on, channel
  FROM messages
  WHERE LOWER(channel) = LOWER($1)
    AND visibility = 'PUBLIC'
  ORDER BY created_on DESC, id DESC
  LIMIT $2
) recent
ORDER BY created_on ASC, id ASC`

const recentPublicRoomMessagesForNickSQL = `SELECT name, trip, hash, message, created_on, channel
FROM (
  SELECT id, name, trip, hash, message, created_on, channel
  FROM messages
  WHERE LOWER(channel) = LOWER($1)
    AND LOWER(name) = LOWER($2)
    AND visibility = 'PUBLIC'
  ORDER BY created_on DESC, id DESC
  LIMIT $3
) recent
ORDER BY created_on ASC, id ASC`

// RecentPublicRoomMessages returns the newest bounded public messages for one room
// in chronological order.
func (d *Database) RecentPublicRoomMessages(ctx context.Context, room string, limit int) ([]repository.PublicRoomMessage, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("recent public room messages: database is not initialized")
	}
	if strings.TrimSpace(room) == "" {
		return nil, fmt.Errorf("recent public room messages: room is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("recent public room messages: limit must be positive")
	}
	rows, err := d.DB.QueryContext(ctx, recentPublicRoomMessagesSQL, room, strconv.Itoa(limit))
	if err != nil {
		return nil, fmt.Errorf("recent public room messages: %w", err)
	}
	defer rows.Close()

	var out []repository.PublicRoomMessage
	for rows.Next() {
		var row repository.PublicRoomMessage
		var name, trip, hash, message, channel sql.NullString
		if err := rows.Scan(&name, &trip, &hash, &message, &row.CreatedOnMillis, &channel); err != nil {
			return nil, fmt.Errorf("recent public room messages: %w", err)
		}
		row.Name, row.Trip, row.Hash, row.Message, row.Channel = name.String, trip.String, hash.String, message.String, channel.String
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent public room messages: %w", err)
	}
	return out, nil
}

// RecentPublicRoomMessagesForNick returns the newest bounded public messages
// for one nick in one trusted room, ordered chronologically.
func (d *Database) RecentPublicRoomMessagesForNick(ctx context.Context, room, nick string, limit int) ([]repository.PublicRoomMessage, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("recent public room messages for nick: database is not initialized")
	}
	if strings.TrimSpace(room) == "" {
		return nil, fmt.Errorf("recent public room messages for nick: room is required")
	}
	if strings.TrimSpace(nick) == "" {
		return nil, fmt.Errorf("recent public room messages for nick: nick is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("recent public room messages for nick: limit must be positive")
	}
	rows, err := d.DB.QueryContext(ctx, recentPublicRoomMessagesForNickSQL, room, nick, strconv.Itoa(limit))
	if err != nil {
		return nil, fmt.Errorf("recent public room messages for nick: %w", err)
	}
	defer rows.Close()
	var out []repository.PublicRoomMessage
	for rows.Next() {
		var row repository.PublicRoomMessage
		var name, trip, hash, message, channel sql.NullString
		if err := rows.Scan(&name, &trip, &hash, &message, &row.CreatedOnMillis, &channel); err != nil {
			return nil, fmt.Errorf("recent public room messages for nick: %w", err)
		}
		row.Name, row.Trip, row.Hash, row.Message, row.Channel = name.String, trip.String, hash.String, message.String, channel.String
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recent public room messages for nick: %w", err)
	}
	return out, nil
}

var _ repository.AgentConversationRepository = (*Database)(nil)
var _ repository.AgentUserMessageHistoryRepository = (*Database)(nil)
