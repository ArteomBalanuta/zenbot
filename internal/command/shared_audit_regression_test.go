package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

// Catch command tokenization altering the original request before the LLM
// receives it. Only the external submission boundary is replaced by a recorder.
func TestSharedAuditDirectLPreservesPromptBody(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}}
	submitter := &recordingDirectAgentSubmitter{}
	definition, ok := directLDefinition(submitter)
	if !ok {
		t.Fatal("direct command unavailable")
	}
	engine.RegisterCommand(&legacyAdapter{engine: engine, def: definition})
	want := "Count both rooms.\nThen output exactly:\n```text\na  b\n\tcafé\n```"
	raw, err := json.Marshal(model.ChatMessage{Name: "alice", Text: "!l " + want})
	if err != nil {
		t.Fatal(err)
	}
	listener.NewUserChatListener(engine).NotifyContext(context.Background(), string(raw))
	if submitter.calls != 1 || submitter.prompt != want {
		t.Fatalf("prompt changed before model submission: calls=%d got=%q want=%q", submitter.calls, submitter.prompt, want)
	}
}

func TestSharedAuditSQLPreservesQueryBody(t *testing.T) {
	for _, tc := range []struct{ name, command, query string }{
		{"embedded command text", "!sql", "SELECT 'sql inner' AS payload"},
		{"literal backslash", "!sql", `SELECT '\n' AS payload`},
		{"resolved uppercase", "!SQL", "SELECT 1"},
		{"resolved anagram", "!qsl", "SELECT 1"},
		{"multiline whitespace", "!sql", "SELECT  'a  b'\n AS payload"},
		{"trailing query padding", "!sql", "SELECT 'café' AS payload\t  "},
		{"leading query padding", "!sql", "  SELECT 'café' AS payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := &recordingRawSQLQuery{}
			engine := &commandEngineStub{bundle: &service.Bundle{SQLCommand: query}, users: map[string]*model.User{"alice": {Name: "alice", Trip: "admin"}}}
			if err := RegisterUserUtilities(engine); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "admin", Text: tc.command + " " + tc.query})
			if err != nil {
				t.Fatal(err)
			}
			listener.NewUserChatListener(engine).NotifyContext(context.Background(), string(raw))
			if len(query.queries) != 1 || query.queries[0] != tc.query {
				t.Fatalf("dispatched SQL changed: got=%q want=%q", query.queries, tc.query)
			}
		})
	}
}

type sharedAuditFailedQuery struct{ err error }

func (q sharedAuditFailedQuery) Query(context.Context, string) (service.SQLTable, error) {
	return service.SQLTable{}, q.err
}

func TestSharedAuditSQLFailureIsNotSuccessfulOutput(t *testing.T) {
	driverErr := errors.New("internal-driver-stack-secret")
	engine := &commandEngineStub{bundle: &service.Bundle{SQLCommand: sharedAuditFailedQuery{driverErr}}}
	definition, ok := commandDefinitionFor("sql")
	if !ok {
		t.Fatal("sql unavailable")
	}
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!sql SELECT 1"}).Execute(context.Background())
	if status != model.FAILED || err == nil {
		t.Errorf("query failure became success: status=%v err=%v", status, err)
	}
	if strings.Contains(strings.Join(engine.chats, "\n"), driverErr.Error()) {
		t.Error("raw storage diagnostic was emitted as room output")
	}
}

func TestSharedAuditSQLDeliveryFailureIsReturned(t *testing.T) {
	query := &recordingRawSQLQuery{}
	sendErr := errors.New("send failed")
	engine := &sendFailureSQLCommandEngine{commandEngineStub: &commandEngineStub{bundle: &service.Bundle{SQLCommand: query}}, sendErr: sendErr}
	definition, _ := commandDefinitionFor("sql")
	status, err := definition.New(engine, &model.ChatMessage{Name: "alice", Text: "!sql SELECT 1"}).Execute(context.Background())
	if status != model.FAILED || !errors.Is(err, sendErr) {
		t.Fatalf("failed delivery was hidden: status=%v err=%v", status, err)
	}
	if len(query.queries) != 1 || engine.sendAttempts != 1 {
		t.Fatalf("query or delivery replayed: queries=%v sends=%d", query.queries, engine.sendAttempts)
	}
}

func TestSharedAuditCloneKeepsCanonicalMemoryBehavior(t *testing.T) {
	definition, ok := commandDefinitionFor("memory")
	if !ok {
		t.Fatal("memory unavailable")
	}
	engine := &commandEngineStub{}
	prototype := definition.New(engine, &model.ChatMessage{Name: "unused", Text: "!memory"})
	cloned := prototype.NewInstance(engine, &model.ChatMessage{Name: "alice", Text: "!mem"})
	status, err := cloned.Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || len(engine.chats) != 1 {
		t.Fatalf("clone lost executable canonical handler: status=%v err=%v replies=%v", status, err, engine.chats)
	}
	if !strings.HasPrefix(engine.chats[0], "alice|") || !strings.Contains(engine.chats[0], "MB") {
		t.Fatalf("clone did not deliver runtime measurements to the new caller: %q", engine.chats)
	}
}
