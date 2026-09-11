package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"zenbot/internal/transport"
)

type replicaTestTransport struct {
	messages   chan transport.InboundMessage
	errors     chan error
	closed     chan struct{}
	closeOnce  sync.Once
	joinError  error
	closeError error
	start      func(context.Context) error
	join       string
}

func newReplicaTestTransport() *replicaTestTransport {
	return &replicaTestTransport{messages: make(chan transport.InboundMessage, 1), errors: make(chan error, 1), closed: make(chan struct{})}
}
func (tr *replicaTestTransport) Start(ctx context.Context) error {
	if tr.start != nil {
		return tr.start(ctx)
	}
	return ctx.Err()
}
func (tr *replicaTestTransport) Messages() <-chan transport.InboundMessage { return tr.messages }
func (tr *replicaTestTransport) Errors() <-chan error                      { return tr.errors }
func (tr *replicaTestTransport) Connected() bool                           { return true }
func (tr *replicaTestTransport) SendText(ctx context.Context, payload string) error {
	tr.join = payload
	if err := ctx.Err(); err != nil {
		return err
	}
	return tr.joinError
}

func TestReplicaJoinEscapesProtocolFields(t *testing.T) {
	tr := newReplicaTestTransport()
	e := &EngineImpl{Channel: `room"\\`, Name: `bot"\\`, Password: `pw"\\`, Transport: tr}
	if err := e.StartContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.StopContext(context.Background()) })
	var joined map[string]string
	if err := json.Unmarshal([]byte(tr.join), &joined); err != nil {
		t.Fatalf("invalid join JSON %q: %v", tr.join, err)
	}
	if len(joined) != 3 || joined["cmd"] != "join" || joined["channel"] != `room"\\` || joined["nick"] != `bot"\\#pw"\\` {
		t.Fatalf("join fields=%#v", joined)
	}
}
func (tr *replicaTestTransport) SendRaw(context.Context, []byte) error { return nil }
func (tr *replicaTestTransport) Close(context.Context) error {
	tr.closeOnce.Do(func() {
		if tr.closeError != nil {
			tr.errors <- tr.closeError
		}
		close(tr.closed)
	})
	return nil
}

func awaitReplicaSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func TestReplicaJoinFailureDoesNotWaitForUnstartedDispatcher(t *testing.T) {
	tr := newReplicaTestTransport()
	tr.joinError = errors.New("join rejected")
	e := &EngineImpl{Channel: "room", Transport: tr}
	started := make(chan error, 1)
	go func() { started <- e.StartContext(context.Background()) }()
	select {
	case err := <-started:
		if !errors.Is(err, tr.joinError) {
			t.Fatalf("start error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup cleanup deadlocked")
	}
	awaitReplicaSignal(t, tr.closed, "failed startup did not close transport")
	awaitReplicaSignal(t, e.runtimeDone, "failed startup did not complete")
}

type replicaContextListener struct{ seen chan context.Context }

func (l replicaContextListener) Notify(string)                               {}
func (l replicaContextListener) NotifyContext(ctx context.Context, _ string) { l.seen <- ctx }

func TestReplicaSurvivesRequestCancellationUntilRemoval(t *testing.T) {
	tr := newReplicaTestTransport()
	seen := make(chan context.Context, 1)
	e := &EngineImpl{Channel: "room", Transport: tr, UserChatListener: replicaContextListener{seen}}
	m := NewReplicaManager("host")
	c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) { return e, nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.AddReplica(ctx, "room"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.StopAll(context.Background()) })
	cancel()
	tr.messages <- transportpkgMessage(`{"cmd":"chat","nick":"alice","text":"!ping"}`)
	select {
	case dispatched := <-seen:
		if dispatched.Err() != nil {
			t.Fatal("command inherited cancelled request")
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation stopped permanent replica dispatch")
	}
	if err := c.RemoveReplica(context.Background(), "room"); err != nil {
		t.Fatal(err)
	}
	awaitReplicaSignal(t, e.runtimeDone, "removed dispatcher remains alive")
	awaitReplicaSignal(t, tr.closed, "removed transport remains open")
}

func TestReplicaRuntimeFailureRemovesAndCompletesDispatcher(t *testing.T) {
	for _, failure := range []string{"transport", "transport closed", "transport canceled", "wrapped transport canceled", "messages closed", "errors closed"} {
		t.Run(failure, func(t *testing.T) {
			tr := newReplicaTestTransport()
			e := &EngineImpl{Channel: "room", Transport: tr}
			m := NewReplicaManager("host")
			c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) { return e, nil })
			if err := c.AddReplica(context.Background(), "room"); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "transport":
				tr.errors <- errors.New("connection lost")
			case "transport closed":
				tr.errors <- transport.ErrClosed
			case "transport canceled":
				tr.errors <- context.Canceled
			case "wrapped transport canceled":
				tr.errors <- errors.Join(errors.New("independent operation"), context.Canceled)
			case "messages closed":
				close(tr.messages)
			case "errors closed":
				close(tr.errors)
			}
			awaitReplicaSignal(t, e.runtimeDone, "failed dispatcher did not exit")
			awaitReplicaSignal(t, tr.closed, "failed transport was not closed")
			if len(m.Channels()) != 0 {
				t.Fatal("failed replica still registered")
			}
		})
	}
}

func TestReplicaExplicitStopDoesNotReportTransportCancellation(t *testing.T) {
	for range 50 {
		tr := newReplicaTestTransport()
		tr.closeError = context.Canceled
		e := &EngineImpl{Channel: "room", Transport: tr}
		m := NewReplicaManager("host")
		reports := make(chan error, 1)
		c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) { return e, nil }, reports)
		if err := c.AddReplica(context.Background(), "room"); err != nil {
			t.Fatal(err)
		}
		if err := c.RemoveReplica(context.Background(), "room"); err != nil {
			t.Fatal(err)
		}
		awaitReplicaSignal(t, e.runtimeDone, "explicit stop left dispatcher alive")
		awaitReplicaSignal(t, tr.closed, "explicit stop left transport open")
		if len(m.Channels()) != 0 {
			t.Fatal("explicit stop left replica registered")
		}
		select {
		case err := <-reports:
			t.Fatalf("explicit stop reported as failure: %v", err)
		case <-time.After(10 * time.Millisecond):
			// runtimeDone precedes reporting. Allow the callback a bounded
			// settling window; this is not proof of indefinite silence.
		}
	}
}

func TestReplicaStartupStopDoesNotReportTransportCancellation(t *testing.T) {
	tr := newReplicaTestTransport()
	entered := make(chan struct{})
	tr.start = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	reports := make(chan error, 1)
	e := &EngineImpl{Channel: "room", Transport: tr, LifecycleErrors: reports}
	started := make(chan error, 1)
	go func() { started <- e.StartContext(context.Background()) }()
	awaitReplicaSignal(t, entered, "startup did not reach transport")
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := e.StopContext(stopCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-started:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup result=%v", err)
		}
	case <-stopCtx.Done():
		t.Fatal("startup cancellation did not return")
	}
	// StartContext has returned, so its synchronous reporting path is finished.
	select {
	case err := <-reports:
		t.Fatalf("startup stop reported as failure: %v", err)
	default:
	}
	awaitReplicaSignal(t, tr.closed, "startup stop left transport open")
}

func TestReplicaSurvivesMasterReplacementAndStopsWithManager(t *testing.T) {
	m := NewReplicaManager("host")
	masterTr, replicaTr := newReplicaTestTransport(), newReplicaTestTransport()
	masterSeen, replicaSeen := make(chan context.Context, 1), make(chan context.Context, 1)
	master := &EngineImpl{Channel: "host", Transport: masterTr, UserChatListener: replicaContextListener{masterSeen}}
	replica := &EngineImpl{Channel: "room", Transport: replicaTr, UserChatListener: replicaContextListener{replicaSeen}}
	construct := func(context.Context, string) (ManagedEngine, error) { return replica, nil }
	master.SetReplicaController(NewManagedReplicaController(m, construct))
	if err := master.StartContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = master.StopContext(context.Background()); _ = m.StopAll(context.Background()) })
	masterTr.messages <- transportpkgMessage(`{"cmd":"chat","nick":"alice","text":"!replica room"}`)
	var masterContext context.Context
	select {
	case masterContext = <-masterSeen:
	case <-time.After(time.Second):
		t.Fatal("master did not dispatch")
	}
	if err := master.AddReplica(masterContext, "room"); err != nil {
		t.Fatal(err)
	}
	if err := master.StopContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	replacement := &EngineImpl{Channel: "host"}
	replacement.SetReplicaController(NewManagedReplicaController(m, construct))
	if len(replacement.ReplicaChannels()) != 1 {
		t.Fatal("replacement master lost existing replica")
	}
	replicaTr.messages <- transportpkgMessage(`{"cmd":"chat","nick":"alice","text":"!ping"}`)
	select {
	case ctx := <-replicaSeen:
		if ctx.Err() != nil {
			t.Fatal("retiring master cancelled replica command")
		}
	case <-time.After(time.Second):
		t.Fatal("replica stopped dispatching after master replacement")
	}
	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	awaitReplicaSignal(t, replica.runtimeDone, "process teardown left replica dispatcher alive")
	awaitReplicaSignal(t, replicaTr.closed, "process teardown left replica transport open")
}

func TestReplicaLateFailureDoesNotRemoveReplacement(t *testing.T) {
	firstTr, nextTr := newReplicaTestTransport(), newReplicaTestTransport()
	first := &EngineImpl{Channel: "room", Transport: firstTr}
	next := &EngineImpl{Channel: "room", Transport: nextTr}
	var calls atomic.Int32
	m := NewReplicaManager("host")
	c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) {
		if calls.Add(1) == 1 {
			return first, nil
		}
		return next, nil
	})
	if err := c.AddReplica(context.Background(), "room"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveReplica(context.Background(), "room"); err != nil {
		t.Fatal(err)
	}
	if err := c.AddReplica(context.Background(), "room"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.StopAll(context.Background()) })
	first.runtimeFailure(errors.New("late first failure"))
	if m.ManagedEngines()["room"] != next {
		t.Fatal("late callback removed replacement")
	}
	select {
	case <-nextTr.closed:
		t.Fatal("late callback stopped replacement")
	default:
	}
}

func TestReplicaConstructionAndStartHonorRequestCancellation(t *testing.T) {
	tr := newReplicaTestTransport()
	entered := make(chan struct{})
	tr.start = func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }
	e := &EngineImpl{Channel: "room", Transport: tr}
	m := NewReplicaManager("host")
	c := NewManagedReplicaController(m, func(ctx context.Context, _ string) (ManagedEngine, error) {
		if ctx == nil {
			t.Error("constructor lost request context")
		}
		return e, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- c.AddReplica(ctx, "room") }()
	awaitReplicaSignal(t, entered, "start not entered")
	cancel()
	select {
	case err := <-started:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not interrupt start")
	}
	awaitReplicaSignal(t, tr.closed, "cancelled startup transport not closed")
	if len(m.Channels()) != 0 {
		t.Fatal("cancelled replica registered")
	}
}

func TestReplicaConcurrentDuplicateStartsKeepOnlyWinningInstance(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	engines := make([]*EngineImpl, 2)
	transports := make([]*replicaTestTransport, 2)
	for i := range engines {
		tr := newReplicaTestTransport()
		tr.start = func(context.Context) error { entered <- struct{}{}; <-release; return nil }
		transports[i] = tr
		engines[i] = &EngineImpl{Channel: "room", Transport: tr}
	}
	var calls atomic.Int32
	m := NewReplicaManager("host")
	c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) {
		return engines[int(calls.Add(1))-1], nil
	})
	results := make(chan error, 2)
	for range engines {
		go func() { results <- c.AddReplica(context.Background(), "room") }()
	}
	awaitReplicaSignal(t, entered, "first startup did not enter")
	awaitReplicaSignal(t, entered, "second startup did not enter")
	close(release)
	var succeeded int
	for range engines {
		select {
		case err := <-results:
			if err == nil {
				succeeded++
			}
		case <-time.After(time.Second):
			t.Fatal("duplicate startup cleanup stuck")
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful starts=%d", succeeded)
	}
	t.Cleanup(func() { _ = m.StopAll(context.Background()) })
	winner := m.ManagedEngines()["room"]
	for i, e := range engines {
		if winner == e {
			select {
			case <-transports[i].closed:
				t.Fatal("winner closed")
			default:
			}
		} else {
			awaitReplicaSignal(t, transports[i].closed, "loser transport open")
			awaitReplicaSignal(t, e.runtimeDone, "loser dispatcher alive")
			e.runtimeFailure(errors.New("late duplicate failure"))
		}
	}
	if m.ManagedEngines()["room"] != winner {
		t.Fatal("loser callback removed winner")
	}
}

func TestReplicaFailedRegistrationClosesCreatedInstance(t *testing.T) {
	m := NewReplicaManager("host")
	if err := m.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	tr := newReplicaTestTransport()
	e := &EngineImpl{Channel: "room", Transport: tr}
	c := NewManagedReplicaController(m, func(context.Context, string) (ManagedEngine, error) { return e, nil })
	if err := c.AddReplica(context.Background(), "room"); err == nil {
		t.Fatal("registration after teardown accepted")
	}
	awaitReplicaSignal(t, tr.closed, "registration failure left transport open")
	awaitReplicaSignal(t, e.runtimeDone, "registration failure left dispatcher alive")
}
