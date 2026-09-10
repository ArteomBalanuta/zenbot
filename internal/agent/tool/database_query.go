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
	parameters := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string","enum":["message_count","registered_user_count","recent_messages_for_requester","recent_messages_for_room"]},"limit":{"type":"integer","minimum":1,"maximum":60},"room":{"type":"string","minLength":1,"maxLength":100},"trip":{"type":"string","minLength":1,"maxLength":100},"nick":{"type":"string","minLength":1,"maxLength":100}},"required":["query"]}`)
	return contract.NewDescriptor(databaseQueryName, "Approved database query", "Run one fixed read-only database query.", "database", contract.AccessUser, contract.ReadOnly, contract.ModelData, parameters, nil, nil, true, 2*time.Second, json.RawMessage(`{"type":"any"}`), []string{"database"}, nil, []string{"Do not use for generated SQL or private data."}, contract.WithPrimaryIntent("approved_database_query"))
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
		return contract.ErrorResult("", t.Name(), "INVALID_ARGUMENTS", "database query arguments were rejected"), nil
	}
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Query) == "" {
		return contract.Result{}, fmt.Errorf("query is required")
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
