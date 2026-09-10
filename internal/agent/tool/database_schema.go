package tool

import (
	"context"
	"encoding/json"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/repository"
)

const databaseSchemaName = "database_schema"

// DatabaseSchema exposes current metadata only to DYNAMIC_SQL-capable callers.
type DatabaseSchema struct {
	Repository repository.AgentSchemaRepository
	Enabled    bool
}

func (t DatabaseSchema) Name() string { return databaseSchemaName }
func (t DatabaseSchema) Descriptor(api.Context) (contract.Descriptor, error) {
	return contract.NewDescriptor(databaseSchemaName, "Inspect database schema", "Inspect the current application schema before bounded dynamic SQL.", "database", contract.AccessAdmin, contract.ReadOnly, contract.ModelData, json.RawMessage(`{"type":"object","additionalProperties":false}`), []string{string(api.DynamicSQL)}, nil, true, 2*time.Second, json.RawMessage(`{"type":"object"}`), []string{"database_schema"}, nil, []string{"Do not use without the DYNAMIC_SQL capability."})
}
func (t DatabaseSchema) Execute(ctx context.Context, agent api.Context, _ json.RawMessage) (contract.Result, error) {
	if !t.Enabled || !agent.HasCapability(api.DynamicSQL) {
		return contract.ErrorResult("", t.Name(), "TOOL_NOT_AUTHORIZED", "tool is unavailable"), nil
	}
	if t.Repository == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "schema inspection failed"), nil
	}
	s, err := t.Repository.DescribeAgentSchema(ctx)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "schema inspection failed"), nil
	}
	return contract.SuccessResult("", t.Name(), s), nil
}

var _ Tool = DatabaseSchema{}
