package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/repository"
)

const userMessageHistoryName = "user_message_history"
const MaxUserMessageHistory = 500

// UserMessageHistory exposes bounded public history across rooms by default.
// A model may optionally narrow the lookup to one room or request fewer rows.
type UserMessageHistory struct {
	Repository repository.AgentUserMessageHistoryRepository
	Limit      int
}

func (t UserMessageHistory) Name() string { return userMessageHistoryName }

func (t UserMessageHistory) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"nick":{"type":"string","minLength":1,"maxLength":100},"room":{"type":"string","minLength":1,"maxLength":100},"limit":{"type":"integer","minimum":1,"maximum":500}},"required":["nick"]}`)
	result := contract.SchemaObject(map[string]json.RawMessage{
		"rows":            json.RawMessage(`{"type":"array"}`),
		"returnedCount":   json.RawMessage(`{"type":"integer"}`),
		"oldestCreatedOn": json.RawMessage(`{"type":"any"}`),
		"newestCreatedOn": json.RawMessage(`{"type":"any"}`),
	}, []string{"rows", "returnedCount", "oldestCreatedOn", "newestCreatedOn"}, false)
	return contract.NewDescriptor(userMessageHistoryName, "User message history", "Fetch up to 500 latest public messages by one named user across all rooms; pass room only to restrict the search.", "history", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 2*time.Second, result, []string{"messages"}, nil, []string{"Do not use for whispers or current-presence claims."})
}

func (t UserMessageHistory) Execute(ctx context.Context, agent api.Context, args json.RawMessage) (contract.Result, error) {
	if t.Repository == nil || t.Limit <= 0 {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	var input struct {
		Nick  string `json:"nick"`
		Room  string `json:"room"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return contract.Result{}, fmt.Errorf("invalid user message history arguments")
	}
	nick := strings.TrimSpace(input.Nick)
	nick = strings.TrimPrefix(nick, "@")
	nick = strings.TrimSpace(nick)
	if nick == "" || len([]rune(nick)) > 100 {
		return contract.Result{}, fmt.Errorf("invalid user message history nick")
	}
	limit := input.Limit
	if limit == 0 {
		limit = t.Limit
	}
	if limit <= 0 || limit > t.Limit || limit > MaxUserMessageHistory {
		return contract.Result{}, fmt.Errorf("invalid user message history limit")
	}
	room := strings.TrimSpace(input.Room)
	rows, err := t.Repository.RecentPublicRoomMessagesForNick(ctx, room, nick, limit)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	type row struct {
		Name      string `json:"name"`
		Trip      string `json:"trip"`
		Hash      string `json:"hash"`
		Message   string `json:"message"`
		CreatedOn int64  `json:"createdOn"`
		Channel   string `json:"channel"`
	}
	out := make([]row, 0, len(rows))
	var oldest, newest *int64
	for _, item := range rows {
		out = append(out, row{Name: item.Name, Trip: item.Trip, Hash: item.Hash, Message: item.Message, CreatedOn: item.CreatedOnMillis, Channel: item.Channel})
		if oldest == nil || item.CreatedOnMillis < *oldest {
			value := item.CreatedOnMillis
			oldest = &value
		}
		if newest == nil || item.CreatedOnMillis > *newest {
			value := item.CreatedOnMillis
			newest = &value
		}
	}
	content, err := json.Marshal(struct {
		Rows            []row  `json:"rows"`
		ReturnedCount   int    `json:"returnedCount"`
		OldestCreatedOn *int64 `json:"oldestCreatedOn"`
		NewestCreatedOn *int64 `json:"newestCreatedOn"`
	}{Rows: out, ReturnedCount: len(out), OldestCreatedOn: oldest, NewestCreatedOn: newest})
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	return contract.SuccessResult("", t.Name(), string(content)), nil
}

var _ Tool = UserMessageHistory{}
