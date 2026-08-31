package command

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zenbot/internal/model"
	"zenbot/internal/repository/h2"
	"zenbot/internal/service"
)

func openMailGroupCParityEngine(t *testing.T) (*commandEngineStub, *sql.DB) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	jar := os.Getenv("H2_JAR")
	if jar == "" {
		jar = "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"
	}
	dir := t.TempDir()
	d, err := h2.Open(context.Background(), h2.Config{
		BaseDir: dir, DatabaseStem: filepath.Join(dir, "mail-group-c-command.db"),
		H2Jar: jar, Port: port, StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	return &commandEngineStub{
		users:  map[string]*model.User{"alice": {Name: "alice", Trip: "origin"}},
		bundle: &service.Bundle{Mail: &service.MailService{DB: d.DB, GroupB: d}},
	}, d.DB
}

func executeMailGroupCCommand(t *testing.T, e *commandEngineStub, alias, text string) model.Status {
	t.Helper()
	d, ok := commandDefinitionFor(alias)
	if !ok {
		t.Fatalf("missing definition for %q", alias)
	}
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "origin", Text: "!" + alias + text, IsWhisper: true}).Execute(context.Background())
	if err != nil {
		t.Fatalf("%s error: %v", alias, err)
	}
	return status
}

func seedMailGroupCCommandRecipient(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range []string{
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-a',1)",
		"INSERT INTO names(name,created_on) VALUES('Merc',1)",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-a' AND n.name='Merc'",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMailGroupCParityAliasesUsageAndBlankReceiver(t *testing.T) {
	e, _ := openMailGroupCParityEngine(t)
	for _, alias := range []string{"mail", "msg", "send"} {
		e.chats = nil
		if got := executeMailGroupCCommand(t, e, alias, ""); got != model.FAILED {
			t.Fatalf("%s no-argument status=%s", alias, got)
		}
		if len(e.chats) != 1 || e.chats[0] != "alice|Example: -mail merc message|true" {
			t.Fatalf("%s no-argument chats=%q", alias, e.chats)
		}

		e.chats = nil
		if got := executeMailGroupCCommand(t, e, alias, " @ message"); got != model.SUCCESSFUL {
			t.Fatalf("%s blank-receiver status=%s", alias, got)
		}
		if len(e.chats) != 1 || e.chats[0] != "alice|Receiver cannot be blank.|true" {
			t.Fatalf("%s blank-receiver chats=%q", alias, e.chats)
		}
	}
}

func TestMailGroupCParityUnknownReceiverDirectory(t *testing.T) {
	e, db := openMailGroupCParityEngine(t)
	seedMailGroupCCommandRecipient(t, db)

	if got := executeMailGroupCCommand(t, e, "mail", " unknown message"); got != model.SUCCESSFUL {
		t.Fatalf("unknown-receiver status=%s", got)
	}
	want := "alice|User you specified is not registered. Please use a name from provided list to send a message to respective trip. \\\\nMerc trip-a\\n|true"
	if len(e.chats) != 1 || e.chats[0] != want {
		t.Fatalf("unknown-receiver chats=%q want=%q", e.chats, want)
	}
}

func TestMailGroupCParitySuccessAcknowledgementAndQueuedPayload(t *testing.T) {
	e, db := openMailGroupCParityEngine(t)
	seedMailGroupCCommandRecipient(t, db)

	if got := executeMailGroupCCommand(t, e, "send", " @mErC quote \"x\" line"); got != model.SUCCESSFUL {
		t.Fatalf("success status=%s", got)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice|trips: trip-a will receive your message as soon they chat|true" {
		t.Fatalf("success chats=%q", e.chats)
	}

	var owner, receiver, message, status, whisper string
	if err := db.QueryRow("SELECT owner,receiver,message,status,is_whisper FROM mail").Scan(&owner, &receiver, &message, &status, &whisper); err != nil {
		t.Fatal(err)
	}
	if owner != "alice#origin" || receiver != "trip-a" || message != "quote \\\"x\\\" line " || status != "PENDING" || whisper != "true" {
		t.Fatalf("queued mail owner=%q receiver=%q message=%q status=%q whisper=%q", owner, receiver, message, status, whisper)
	}
}
