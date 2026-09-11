package command

import (
	"context"
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/testutil/sqlitefixture"
)

// Tracer 7 only: the SQL shell is intentionally non-operational until tracer 8.
func TestFactoryWiresRawSQLiteSQLCapabilityAndRegistersConcreteSQLShell(t *testing.T) {
	withoutCapability, err := factory.NewEngineWithOptions(model.MASTER, &config.Config{
		WebsocketUrl: "ws://127.0.0.1:1",
		Channel:      "test",
	}, nil, factory.EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterUserUtilities(withoutCapability); err != nil {
		t.Fatal(err)
	}
	if _, registered := (*withoutCapability.GetEnabledCommands())["sql"]; registered {
		t.Fatal("sql registered without raw SQL capability")
	}

	database := sqlitefixture.Open(t, "raw_sql_command")
	engine, err := factory.NewEngineWithOptions(model.MASTER, &config.Config{
		WebsocketUrl: "ws://127.0.0.1:1",
		Channel:      "test",
	}, database, factory.EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if engine.Services == nil || engine.Services.SQLCommand == nil {
		t.Fatalf("raw SQL capability was not wired: services=%+v", engine.Services)
	}
	table, err := engine.Services.SQLCommand.Query(context.Background(), "SELECT 7 AS number, NULL AS absent")
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Columns) != 2 || table.Columns[0] != "number" || table.Columns[1] != "absent" || len(table.Rows) != 1 || len(table.Rows[0]) != 2 || table.Rows[0][0] != "7" || table.Rows[0][1] != "null" {
		t.Fatalf("raw SQLite query result=%+v", table)
	}

	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	metadata, registered := (*engine.GetEnabledCommands())["sql"]
	if !registered {
		t.Fatal("sql was not registered with raw SQL capability")
	}
	if _, ok := metadata.Command(&model.ChatMessage{}).(*legacyAdapter).def.New(engine, &model.ChatMessage{}).(*sqlCommand); !ok {
		t.Fatal("sql registration did not select concrete SQL shell")
	}
}
