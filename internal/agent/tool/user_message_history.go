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

// UserMessageHistory exposes only bounded public history in the invocation's
// trusted current room. Room and limit are deliberately not model arguments.
type UserMessageHistory struct {
	Repository repository.AgentUserMessageHistoryRepository
	Limit      int
}

func (t UserMessageHistory) Name() string { return userMessageHistoryName }

func (t UserMessageHistory) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"nick":{"type":"string","minLength":1,"maxLength":100}},"required":["nick"]}`)
	result := contract.SchemaObject(map[string]json.RawMessage{
		"rows":          json.RawMessage(`{"type":"array"}`),
		"returnedCount": json.RawMessage(`{"type":"integer"}`),
	}, []string{"rows", "returnedCount"}, false)
	return contract.NewDescriptor(userMessageHistoryName, "User message history", "Read recent public messages by one named user in this room.", "history", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 2*time.Second, result, []string{"messages"}, nil, []string{"Do not use for private or cross-room history."})
}

func (t UserMessageHistory) Execute(ctx context.Context, agent api.Context, args json.RawMessage) (contract.Result, error) {
	if t.Repository == nil || t.Limit <= 0 {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	var input struct {
		Nick string `json:"nick"`
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
	room := agent.Room()
	if strings.TrimSpace(room) == "" {
		return contract.Result{}, fmt.Errorf("trusted room is required")
	}
	rows, err := t.Repository.RecentPublicRoomMessagesForNick(ctx, room, nick, t.Limit)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	type row struct {
		Name      string `json:"name"`
		Message   string `json:"message"`
		CreatedOn int64  `json:"createdOn"`
		Channel   string `json:"channel"`
	}
	out := make([]row, 0, len(rows))
	for _, item := range rows {
		out = append(out, row{Name: item.Name, Message: item.Message, CreatedOn: item.CreatedOnMillis, Channel: item.Channel})
	}
	content, err := json.Marshal(struct {
		Rows          []row `json:"rows"`
		ReturnedCount int   `json:"returnedCount"`
	}{Rows: out, ReturnedCount: len(out)})
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "tool execution failed"), nil
	}
	return contract.SuccessResult("", t.Name(), string(content)), nil
}

var _ Tool = UserMessageHistory{}
