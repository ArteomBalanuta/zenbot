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
		Room  string `json:"room"`
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
	case "recent_messages_for_room":
		rows, err := d.RecentPublicRoomMessages(ctx, room, limit)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, map[string]any{"name": row.Name, "message": row.Message, "createdOn": row.CreatedOnMillis, "channel": row.Channel})
		}
		return json.Marshal(map[string]any{"rows": out})
	default:
		return nil, fmt.Errorf("unknown agent database query: %s", name)
	}
}

var _ repository.AgentNamedQueryRepository = (*Database)(nil)
