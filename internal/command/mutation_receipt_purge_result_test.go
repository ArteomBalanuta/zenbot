package command

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"zenbot/internal/agent/api"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/agent/tool/execution"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/repository/h2"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

// Execute the real H2 deletion, changing only the affected-row response.
type purgeResultConnector struct {
	driver.Connector
	countErr   error
	executions *atomic.Int32
}
type purgeResultConn struct {
	driver.Conn
	countErr   error
	executions *atomic.Int32
}
type purgeResult struct {
	driver.Result
	countErr error
}

func (c purgeResultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &purgeResultConn{Conn: conn, countErr: c.countErr, executions: c.executions}, nil
}
func (c *purgeResultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.executions.Add(1)
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return purgeResult{Result: result, countErr: c.countErr}, nil
}
func (r purgeResult) RowsAffected() (int64, error) {
	if r.countErr != nil {
		return 7, fmt.Errorf("affected-row metadata: %w", r.countErr)
	}
	return r.Result.RowsAffected()
}
func purgeResultDatabase(t *testing.T, db *h2.Database, countErr error) (*sql.DB, *atomic.Int32) {
	t.Helper()
	cfg, err := pgx.ParseConfig(fmt.Sprintf("postgres://sa@%s/db?sslmode=disable", db.Server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	cfg.User = ""
	cfg.RuntimeParams = map[string]string{}
	calls := &atomic.Int32{}
	wrapped := sql.OpenDB(purgeResultConnector{Connector: stdlib.GetConnector(*cfg), countErr: countErr, executions: calls})
	t.Cleanup(func() {
		if err := wrapped.Close(); err != nil {
			t.Error(err)
		}
	})
	return wrapped, calls
}

func TestMutationReceiptPurgeAffectedRowsResult(t *testing.T) {
	lost := errors.New("private affected-row failure")
	for _, tc := range []struct {
		name        string
		seed        bool
		countErr    error
		wantReceipt int
	}{
		{"failed count after committed deletion", true, lost, 0},
		{"zero rows", false, nil, 0},
		{"positive rows", true, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := h2fixture.Open(t, "db")
			if tc.seed {
				if _, err := db.DB.Exec("INSERT INTO notes(trip,note,created_on) VALUES('Trip','private',1)"); err != nil {
					t.Fatal(err)
				}
			}
			wrapped, calls := purgeResultDatabase(t, db, tc.countErr)
			ctx, receipt := common.WithMutationRecorder(context.Background())
			err := (&service.NoteService{DB: wrapped}).Clear(ctx, "Trip")
			if !errors.Is(err, tc.countErr) || receipt.Count() != tc.wantReceipt || calls.Load() != 1 {
				t.Errorf("err=%v receipts=%d executions=%d", err, receipt.Count(), calls.Load())
			}
			var remaining int
			if err := db.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE trip='Trip'").Scan(&remaining); err != nil || remaining != 0 {
				t.Fatalf("actual deletion remaining=%d err=%v", remaining, err)
			}
		})
	}
}

func TestMutationReceiptPurgeCountFailureIsUnknownWithoutAckOrReplay(t *testing.T) {
	db := h2fixture.Open(t, "db")
	if _, err := db.DB.Exec("INSERT INTO notes(trip,note,created_on) VALUES('Trip','private',1)"); err != nil {
		t.Fatal(err)
	}
	wrapped, calls := purgeResultDatabase(t, db, errors.New("private affected-row failure"))
	engine := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: &service.Bundle{Notes: &service.NoteService{DB: wrapped}}}}
	definition, _ := commandcatalog.AgentEntry("notes")
	purge := tool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(engine)}
	executor := &execution.Executor{Registry: tool.NewRegistry([]tool.Tool{purge}, []string{purge.Name()}), Ledger: execution.NewLedger(nil, 3)}
	caller, _ := api.NewContext("room", "caller", "Trip", "", false, []string{})
	args := json.RawMessage(`{"operation":"purge"}`)
	result := executor.Execute(context.Background(), caller, execution.Call{ID: "first", Name: purge.Name(), Arguments: args})
	if result.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || result.EffectState != contract.EffectUnknown || result.EffectsCommitted || result.ActionCount != 0 || result.DeliveryCount != 0 || engine.sends != 0 {
		t.Errorf("result=%+v sends=%d", result, engine.sends)
	}
	if strings.Contains(result.Content, "private") {
		t.Errorf("private storage failure exposed: %+v", result)
	}
	repeat := executor.Execute(context.Background(), caller, execution.Call{ID: "repeat", Name: purge.Name(), Arguments: args})
	if repeat.ErrorCode != "ACTION_NOT_EXECUTED" || calls.Load() != 1 {
		t.Errorf("repeat=%+v executions=%d", repeat, calls.Load())
	}
	var remaining int
	if err := db.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE trip='Trip'").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("actual deletion remaining=%d err=%v", remaining, err)
	}
}
