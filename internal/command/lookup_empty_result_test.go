package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/testutil/h2fixture"
)

func TestLookupEmptyResultsDeliverExplanations(t *testing.T) {
	db := h2fixture.Open(t, "lookup-empty-results")
	bundle := &service.Bundle{Users: &service.UserService{Queries: db}, Notes: &service.NoteService{DB: db.DB}}
	for _, tc := range []struct {
		aliases    []string
		args, want string
		private    bool
	}{
		{[]string{"lastonline", "seen", "last", "online", "lastseen"}, " nath", "No public history found for nickname or trip: nath.", false},
		{[]string{"nicks", "t2n"}, " UnknownTrip", "No nicknames found for trip: UnknownTrip.", false},
		{[]string{"users", "whitelist", "blacklist", "offenders", "knownoffenders"}, "", "No registered users found.", false},
		{[]string{"notes"}, "", "No saved notes found.", true},
	} {
		for _, alias := range tc.aliases {
			for _, whisper := range []bool{false, true} {
				t.Run(alias+boolString(whisper), func(t *testing.T) {
					e := &core.EngineImpl{Prefix: "*", Services: bundle, OutMessageQueue: make(chan string, 2)}
					d, _ := commandDefinitionFor(alias)
					status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "caller-trip", Text: "*" + alias + tc.args, IsWhisper: whisper}).Execute(context.Background())
					missingHistory := tc.aliases[0] == "lastonline"
					if missingHistory && (status != model.FAILED || !errors.Is(err, repository.ErrNotFound)) {
						t.Fatalf("missing history status=%v err=%v", status, err)
					}
					if !missingHistory && (err != nil || status != model.SUCCESSFUL) {
						t.Fatalf("status=%v err=%v", status, err)
					}
					select {
					case frame := <-e.OutMessageQueue:
						var got struct{ Cmd, Text string }
						if err := json.Unmarshal([]byte(frame), &got); err != nil {
							t.Fatal(err)
						}
						want := "@alice " + tc.want
						if whisper || tc.private {
							want = "/whisper @alice .\n" + tc.want
						}
						if got.Cmd != "chat" || got.Text != want {
							t.Fatalf("frame=%q want=%q", frame, want)
						}
					default:
						t.Fatal("lookup completed without a reply")
					}
					if len(e.OutMessageQueue) != 0 {
						t.Fatal("duplicate empty-result reply")
					}
				})
			}
		}
	}
	for _, alias := range []string{"lastseen", "t2n", "users", "notes"} {
		wantErr := errors.New("send failed")
		e := &gatewayEngine{commandEngineStub: commandEngineStub{bundle: bundle}, sendErr: wantErr}
		d, _ := commandDefinitionFor(alias)
		args := ""
		if alias == "lastseen" || alias == "t2n" {
			args = " absent"
		}
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Trip: "caller-trip", Text: "*" + alias + args}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, wantErr) {
			t.Errorf("%s status=%v err=%v", alias, status, err)
		}
	}
}

func TestNicksMissingServiceIsNotSuccessfulEmptyLookup(t *testing.T) {
	e := &commandEngineStub{}
	d, _ := commandDefinitionFor("t2n")
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "*t2n Trip"}).Execute(context.Background())
	if status != model.FAILED || err == nil || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}
