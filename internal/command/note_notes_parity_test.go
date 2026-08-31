package command

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository/h2"
	"zenbot/internal/service"
)

func openNotesParityEngine(t *testing.T) *commandEngineStub {
	t.Helper()
	dir := t.TempDir()
	d, err := h2.Open(context.Background(), h2.Config{
		BaseDir: dir, DatabaseStem: filepath.Join(dir, "db"),
		H2Jar: "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar",
		// Use a dedicated port: command integration tests already own 55437.
		Port: 55438, StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return &commandEngineStub{
		users:  map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}},
		bundle: &service.Bundle{Notes: &service.NoteService{DB: d.DB}},
	}
}

func executeNotesParityCommand(t *testing.T, e *commandEngineStub, alias, trip, text string) model.Status {
	t.Helper()
	d, ok := commandDefinitionFor(alias)
	if !ok {
		t.Fatalf("missing definition for %q", alias)
	}
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: trip, Text: "!" + alias + text}).Execute(context.Background())
	if err != nil {
		t.Fatalf("%s error: %v", alias, err)
	}
	return status
}

func TestNoteAndSaveParityAliasesAndTripBoundary(t *testing.T) {
	e := openNotesParityEngine(t)
	for _, alias := range []string{"note", "save"} {
		e.chats = nil
		if got := executeNotesParityCommand(t, e, alias, "trip", " hello world"); got != model.SUCCESSFUL {
			t.Fatalf("%s status=%s", alias, got)
		}
		if len(e.chats) != 1 || e.chats[0] != "alice|note successfully saved!|false" {
			t.Fatalf("%s chats=%q", alias, e.chats)
		}
	}
	listed, err := e.bundle.Notes.List("trip")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0] != "hello world" || listed[1] != "hello world" {
		t.Fatalf("saved notes=%q", listed)
	}

	e.chats = nil
	if got := executeNotesParityCommand(t, e, "note", "", " no trip"); got != model.SUCCESSFUL {
		t.Fatalf("no-trip status=%s", got)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice|note successfully saved!|false" {
		t.Fatalf("no-trip chats=%q", e.chats)
	}
	listed, err = e.bundle.Notes.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("no-trip unexpectedly saved=%q", listed)
	}

	e.chats = nil
	if got := executeNotesParityCommand(t, e, "save", "trip", ""); got != model.FAILED {
		t.Fatalf("no-argument status=%s", got)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice|Example: !note Jedi am I?!|false" {
		t.Fatalf("no-argument chats=%q", e.chats)
	}
}

func TestNotesParityListPurgeClearAndInvalidArguments(t *testing.T) {
	e := openNotesParityEngine(t)
	if err := e.bundle.Notes.Save("trip", "quote \"line\nbackslash\\"); err != nil {
		t.Fatal(err)
	}

	e.chats = nil
	if got := executeNotesParityCommand(t, e, "notes", "", ""); got != model.FAILED {
		t.Fatalf("no-trip status=%s", got)
	}
	if len(e.chats) != 1 || e.chats[0] != "alice|\\n Set your trip first. Example: !notes|false" {
		t.Fatalf("no-trip chats=%q", e.chats)
	}

	e.chats = nil
	if got := executeNotesParityCommand(t, e, "notes", "trip", ""); got != model.SUCCESSFUL {
		t.Fatalf("list status=%s", got)
	}
	wantList := "alice|'s notes: \\n ```Text \\n[quote \\\"line\\nbackslash\\\\]\\n```|false"
	if len(e.chats) != 1 || e.chats[0] != wantList {
		t.Fatalf("list chats=%q want=%q", e.chats, wantList)
	}

	for _, alias := range []string{"purge", "clear"} {
		e.chats = nil
		if got := executeNotesParityCommand(t, e, "notes", "trip", " "+alias); got != model.SUCCESSFUL {
			t.Fatalf("%s status=%s", alias, got)
		}
		if len(e.chats) != 1 || e.chats[0] != "alice|'s notes has been deleted|false" {
			t.Fatalf("%s chats=%q", alias, e.chats)
		}
		if err := e.bundle.Notes.Save("trip", "again"); err != nil {
			t.Fatal(err)
		}
	}

	for _, arg := range []string{"PURGE", "anything"} {
		e.chats = nil
		if got := executeNotesParityCommand(t, e, "notes", "trip", " "+arg); got != model.FAILED {
			t.Fatalf("%s status=%s", arg, got)
		}
		if len(e.chats) != 0 {
			t.Fatalf("%s chats=%q", arg, e.chats)
		}
		listed, err := e.bundle.Notes.List("trip")
		if err != nil || len(listed) != 1 || listed[0] != "again" {
			t.Fatalf("%s changed notes=%q err=%v", arg, listed, err)
		}
	}
}

var _ common.Engine = (*commandEngineStub)(nil)
