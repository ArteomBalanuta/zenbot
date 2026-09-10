package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
)

const roomUsersName = "room_users"
const defaultRoomUsersMaximum = 200

var unicodeCaseFold = cases.Fold()

// RoomUserSnapshot is a read-only managed-room user snapshot.
type RoomUserSnapshot = contract.RoomUserSnapshot

// RoomUserDirectory is intentionally narrow so agent tools do not depend on core.
type RoomUserDirectory interface {
	FindRoomUsers(room string) (RoomUserSnapshot, bool)
}

// RoomUsers exposes one current public managed-room user snapshot.
type RoomUsers struct {
	Directory RoomUserDirectory
	MaxUsers  int
}

func (t RoomUsers) Name() string { return roomUsersName }

func (t RoomUsers) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"room":{"type":"string","minLength":1,"maxLength":100}}}`)
	result := contract.SchemaObject(map[string]json.RawMessage{
		"room":          json.RawMessage(`{"type":"string"}`),
		"users":         json.RawMessage(`{"type":"array","items":{"type":"string"}}`),
		"count":         json.RawMessage(`{"type":"integer"}`),
		"returnedCount": json.RawMessage(`{"type":"integer"}`),
		"truncated":     json.RawMessage(`{"type":"boolean"}`),
	}, []string{"room", "users", "count", "returnedCount", "truncated"}, false)
	return contract.NewDescriptor(roomUsersName, "Managed room users", "List the current users in one managed public room.", "managed-room-users", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 2*time.Second, result, []string{"managed_room_users"}, nil, []string{"Do not use for whisper or private rooms, unmanaged rooms, or historical messages."})
}

func (t RoomUsers) Execute(_ context.Context, agent api.Context, args json.RawMessage) (contract.Result, error) {
	if t.Directory == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil || raw == nil {
		return contract.Result{}, fmt.Errorf("invalid room users arguments")
	}
	for name := range raw {
		if name != "room" {
			return contract.Result{}, fmt.Errorf("invalid room users arguments")
		}
	}
	var input struct {
		Room *string `json:"room"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return contract.Result{}, fmt.Errorf("invalid room users arguments")
	}
	room := agent.Room()
	if input.Room != nil {
		room = *input.Room
	}
	room = strings.TrimSpace(room)
	if room == "" || len([]rune(room)) > 100 {
		return contract.Result{}, fmt.Errorf("invalid room users room")
	}
	snapshot, ok := t.Directory.FindRoomUsers(room)
	if !ok || strings.TrimSpace(snapshot.Room) == "" {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	users := make([]string, 0, len(snapshot.Users))
	for _, user := range snapshot.Users {
		if strings.TrimSpace(user) != "" {
			users = append(users, user)
		}
	}
	sort.Slice(users, func(i, j int) bool {
		left, right := unicodeCaseFold.String(users[i]), unicodeCaseFold.String(users[j])
		if left == right {
			return users[i] < users[j]
		}
		return left < right
	})
	count := len(users)
	maximum := t.MaxUsers
	if maximum <= 0 {
		maximum = defaultRoomUsersMaximum
	}
	if len(users) > maximum {
		users = users[:maximum]
	}
	content, err := json.Marshal(struct {
		Room          string   `json:"room"`
		Users         []string `json:"users"`
		Count         int      `json:"count"`
		ReturnedCount int      `json:"returnedCount"`
		Truncated     bool     `json:"truncated"`
	}{Room: snapshot.Room, Users: users, Count: count, ReturnedCount: len(users), Truncated: count > len(users)})
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	return contract.SuccessResult("", t.Name(), string(content)), nil
}

var _ Tool = RoomUsers{}
