package snapshot

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/model"
)

type fakeSession struct {
	id       string
	closed   atomic.Int32
	flushed  atomic.Int32
	started  atomic.Int32
	startErr error
}

func (s *fakeSession) ID() string { return s.id }
func (s *fakeSession) Start() error {
	s.started.Add(1)
	return s.startErr
}
func (s *fakeSession) Close() error         { s.closed.Add(1); return nil }
func (s *fakeSession) Flush() error         { s.flushed.Add(1); return nil }
func (s *fakeSession) SendRaw(string) error { return nil }

type fakeFactory struct {
	session Session
	err     error
}

func (f fakeFactory) Create(RoomSnapshotRequest, SnapshotSink) (Session, error) {
	return f.session, f.err
}

type recordingOperation struct {
	mu      sync.Mutex
	calls   int
	result  OperationResult
	err     error
	seenLen int
}

func (o *recordingOperation) Apply(_ RoomSnapshotContext, snapshot Snapshot) (OperationResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	o.seenLen = len(snapshot.Users)
	return o.result, o.err
}

func request(op RoomSnapshotOperation) RoomSnapshotRequest {
	return RoomSnapshotRequest{WorkflowID: "wf-1", Author: "author", SourceChannel: "source", TargetChannel: "room", Operation: op}
}

func TestCoordinatorProcessesFirstCorrelatedSnapshotAndCleansUp(t *testing.T) {
	s := &fakeSession{id: "session-1"}
	op := &recordingOperation{result: OperationResult{Outcome: OutcomeSuccess, Reply: "done"}}
	var replies []string
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, func(_ RoomSnapshotRequest, reply string) error { replies = append(replies, reply); return nil }, ParseUsers, time.Second)

	if err := c.Submit(request(op)); err != nil {
		t.Fatal(err)
	}
	if c.State("wf-1") != StateRunning {
		t.Fatalf("state=%s", c.State("wf-1"))
	}
	if c.OnSnapshot("wrong-session", `{"cmd":"onlineSet","users":[]}`) {
		t.Fatal("unrelated event was accepted")
	}
	if !c.OnSnapshot("session-1", `{"cmd":"onlineSet","users":[{"nick":"Alice"}]}`) {
		t.Fatal("snapshot was not accepted")
	}
	if c.OnSnapshot("session-1", `{"cmd":"onlineSet","users":[{"nick":"Bob"}]}`) {
		t.Fatal("duplicate snapshot was accepted")
	}
	if op.calls != 1 || op.seenLen != 1 || s.flushed.Load() != 1 || s.closed.Load() != 1 {
		t.Fatalf("calls=%d users=%d flushed=%d closed=%d", op.calls, op.seenLen, s.flushed.Load(), s.closed.Load())
	}
	if len(replies) != 1 || replies[0] != "done" {
		t.Fatalf("replies=%v", replies)
	}
	if c.ActiveWorkflowCount() != 0 || c.State("wf-1") != StateCompleted {
		t.Fatalf("active=%d state=%s", c.ActiveWorkflowCount(), c.State("wf-1"))
	}
}

func TestCoordinatorCompletesRequestAfterPublishingReply(t *testing.T) {
	s := &fakeSession{id: "session-completion"}
	op := &recordingOperation{result: Success("remote users")}
	events := make([]string, 0, 2)
	var completed OperationResult
	var completionFlushed, completionClosed int32
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, func(_ RoomSnapshotRequest, _ string) error {
		events = append(events, "reply")
		return nil
	}, ParseUsers, time.Second)
	req := RoomSnapshotRequest{
		WorkflowID:    "wf-completion",
		Author:        "author",
		SourceChannel: "source",
		TargetChannel: "room",
		Operation:     op,
		OnComplete: func(result OperationResult) {
			completed = result
			completionFlushed = s.flushed.Load()
			completionClosed = s.closed.Load()
			events = append(events, "completion")
		},
	}

	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	if !c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`) {
		t.Fatal("snapshot was not accepted")
	}
	if completed.Outcome != OutcomeSuccess || completed.Reply != "remote users" || len(completed.Data) != 0 {
		t.Fatalf("completion result=%+v", completed)
	}
	if len(events) != 2 || events[0] != "reply" || events[1] != "completion" {
		t.Fatalf("event order=%v, want [reply completion]", events)
	}
	if completionFlushed != 1 || completionClosed != 1 {
		t.Fatalf("completion observed flushed=%d closed=%d, want terminal session lifecycle", completionFlushed, completionClosed)
	}
}

func TestCoordinatorClonesTypedDataAcrossOutcomeAndCompletionObservers(t *testing.T) {
	s := &fakeSession{id: "list-session"}
	operation := &recordingOperation{result: OperationResult{
		Outcome: OutcomeSuccess,
		Reply:   "remote users",
		Data:    json.RawMessage(`{"room":"lounge","users":[],"count":0,"returnedCount":0,"truncated":false}`),
	}}
	var completed OperationResult
	c := NewRoomSnapshotCoordinatorWithOutcome(fakeFactory{session: s}, nil, ParseUsers, time.Second, func(_ RoomSnapshotRequest, result OperationResult) {
		for index := range result.Data {
			result.Data[index] = 'x'
		}
	})
	req := request(operation)
	req.OnComplete = func(result OperationResult) { completed = result }
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	if !c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`) {
		t.Fatal("snapshot was not accepted")
	}
	if got, want := string(completed.Data), `{"room":"lounge","users":[],"count":0,"returnedCount":0,"truncated":false}`; got != want {
		t.Fatalf("completion data=%s, want %s", got, want)
	}
}

func TestCoordinatorCompletesRequestOnWorkflowFailure(t *testing.T) {
	s := &fakeSession{id: "session-failure"}
	completed := make(chan OperationResult, 1)
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	req := RoomSnapshotRequest{
		WorkflowID:    "wf-failure",
		Author:        "author",
		SourceChannel: "source",
		TargetChannel: "room",
		Operation:     &recordingOperation{},
		OnComplete:    func(result OperationResult) { completed <- result },
	}

	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	if !c.OnTransportError(s.id, errors.New("transport failed")) {
		t.Fatal("transport failure was not accepted")
	}
	result := <-completed
	if result.Outcome != OutcomeFailed || result.Reply != "transport failed" {
		t.Fatalf("completion result=%+v", result)
	}
}

func TestCoordinatorReportsEmptyOutcome(t *testing.T) {
	s := &fakeSession{id: "session"}
	op := &recordingOperation{result: OperationResult{Outcome: OutcomeEmpty, Reply: "room empty"}}
	var got OperationResult
	c := NewRoomSnapshotCoordinatorWithOutcome(fakeFactory{session: s}, nil, ParseUsers, time.Second, func(_ RoomSnapshotRequest, result OperationResult) { got = result })
	if err := c.Submit(request(op)); err != nil {
		t.Fatal(err)
	}
	c.OnSnapshot("session", `{"cmd":"onlineSet","users":[]}`)
	if got.Outcome != OutcomeEmpty || got.Reply != "room empty" {
		t.Fatalf("result=%+v", got)
	}
}

func TestCoordinatorReportsOperationFailureOutcome(t *testing.T) {
	s := &fakeSession{id: "session"}
	op := &recordingOperation{err: errors.New("operation failed")}
	var got OperationResult
	c := NewRoomSnapshotCoordinatorWithOutcome(fakeFactory{session: s}, nil, ParseUsers, time.Second, func(_ RoomSnapshotRequest, result OperationResult) { got = result })
	if err := c.Submit(request(op)); err != nil {
		t.Fatal(err)
	}
	if !c.OnSnapshot("session", `{"cmd":"onlineSet","users":[]}`) {
		t.Fatal("snapshot was not accepted")
	}
	if got.Outcome != OutcomeFailed || c.State("wf-1") != StateFailed || s.closed.Load() != 1 {
		t.Fatalf("result=%+v state=%s closed=%d", got, c.State("wf-1"), s.closed.Load())
	}
}

func TestCoordinatorFailureAndStartFailureCleanup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event func(*RoomSnapshotCoordinator)
		want  WorkflowState
	}{
		{"transport", func(c *RoomSnapshotCoordinator) { c.OnTransportError("session", errors.New("broken")) }, StateFailed},
		{"closed", func(c *RoomSnapshotCoordinator) { c.OnClosed("session", 1006, "gone") }, StateFailed},
		{"cancel", func(c *RoomSnapshotCoordinator) { c.Cancel("wf-1", "user cancelled") }, StateCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeSession{id: "session"}
			c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
			completed := make(chan OperationResult, 1)
			req := request(&recordingOperation{})
			req.OnComplete = func(r OperationResult) { completed <- r }
			if err := c.Submit(req); err != nil {
				t.Fatal(err)
			}
			tc.event(c)
			<-completed
			if c.State("wf-1") != tc.want || s.closed.Load() != 1 || c.ActiveWorkflowCount() != 0 {
				t.Fatalf("state=%s closed=%d active=%d", c.State("wf-1"), s.closed.Load(), c.ActiveWorkflowCount())
			}
		})
	}

	s := &fakeSession{id: "session", startErr: errors.New("start")}
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	if err := c.Submit(request(&recordingOperation{})); err == nil {
		t.Fatal("expected start error")
	}
	if s.closed.Load() != 1 || c.ActiveWorkflowCount() != 0 || c.State("wf-1") != StateFailed {
		t.Fatalf("closed=%d active=%d state=%s", s.closed.Load(), c.ActiveWorkflowCount(), c.State("wf-1"))
	}
}

func TestCoordinatorUsesRequestSpecificFailureReply(t *testing.T) {
	session := &fakeSession{id: "list-session"}
	var replies []string
	coordinator := NewRoomSnapshotCoordinator(fakeFactory{session: session}, func(_ RoomSnapshotRequest, reply string) error {
		replies = append(replies, reply)
		return nil
	}, ParseUsers, time.Second)
	req := request(&recordingOperation{})
	req.ReplyMessage = "Unable to list users in the requested room."

	if err := coordinator.Submit(req); err != nil {
		t.Fatal(err)
	}
	if !coordinator.OnSnapshot(session.id, `{"cmd":"warn","text":"invalid nick"}`) {
		t.Fatal("invalid snapshot frame did not terminate the workflow")
	}

	if len(replies) != 1 || replies[0] != req.ReplyMessage {
		t.Fatalf("failure replies = %#v, want %#v", replies, []string{req.ReplyMessage})
	}
}

func TestCoordinatorTimeoutAndLateEventsAreRejected(t *testing.T) {
	s := &fakeSession{id: "session"}
	op := &recordingOperation{}
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, 15*time.Millisecond)
	if err := c.Submit(request(op)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for c.State("wf-1") != StateTimedOut && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.State("wf-1") != StateTimedOut || s.closed.Load() != 1 {
		t.Fatalf("state=%s closed=%d", c.State("wf-1"), s.closed.Load())
	}
	if c.OnSnapshot("session", `{"cmd":"onlineSet","users":[]}`) {
		t.Fatal("late event accepted")
	}
	if op.calls != 0 {
		t.Fatal("late event ran operation")
	}
}

func ParseUsers(payload string) (Snapshot, error) { return Parse(payload, false) }

var _ = model.User{}
