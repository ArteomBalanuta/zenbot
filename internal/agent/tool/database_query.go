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

const databaseQueryName = "database_query"

// DatabaseQuery implements Saturn's fixed named read-only query surface. It
// deliberately delegates only query names to the repository, never SQL text.
type DatabaseQuery struct {
	Repository repository.AgentNamedQueryRepository
}

func (t DatabaseQuery) Name() string { return databaseQueryName }
func (t DatabaseQuery) Descriptor(api.Context) (contract.Descriptor, error) {
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
		"query":{"type":"string","enum":["message_count","registered_user_count","recent_messages_for_requester","recent_messages_for_room"],"description":"Count all stored public messages or registered trips, or fetch latest public messages for the caller's trip across rooms or for one named room."},
		"room":{"type":"string","minLength":1,"maxLength":100,"description":"Required only for recent_messages_for_room; omit for every other query."},
		"limit":{"type":"integer","minimum":1,"maximum":60,"description":"Maximum rows for recent-message queries (default 10); omit for count queries."}
	},"required":["query"]}`)
	row := `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"},"message":{"type":"string"},"createdOn":{"type":"integer"},"channel":{"type":"string"}},"required":["name","message","createdOn","channel"]}`
	result := json.RawMessage(`{"oneOf":[
		{"type":"object","additionalProperties":false,"properties":{"count":{"type":"integer","minimum":0}},"required":["count"]},
		{"type":"object","additionalProperties":false,"properties":{"rows":{"type":"array","items":` + row + `,"maxItems":60}},"required":["rows"]}
	]}`)
	return contract.NewDescriptor(databaseQueryName, "Approved database query", "Run one fixed read-only database query.", "database", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 2*time.Second, result, []string{"database"}, nil, []string{"Do not use for generated SQL or private data."}, contract.WithPrimaryIntent("approved_database_query"), contract.WithMaxModelResultBytes(8192))
}
func (t DatabaseQuery) Execute(ctx context.Context, agent api.Context, raw json.RawMessage) (contract.Result, error) {
	if t.Repository == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "database query failed"), nil
	}
	descriptor, err := t.Descriptor(agent)
	if err != nil {
		return contract.Result{}, err
	}
	if err := contract.ValidateArguments(descriptor.Parameters(), raw); err != nil {
		return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", err.Error()), nil
	}
	var input struct {
		Query string  `json:"query"`
		Room  *string `json:"room"`
		Limit *int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Query) == "" {
		return contract.Result{}, fmt.Errorf("query is required")
	}
	switch input.Query {
	case "message_count", "registered_user_count":
		if input.Room != nil || input.Limit != nil {
			return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", "Count queries accept only query; omit room and limit."), nil
		}
	case "recent_messages_for_requester":
		if input.Room != nil {
			return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", "recent_messages_for_requester does not accept room; omit it or use recent_messages_for_room."), nil
		}
	case "recent_messages_for_room":
		if input.Room == nil || strings.TrimSpace(*input.Room) == "" {
			return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", "recent_messages_for_room requires a nonblank room."), nil
		}
	}
	trip := ""
	if value := agent.Trip(); value != nil {
		trip = *value
	}
	data, err := t.Repository.ExecuteAgentQuery(ctx, strings.TrimSpace(input.Query), raw, agent.Room(), trip)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "database query failed"), nil
	}
	return contract.SuccessResult("", t.Name(), string(data)), nil
}

var _ Tool = DatabaseQuery{}
