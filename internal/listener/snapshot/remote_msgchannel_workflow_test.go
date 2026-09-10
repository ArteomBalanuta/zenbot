package snapshot

import (
	"errors"
	"testing"
)

type remoteWorkflowSession struct {
	id      string
	raws    []string
	flushed int
	closed  int
	sendErr error
}

func (s *remoteWorkflowSession) ID() string   { return s.id }
func (s *remoteWorkflowSession) Start() error { return nil }
func (s *remoteWorkflowSession) Flush() error { s.flushed++; return nil }
func (s *remoteWorkflowSession) Close() error { s.closed++; return nil }
func (s *remoteWorkflowSession) SendRaw(raw string) error {
	s.raws = append(s.raws, raw)
	return s.sendErr
}

func remoteWorkflowRequest() RoomSnapshotRequest {
	return RoomSnapshotRequest{WorkflowID: "remote-workflow", Author: "alice", Whisper: true, SourceChannel: "source", TargetChannel: "target", ReplyMessage: "Unable to complete room operation.", RemoteMessage: "body", Operation: NewRemoteMessageOperation("body")}
}

func TestRemoteMsgChannelWorkflow(t *testing.T) {
	t.Run("success replies once and cleans up late events", func(t *testing.T) {
		session := &remoteWorkflowSession{id: "remote-session"}
		var replies []string
		coordinator := NewRoomSnapshotCoordinator(fakeFactory{session: session}, func(_ RoomSnapshotRequest, reply string) { replies = append(replies, reply) }, ParseUsers, 0)
		if err := coordinator.Submit(remoteWorkflowRequest()); err != nil {
			t.Fatal(err)
		}
		if !coordinator.OnSnapshot(session.id, `{"cmd":"onlineSet","users":[{"nick":"temporary","isme":true},{"nick":"member"}]}`) || coordinator.OnSnapshot(session.id, `{"cmd":"onlineSet","users":[{"nick":"member"}]}`) {
			t.Fatal("first snapshot should be accepted exactly once")
		}
		if len(session.raws) != 1 || len(replies) != 1 || replies[0] != "sent successfully." || session.flushed != 1 || session.closed != 1 || coordinator.ActiveWorkflowCount() != 0 {
			t.Fatalf("raws=%v replies=%v flush=%d close=%d active=%d", session.raws, replies, session.flushed, session.closed, coordinator.ActiveWorkflowCount())
		}
	})

	t.Run("all is me replies empty without raw", func(t *testing.T) {
		session := &remoteWorkflowSession{id: "empty-session"}
		var replies []string
		coordinator := NewRoomSnapshotCoordinator(fakeFactory{session: session}, func(_ RoomSnapshotRequest, reply string) { replies = append(replies, reply) }, ParseUsers, 0)
		request := remoteWorkflowRequest()
		request.WorkflowID = "empty-workflow"
		if err := coordinator.Submit(request); err != nil || !coordinator.OnSnapshot(session.id, `{"cmd":"onlineSet","users":[{"nick":"temporary","isme":true}]}`) {
			t.Fatalf("submit=%v", err)
		}
		if len(session.raws) != 0 || len(replies) != 1 || replies[0] != " target is empty" || session.flushed != 1 || session.closed != 1 {
			t.Fatalf("raws=%v replies=%v flush=%d close=%d", session.raws, replies, session.flushed, session.closed)
		}
	})

	t.Run("parser and raw errors publish existing generic failure", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			parser  SnapshotParser
			sendErr error
			payload string
		}{
			{name: "parser", parser: func(string) (Snapshot, error) { return Snapshot{}, errors.New("parse failed") }, payload: "bad"},
			{name: "raw", parser: ParseUsers, sendErr: errors.New("raw failed"), payload: `{"cmd":"onlineSet","users":[{"nick":"member"}]}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				session := &remoteWorkflowSession{id: tc.name + "-session", sendErr: tc.sendErr}
				var replies []string
				coordinator := NewRoomSnapshotCoordinator(fakeFactory{session: session}, func(_ RoomSnapshotRequest, reply string) { replies = append(replies, reply) }, tc.parser, 0)
				request := remoteWorkflowRequest()
				request.WorkflowID = tc.name + "-workflow"
				if err := coordinator.Submit(request); err != nil || !coordinator.OnSnapshot(session.id, tc.payload) {
					t.Fatalf("submit=%v", err)
				}
				if len(replies) != 1 || replies[0] != "Unable to complete room operation." || session.closed != 1 || coordinator.ActiveWorkflowCount() != 0 {
					t.Fatalf("replies=%v close=%d active=%d", replies, session.closed, coordinator.ActiveWorkflowCount())
				}
			})
		}
	})
}
