package snapshot

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestOwnershipZeroWriteCancellationDuringCloseIsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &controlledSession{fakeSession: fakeSession{id: "zero-write-close"}, closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	completed := make(chan OperationResult, 1)
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	req := request(NewListRoomOperation())
	req.Context = ctx
	req.OnComplete = func(r OperationResult) { completed <- r }
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	received := make(chan struct{})
	go func() { c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`); close(received) }()
	<-s.closeEntered
	cancel()
	select {
	case <-completed:
		t.Error("completed before close returned")
	default:
	}
	close(s.closeRelease)
	<-received
	r := <-completed
	if r.Outcome != OutcomeFailed || !r.OutcomeUnknown || r.ActionCount != 0 || r.DeliveryCount != 0 || !errors.Is(r.Error, context.Canceled) || c.State(req.WorkflowID) != StateCancelled {
		t.Fatalf("result=%+v state=%s", r, c.State(req.WorkflowID))
	}
}

func TestOwnershipCancellationDuringBlockedReplyRemainsObservable(t *testing.T) {
	for _, mode := range []string{"caller", "coordinator"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			completed := make(chan OperationResult, 1)
			s := &fakeSession{id: "blocked-reply"}
			c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, func(RoomSnapshotRequest, string) error { close(entered); <-release; return nil }, ParseUsers, time.Second)
			req := request(NewListRoomOperation())
			req.Context = ctx
			req.OnComplete = func(r OperationResult) { completed <- r }
			if err := c.Submit(req); err != nil {
				t.Fatal(err)
			}
			received := make(chan struct{})
			go func() { c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`); close(received) }()
			<-entered
			if mode == "caller" {
				cancel()
			} else if !c.Cancel(req.WorkflowID, "cancel while delivering") {
				t.Error("coordinator rejected cancellation before reply returned")
			}
			select {
			case <-completed:
				t.Error("completed before reply returned")
			default:
			}
			if c.ActiveWorkflowCount() != 1 {
				t.Error("blocked delivery lost owner")
			}
			close(release)
			<-received
			r := <-completed
			if r.Outcome != OutcomeFailed || !r.OutcomeUnknown || r.ActionCount != 1 || r.DeliveryCount != 1 || r.Error == nil || c.State(req.WorkflowID) != StateCancelled {
				t.Fatalf("result=%+v state=%s", r, c.State(req.WorkflowID))
			}
		})
	}
}

func TestOwnershipLateCallbacksCannotCancelReusedWorkflowAndRetentionIsBounded(t *testing.T) {
	var oldRequest RoomSnapshotRequest
	var oldSink SnapshotSink
	sessions := 0
	c := NewRoomSnapshotCoordinator(SessionFactoryFunc(func(req RoomSnapshotRequest, sink SnapshotSink) (Session, error) {
		sessions++
		if sessions == 1 {
			oldRequest, oldSink = req, sink
		}
		return &fakeSession{id: fmt.Sprintf("session-%d", sessions)}, nil
	}), nil, ParseUsers, time.Second)
	req := request(&recordingOperation{result: Success()})
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	c.OnSnapshot("session-1", `{"cmd":"onlineSet","users":[]}`)
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	oldSink(`{"cmd":"onlineSet","users":[]}`)
	oldRequest.onTransportError(errors.New("late"))
	if c.ActiveWorkflowCount() != 1 || c.State(req.WorkflowID) != StateRunning {
		t.Fatal("old callbacks changed replacement workflow")
	}
	c.OnSnapshot("session-2", `{"cmd":"onlineSet","users":[]}`)
	for i := 0; i < 300; i++ {
		req.WorkflowID = fmt.Sprintf("retained-%d", i)
		if err := c.Submit(req); err != nil {
			t.Fatal(err)
		}
		c.OnSnapshot(fmt.Sprintf("session-%d", i+3), `{"cmd":"onlineSet","users":[]}`)
	}
	if c.State("retained-0") != "" || c.State("retained-299") != StateCompleted || len(c.states) > 256 {
		t.Fatalf("retained=%d oldest=%s newest=%s", len(c.states), c.State("retained-0"), c.State("retained-299"))
	}
}

func TestOwnershipRetainsCloseAndReplyErrors(t *testing.T) {
	for _, tc := range []struct {
		name               string
		closeErr, replyErr error
	}{{"close", errors.New("close failed"), nil}, {"reply", nil, errors.New("reply failed")}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &controlledSession{fakeSession: fakeSession{id: "errors"}, closeErr: tc.closeErr}
			done := make(chan OperationResult, 1)
			c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, func(RoomSnapshotRequest, string) error { return tc.replyErr }, ParseUsers, time.Second)
			req := request(&recordingOperation{result: Success("done")})
			req.OnComplete = func(r OperationResult) { done <- r }
			if err := c.Submit(req); err != nil {
				t.Fatal(err)
			}
			c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`)
			r := <-done
			expected := tc.closeErr
			if expected == nil {
				expected = tc.replyErr
			}
			if r.Outcome != OutcomeFailed || !errors.Is(r.Error, expected) || r.DeliveryCount != 0 {
				t.Fatalf("result=%+v", r)
			}
		})
	}
}

func TestOwnershipCancellationDuringCreationDoesNotStartReturnedSession(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := &fakeSession{id: "created-late"}
	c := NewRoomSnapshotCoordinator(SessionFactoryFunc(func(req RoomSnapshotRequest, _ SnapshotSink) (Session, error) {
		close(entered)
		<-release
		return s, nil
	}), nil, ParseUsers, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := request(&recordingOperation{})
	req.Context = ctx
	returned := make(chan error, 1)
	go func() { returned <- c.Submit(req) }()
	<-entered
	cancel()
	close(release)
	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatalf("submit=%v", err)
	}
	if s.started.Load() != 0 || s.closed.Load() != 1 || c.ActiveWorkflowCount() != 0 {
		t.Fatalf("started=%d closed=%d active=%d", s.started.Load(), s.closed.Load(), c.ActiveWorkflowCount())
	}
}

func TestOwnershipPanicRetainsBoundaryEvidenceAndCloses(t *testing.T) {
	s := &fakeSession{id: "panic"}
	completed := make(chan OperationResult, 1)
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	req := request(OperationFunc(func(ctx RoomSnapshotContext, _ Snapshot) (OperationResult, error) {
		_ = ctx.SendRaw(`{"cmd":"ban","nick":"alice"}`)
		panic("after send")
	}))
	req.OnComplete = func(r OperationResult) { completed <- r }
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`)
	r := <-completed
	if r.ActionCount != 1 || !r.OutcomeUnknown || r.Outcome != OutcomeFailed || s.closed.Load() != 1 {
		t.Fatalf("result=%+v closed=%d", r, s.closed.Load())
	}
}

func TestOwnershipDuplicateSnapshotOnlyExecutesOnce(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := &fakeSession{id: "duplicate"}
	calls := 0
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	req := request(OperationFunc(func(RoomSnapshotContext, Snapshot) (OperationResult, error) {
		calls++
		close(entered)
		<-release
		return Success(), nil
	}))
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`); close(done) }()
	<-entered
	if c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`) {
		t.Error("duplicate accepted")
	}
	close(release)
	<-done
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestOwnershipSynchronousStartCallbackDoesNotDeadlock(t *testing.T) {
	completed := make(chan OperationResult, 1)
	c := NewRoomSnapshotCoordinator(SessionFactoryFunc(func(_ RoomSnapshotRequest, sink SnapshotSink) (Session, error) {
		return &callbackStartSession{fakeSession: fakeSession{id: "sync"}, sink: sink}, nil
	}), nil, ParseUsers, time.Second)
	req := request(&recordingOperation{result: Success()})
	req.OnComplete = func(r OperationResult) { completed <- r }
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-completed:
		if r.Outcome != OutcomeSuccess {
			t.Fatalf("result=%+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("callback deadlocked owner")
	}
}

type callbackStartSession struct {
	fakeSession
	sink SnapshotSink
}

func (s *callbackStartSession) Start() error { s.sink(`{"cmd":"onlineSet","users":[]}`); return nil }

type controlledSession struct {
	fakeSession
	flushEntered, flushRelease, closeEntered, closeRelease chan struct{}
	flushErr, closeErr                                     error
}

func (s *controlledSession) Flush() error {
	if s.flushEntered != nil {
		close(s.flushEntered)
		<-s.flushRelease
	}
	return s.flushErr
}
func (s *controlledSession) Close() error {
	if s.closeEntered != nil {
		close(s.closeEntered)
		<-s.closeRelease
	}
	return s.closeErr
}

func TestOwnershipRemainsActiveUntilOperationFlushAndCloseReturn(t *testing.T) {
	s := &controlledSession{fakeSession: fakeSession{id: "owned"}, flushEntered: make(chan struct{}), flushRelease: make(chan struct{}), closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	entered, release, completed := make(chan struct{}), make(chan struct{}), make(chan OperationResult, 1)
	c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
	req := request(OperationFunc(func(RoomSnapshotContext, Snapshot) (OperationResult, error) {
		close(entered)
		<-release
		return Success(), nil
	}))
	req.OnComplete = func(r OperationResult) { completed <- r }
	if err := c.Submit(req); err != nil {
		t.Fatal(err)
	}
	received := make(chan struct{})
	go func() { c.OnSnapshot("owned", `{"cmd":"onlineSet","users":[]}`); close(received) }()
	<-entered
	check := func() {
		t.Helper()
		select {
		case <-completed:
			t.Error("workflow completed before operation/flush cleanup")
		default:
		}
		if c.ActiveWorkflowCount() != 1 || c.State(req.WorkflowID) != StateRunning {
			t.Error("running operation lost lifecycle owner")
		}
	}
	check()
	close(release)
	<-s.flushEntered
	check()
	close(s.flushRelease)
	<-s.closeEntered
	check()
	close(s.closeRelease)
	<-received
	if r := <-completed; r.Outcome != OutcomeSuccess {
		t.Fatalf("result=%+v", r)
	}
}

func TestOwnershipFlushFailureAndFailedResultCannotPublishSuccess(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   OperationResult
		flushErr error
	}{{"flush", Success("done"), errors.New("flush failed")}, {"failed", Failed("failed"), nil}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &controlledSession{fakeSession: fakeSession{id: "owned"}, flushErr: tc.flushErr}
			completed := make(chan OperationResult, 1)
			c := NewRoomSnapshotCoordinator(fakeFactory{session: s}, nil, ParseUsers, time.Second)
			req := request(&recordingOperation{result: tc.result})
			req.OnComplete = func(r OperationResult) { completed <- r }
			if err := c.Submit(req); err != nil {
				t.Fatal(err)
			}
			c.OnSnapshot(s.id, `{"cmd":"onlineSet","users":[]}`)
			if r := <-completed; r.Outcome != OutcomeFailed || c.State(req.WorkflowID) != StateFailed {
				t.Fatalf("result=%+v state=%s", r, c.State(req.WorkflowID))
			}
		})
	}
}
