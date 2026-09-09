package h2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"zenbot/internal/repository"
)

// ExecuteAgentQuery is Saturn's bounded named-query boundary. Query names—not
// generated SQL—select the fixed read-only statements.
func (d *Database) ExecuteAgentQuery(ctx context.Context, name string, raw json.RawMessage, room, trip string) (json.RawMessage, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("agent query database unavailable")
	}
	var args struct {
		Limit int    `json:"limit"`
		Nick  string `json:"nick"`
		Room  string `json:"room"`
		Trip  string `json:"trip"`
	}
	if len(raw) != 0 && json.Unmarshal(raw, &args) != nil {
		return nil, fmt.Errorf("invalid agent query arguments")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 60 {
		limit = 60
	}
	if strings.TrimSpace(args.Room) != "" {
		room = strings.TrimSpace(args.Room)
	}
	if strings.TrimSpace(args.Trip) != "" {
		trip = strings.TrimSpace(args.Trip)
	}
	count := func(sql string) (json.RawMessage, error) {
		var n int64
		err := d.DB.QueryRowContext(ctx, sql).Scan(&n)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]int64{"count": n})
	}
	switch name {
	case "message_count":
		return count("SELECT COUNT(*) FROM messages WHERE visibility='PUBLIC'")
	case "registered_user_count":
		return count("SELECT COUNT(*) FROM trips")
	case "recent_messages_for_requester":
		if strings.TrimSpace(trip) == "" {
			return json.RawMessage(`{"rows":[]}`), nil
		}
		rows, err := d.DB.QueryContext(ctx, `SELECT name,message,created_on,channel FROM messages WHERE trip=$1 AND visibility='PUBLIC' ORDER BY created_on DESC,id DESC LIMIT $2`, trip, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var name, message, channel string
			var created int64
			if err := rows.Scan(&name, &message, &created, &channel); err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"name": name, "message": message, "createdOn": created, "channel": channel})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"rows": out})
	case "recent_messages_for_user":
		if strings.TrimSpace(args.Nick) == "" {
			return nil, fmt.Errorf("nick is required")
		}
		query := `SELECT name,trip,hash,message,created_on,channel FROM messages WHERE LOWER(name)=LOWER($1) AND visibility='PUBLIC' ORDER BY created_on DESC,id DESC LIMIT $2`
		params := []any{strings.TrimSpace(args.Nick), limit}
		if strings.TrimSpace(room) != "" {
			query = `SELECT name,trip,hash,message,created_on,channel FROM messages WHERE LOWER(name)=LOWER($1) AND LOWER(channel)=LOWER($2) AND visibility='PUBLIC' ORDER BY created_on DESC,id DESC LIMIT $3`
			params = []any{strings.TrimSpace(args.Nick), room, limit}
		}
		rows, err := d.DB.QueryContext(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var name, tripValue, hash, message, channel string
			var created int64
			if err := rows.Scan(&name, &tripValue, &hash, &message, &created, &channel); err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"name": name, "trip": tripValue, "hash": hash, "message": message, "createdOn": created, "channel": channel})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"rows": out})
	case "recent_messages_for_room":
		rows, err := d.RecentPublicRoomMessages(ctx, room, limit)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"rows": rows})
	case "known_nicks_for_trip":
		if strings.TrimSpace(trip) == "" {
			return json.RawMessage(`{"rows":[]}`), nil
		}
		rows, err := d.DB.QueryContext(ctx, `SELECT DISTINCT n.name FROM trips t JOIN trip_names tn ON tn.trip_id=t.id JOIN names n ON n.id=tn.name_id WHERE t.trip=$1 ORDER BY n.name LIMIT $2`, trip, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []map[string]string{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			out = append(out, map[string]string{"name": name})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"rows": out})
	default:
		return nil, fmt.Errorf("unknown agent database query: %s", name)
	}
}

var _ repository.AgentNamedQueryRepository = (*Database)(nil)
