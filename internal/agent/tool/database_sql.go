package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"zenbot/internal/agent/api"
	agentsql "zenbot/internal/agent/sql"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/config"
	"zenbot/internal/repository"
)

const databaseSQLName = "database_sql"

// DatabaseSQL is the capability-gated, schema-prerequisite dynamic SQL tool.
type DatabaseSQL struct {
	Schema     repository.AgentSchemaRepository
	Repository repository.AgentSQLRepository
	Config     config.AgentSqlConfig
}

func (t DatabaseSQL) Name() string { return databaseSQLName }
func (t DatabaseSQL) Descriptor(api.Context) (contract.Descriptor, error) {
	p := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"sql":{"type":"string","minLength":1}},"required":["sql"]}`)
	return contract.NewDescriptor(databaseSQLName, "Run bounded read-only SQL", "Run validated read-only SQL after inspecting the schema.", "database", contract.AccessAdmin, contract.ReadOnly, contract.ModelData, p, []string{string(api.DynamicSQL)}, []string{databaseSchemaName}, true, time.Duration(t.Config.TimeoutMillis)*time.Millisecond, json.RawMessage(`{"type":"any"}`), []string{"database"}, nil, []string{"Do not use without DYNAMIC_SQL or database_schema."})
}
func (t DatabaseSQL) Execute(ctx context.Context, agent api.Context, raw json.RawMessage) (contract.Result, error) {
	if !t.Config.Enabled || !agent.HasCapability(api.DynamicSQL) {
		return contract.ErrorResult("", t.Name(), "TOOL_NOT_AUTHORIZED", "tool is unavailable"), nil
	}
	if t.Schema == nil || t.Repository == nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "SQL execution failed"), nil
	}
	var in struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return contract.Result{}, fmt.Errorf("invalid SQL arguments")
	}
	schema, err := t.Schema.DescribeAgentSchema(ctx)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "TOOL_EXECUTION_FAILED", "SQL execution failed"), nil
	}
	validated, err := agentsql.NewSQLiteSelectPolicy(t.Config.MaxSQLChars).Validate(in.SQL, schema)
	if err != nil {
		code := "EXECUTION_FAILED"
		if e, ok := err.(*agentsql.AgentSqlPolicyError); ok {
			code = string(e.CodeValue())
		}
		return contract.ErrorResult("", t.Name(), code, "SQL was rejected"), nil
	}
	out, err := t.Repository.ExecuteAgentSQL(ctx, validated.SQL, t.Config.MaxRows, t.Config.MaxColumns, t.Config.MaxCellChars, t.Config.MaxResultChars)
	if err != nil {
		return contract.ErrorResult("", t.Name(), "EXECUTION_FAILED", "SQL execution failed"), nil
	}
	return contract.SuccessResult("", t.Name(), string(out)), nil
}

var _ Tool = DatabaseSQL{}
