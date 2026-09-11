package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

type recordingRawSQLQuery struct {
	queries []string
}

func (q *recordingRawSQLQuery) Query(_ context.Context, rawSQL string) (service.SQLTable, error) {
	q.queries = append(q.queries, rawSQL)
	return service.SQLTable{}, nil
}

type cancelDuringSQLQuery struct {
	calls  int
	cancel context.CancelFunc
}

func (q *cancelDuringSQLQuery) Query(_ context.Context, _ string) (service.SQLTable, error) {
	q.calls++
	q.cancel()
	return service.SQLTable{}, context.Canceled
}

type sendFailureSQLCommandEngine struct {
	*commandEngineStub
	sendAttempts int
	sendErr      error
}

func (e *sendFailureSQLCommandEngine) SendChatMessage(string, string, bool) (string, error) {
	e.sendAttempts++
	return "", e.sendErr
}

func TestSQLCommandReturnsSendFailureWithoutRequery(t *testing.T) {
	query := &recordingRawSQLQuery{}
	engine := &sendFailureSQLCommandEngine{
		commandEngineStub: &commandEngineStub{
			bundle: &service.Bundle{SQLCommand: query},
			users:  map[string]*model.User{},
		},
		sendErr: errors.New("send failed"),
	}

	status, err := newCommand("sql", []string{"sql"}, model.ADMIN, engine, &model.ChatMessage{Name: "alice", Text: "!sql SELECT 1"}).Execute(context.Background())

	if status != model.FAILED || !errors.Is(err, engine.sendErr) {
		t.Fatalf("status=%v err=%v, want failed delivery", status, err)
	}
	if len(query.queries) != 1 || query.queries[0] != "SELECT 1" {
		t.Fatalf("queries=%v, want one raw query", query.queries)
	}
	if engine.sendAttempts != 1 || len(engine.chats) != 0 || len(engine.raws) != 0 {
		t.Fatalf("send attempts=%d chats=%v raws=%v, want one failed attempt and no retry", engine.sendAttempts, engine.chats, engine.raws)
	}
}

func TestSQLCommandParsesRawSourcePayloadOnly(t *testing.T) {
	query := &recordingRawSQLQuery{}
	engine := &commandEngineStub{
		bundle: &service.Bundle{SQLCommand: query},
		users:  map[string]*model.User{},
	}

	for _, tc := range []struct {
		name    string
		text    string
		wantSQL string
		wantErr bool
	}{
		{name: "exact raw payload", text: "!sql SELECT 1", wantSQL: "SELECT 1"},
		{name: "literal backslash is preserved", text: `!sql SELECT\n1`, wantSQL: `SELECT\n1`},
		{name: "uppercase command token", text: "!SQL SELECT 1", wantSQL: "SELECT 1"},
		{name: "missing separator", text: "!sql", wantErr: true},
		{name: "empty payload", text: "!sql ", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			beforeQueries := len(query.queries)
			beforeChats := len(engine.chats)
			status, err := newCommand("sql", []string{"sql"}, model.ADMIN, engine, &model.ChatMessage{Name: "alice", Text: tc.text}).Execute(context.Background())

			if tc.wantErr {
				if status != model.FAILED || err == nil {
					t.Fatalf("status=%v err=%v, want internal failure", status, err)
				}
				if len(query.queries) != beforeQueries || len(engine.chats) != beforeChats {
					t.Fatalf("malformed payload queried or replied: queries=%v chats=%v", query.queries[beforeQueries:], engine.chats[beforeChats:])
				}
				return
			}

			if status != model.SUCCESSFUL || err != nil {
				t.Fatalf("status=%v err=%v", status, err)
			}
			if len(query.queries) != beforeQueries+1 || query.queries[beforeQueries] != tc.wantSQL {
				t.Fatalf("queries=%v, want appended %q", query.queries, tc.wantSQL)
			}
		})
	}
}

func TestUserChatListenerSQLRepliesWithSaturnASCIIH2Goldens(t *testing.T) {
	database := h2fixture.Open(t, "sql-reply-ascii")
	engine := &commandEngineStub{
		bundle: &service.Bundle{SQLCommand: &service.RawSQLService{DB: database.SQLDB()}},
		users:  map[string]*model.User{"alice": {Name: "alice", Trip: "admin"}},
	}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "row includes column names and null",
			query: "SELECT 7 AS \"ID\", CAST(NULL AS VARCHAR) AS \"NOTE\"",
			want:  "\\n```Text\\n\\n\\n+------+--------+\\n|  id  |  note  |\\n+------+--------+\\n|  7   |  null  |\\n+------+--------+\\n\\n\\n ```",
		},
		{
			name:  "zero rows retains table",
			query: "SELECT 7 AS \"ID\", CAST(NULL AS VARCHAR) AS \"NOTE\" WHERE FALSE",
			want:  "\\n```Text\\n\\n\\n+------+--------+\\n|  id  |  note  |\\n+------+--------+\\n+------+--------+\\n\\n\\n ```",
		},
	} {
		for _, whisper := range []bool{false, true} {
			t.Run(tc.name+"/whisper="+boolString(whisper), func(t *testing.T) {
				engine.chats = nil
				message, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "admin", Text: "!sql " + tc.query, Type: whisperType(whisper)})
				if err != nil {
					t.Fatal(err)
				}
				listener.NewUserChatListener(engine).Notify(string(message))

				want := "alice|Result: \\n" + tc.want + "|" + boolString(whisper)
				if len(engine.chats) != 1 || engine.chats[0] != want {
					t.Fatalf("replies=%q, want one exact reply %q", engine.chats, want)
				}
			})
		}
	}
}

func TestUserChatListenerSQLDoesNotPublishRawH2DriverError(t *testing.T) {
	database := h2fixture.Open(t, "sql-driver-error")
	query := "SELEC 1"
	if _, err := (&service.RawSQLService{DB: database.SQLDB()}).Query(context.Background(), query); err == nil {
		t.Fatal("malformed query unexpectedly succeeded")
	} else {
		engine := &commandEngineStub{
			bundle: &service.Bundle{SQLCommand: &service.RawSQLService{DB: database.SQLDB()}},
			users:  map[string]*model.User{"alice": {Name: "alice", Trip: "admin"}},
		}
		if err := RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}

		message, marshalErr := json.Marshal(model.ChatMessage{Name: "alice", Trip: "admin", Text: "!sql " + query, Type: "whisper"})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		listener.NewUserChatListener(engine).Notify(string(message))

		if len(engine.chats) != 0 {
			t.Fatalf("query error was published as room data: %q", engine.chats)
		}
	}
}

func TestSQLCommandPreCancelledContextDoesNotQueryOrReply(t *testing.T) {
	query := &recordingRawSQLQuery{}
	engine := &commandEngineStub{
		bundle: &service.Bundle{SQLCommand: query},
		users:  map[string]*model.User{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := newCommand("sql", []string{"sql"}, model.ADMIN, engine, &model.ChatMessage{Name: "alice", Text: "!sql SELECT 1"}).Execute(ctx)

	if status != model.FAILED || !errors.Is(err, context.Canceled) {
		t.Fatalf("status=%v err=%v, want FAILED context.Canceled", status, err)
	}
	if len(query.queries) != 0 || len(engine.chats) != 0 {
		t.Fatalf("pre-cancelled command queried or replied: queries=%v chats=%v", query.queries, engine.chats)
	}
}

func TestSQLCommandCancellationDuringQueryDoesNotReply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	query := &cancelDuringSQLQuery{cancel: cancel}
	engine := &commandEngineStub{
		bundle: &service.Bundle{SQLCommand: query},
		users:  map[string]*model.User{},
	}

	status, err := newCommand("sql", []string{"sql"}, model.ADMIN, engine, &model.ChatMessage{Name: "alice", Text: "!sql SELECT 1"}).Execute(ctx)

	if status != model.FAILED || !errors.Is(err, context.Canceled) {
		t.Fatalf("status=%v err=%v, want FAILED context.Canceled", status, err)
	}
	if query.calls != 1 || len(engine.chats) != 0 {
		t.Fatalf("calls=%d chats=%v, want one cancelled query and no reply", query.calls, engine.chats)
	}
}

func TestUserChatListenerSQLRepliesWithSaturnUnicodeControlH2Golden(t *testing.T) {
	database := h2fixture.Open(t, "sql-reply-unicode")
	engine := &commandEngineStub{
		bundle: &service.Bundle{SQLCommand: &service.RawSQLService{DB: database.SQLDB()}},
		users:  map[string]*model.User{"alice": {Name: "alice", Trip: "admin"}},
	}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	query := "SELECT STRINGDECODE('\\uD83D\\uDE00') AS emoji, 'quote' || CHAR(34) || ' slash' || CHAR(92) || ' tab' || CHAR(9) || 'ctrl' || CHAR(1) || 'cr' || CHAR(13) || 'nl' || CHAR(10) AS text"
	wantTable := `\n` + "```" + `Text\n\n\n+----------+----------------------------------+\n|  emoji   |               text               |\n+----------+----------------------------------+\n|    \uD83D\uDE00    |  quote\" slash\\ tab` + string('\\') + `tctrl\u0001cr\rnl\n   |\n+----------+----------------------------------+\n\n\n ` + "```"

	for _, whisper := range []bool{false, true} {
		t.Run("whisper="+boolString(whisper), func(t *testing.T) {
			engine.chats = nil
			message, err := json.Marshal(model.ChatMessage{Name: "alice", Trip: "admin", Text: "!sql " + query, Type: whisperType(whisper)})
			if err != nil {
				t.Fatal(err)
			}
			listener.NewUserChatListener(engine).Notify(string(message))

			want := "alice|Result: \\n" + wantTable + "|" + boolString(whisper)
			if len(engine.chats) != 1 || engine.chats[0] != want {
				t.Fatalf("replies=%q, want one exact reply %q", engine.chats, want)
			}
		})
	}
}
