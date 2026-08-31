package tool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agenttool "zenbot/internal/agent/tool"
)

type roomDirectoryStub struct {
	snapshots map[string]agenttool.RoomUserSnapshot
	calls     int
	lastRoom  string
}

func (d *roomDirectoryStub) FindRoomUsers(room string) (agenttool.RoomUserSnapshot, bool) {
	d.calls++
	d.lastRoom = room
	snapshot, ok := d.snapshots[strings.ToLower(room)]
	return snapshot, ok
}

func TestRoomUsersDescriptorAndBoundedDeterministicResult(t *testing.T) {
	users := make([]string, 0, 203)
	users = append(users, "", "bob", "Alice", "alice", `quote " name`)
	for i := 0; i < 198; i++ {
		users = append(users, "user"+string(rune('a'+i%26)))
	}
	directory := &roomDirectoryStub{snapshots: map[string]agenttool.RoomUserSnapshot{"other": {Room: "Other", Users: users}}}
	roomUsers := agenttool.RoomUsers{Directory: directory}
	d, err := roomUsers.Descriptor(historyContext(t, "trusted"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "room_users" || !d.IsReadOnly() || d.Timeout().Seconds() != 2 {
		t.Fatalf("descriptor=%#v", d)
	}
	var schema struct {
		Additional bool                      `json:"additionalProperties"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(d.Parameters(), &schema); err != nil || schema.Additional || len(schema.Required) != 0 || schema.Properties["room"]["maxLength"] != float64(100) {
		t.Fatalf("schema=%s err=%v", d.Parameters(), err)
	}
	r, err := roomUsers.Execute(context.Background(), historyContext(t, "trusted"), json.RawMessage(`{"room":" OTHER "}`))
	if err != nil || r.IsError {
		t.Fatalf("result=%#v err=%v", r, err)
	}
	var output struct {
		Room          string   `json:"room"`
		Users         []string `json:"users"`
		Count         int      `json:"count"`
		ReturnedCount int      `json:"returnedCount"`
		Truncated     bool     `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(r.Content), &output); err != nil {
		t.Fatal(err)
	}
	if directory.lastRoom != "OTHER" || output.Room != "Other" || output.Count != 202 || output.ReturnedCount != 200 || !output.Truncated || output.Users[0] != "Alice" || output.Users[1] != "alice" || strings.Contains(r.Content, "trip") {
		t.Fatalf("output=%s directory=%#v", r.Content, directory)
	}
}

func TestRoomUsersUsesTrustedDefaultAndRejectsInvalidWithoutLookup(t *testing.T) {
	directory := &roomDirectoryStub{snapshots: map[string]agenttool.RoomUserSnapshot{"trusted": {Room: "Trusted", Users: []string{"alice"}}}}
	roomUsers := agenttool.RoomUsers{Directory: directory}
	result, err := roomUsers.Execute(context.Background(), historyContext(t, "trusted"), json.RawMessage(`{}`))
	if err != nil || result.IsError || directory.lastRoom != "trusted" {
		t.Fatalf("default result=%#v err=%v directory=%#v", result, err, directory)
	}
	for _, args := range []string{`{"room":" "}`, `{"room":123}`, `{"room":"` + strings.Repeat("a", 101) + `"}`} {
		directory.calls = 0
		if _, err := roomUsers.Execute(context.Background(), historyContext(t, "trusted"), json.RawMessage(args)); err == nil || directory.calls != 0 {
			t.Fatalf("args=%s err=%v calls=%d", args, err, directory.calls)
		}
	}
	unavailable, err := (agenttool.RoomUsers{}).Execute(context.Background(), historyContext(t, "trusted"), json.RawMessage(`{}`))
	if err != nil || !unavailable.IsError || unavailable.ErrorCode != "TOOL_EXECUTION_FAILED" || strings.Contains(unavailable.Content, "directory") {
		t.Fatalf("unavailable=%#v err=%v", unavailable, err)
	}
}

func TestRoomUsersSortsWithUnicodeCaseFoldingAndRawTieBreak(t *testing.T) {
	directory := &roomDirectoryStub{snapshots: map[string]agenttool.RoomUserSnapshot{
		"trusted": {Room: "trusted", Users: []string{"t", "ſ", "S"}},
	}}
	result, err := (agenttool.RoomUsers{Directory: directory}).Execute(context.Background(), historyContext(t, "trusted"), json.RawMessage(`{}`))
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var output struct {
		Users []string `json:"users"`
	}
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(output.Users, ","), "S,ſ,t"; got != want {
		t.Fatalf("Unicode case-fold ordering = %q, want %q", got, want)
	}
}
