package snapshot

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type RoomSnapshotRequest struct {
	WorkflowID         string
	Author             string
	SourceChannel      string
	TargetChannel      string
	DestinationChannel string
	ReplyMessage       string
	Operation          RoomSnapshotOperation
}

func (r RoomSnapshotRequest) validate() error {
	for name, value := range map[string]string{"workflowId": r.WorkflowID, "author": r.Author, "sourceChannel": r.SourceChannel, "targetChannel": r.TargetChannel} {
		if value == "" {
			return fmt.Errorf("%s cannot be blank", name)
		}
	}
	if r.Operation == nil {
		return errors.New("operation cannot be nil")
	}
	return nil
}

type RoomSnapshotContext struct {
	WorkflowID, Author, SourceChannel, TargetChannel, DestinationChannel string
	Reply                                                                func(string)
	SendRaw                                                              func(string) error
}

type RoomSnapshotOperation interface {
	Apply(RoomSnapshotContext, Snapshot) (OperationResult, error)
}
type OperationFunc func(RoomSnapshotContext, Snapshot) (OperationResult, error)

func (f OperationFunc) Apply(c RoomSnapshotContext, s Snapshot) (OperationResult, error) {
	return f(c, s)
}

type OperationOutcome string

const (
	OutcomeSuccess      OperationOutcome = "SUCCESS"
	OutcomeEmpty        OperationOutcome = "EMPTY"
	OutcomeAbsentTarget OperationOutcome = "ABSENT_TARGET"
	OutcomeSkipped      OperationOutcome = "SKIPPED"
	OutcomeFailed       OperationOutcome = "FAILED"
)

type OperationResult struct {
	Outcome OperationOutcome
	Reply   string
}

func Success(reply ...string) OperationResult { return result(OutcomeSuccess, reply...) }
func Empty(reply ...string) OperationResult   { return result(OutcomeEmpty, reply...) }
func Absent(reply ...string) OperationResult  { return result(OutcomeAbsentTarget, reply...) }
func Skipped() OperationResult                { return OperationResult{Outcome: OutcomeSkipped} }
func Failed(reply ...string) OperationResult  { return result(OutcomeFailed, reply...) }
func result(outcome OperationOutcome, reply ...string) OperationResult {
	r := OperationResult{Outcome: outcome}
	if len(reply) > 0 {
		r.Reply = reply[0]
	}
	return r
}

type Session interface {
	ID() string
	Start() error
	Close() error
	Flush() error
	SendRaw(string) error
}
type SnapshotSink func(string)
type SessionFactory interface {
	Create(RoomSnapshotRequest, SnapshotSink) (Session, error)
}
type SessionFactoryFunc func(RoomSnapshotRequest, SnapshotSink) (Session, error)

func (f SessionFactoryFunc) Create(r RoomSnapshotRequest, sink SnapshotSink) (Session, error) {
	return f(r, sink)
}

type ReplySink func(RoomSnapshotRequest, string)
type OutcomeSink func(RoomSnapshotRequest, OperationResult)
type SnapshotParser func(string) (Snapshot, error)

type WorkflowState string

const (
	StatePending   WorkflowState = "PENDING"
	StateRunning   WorkflowState = "RUNNING"
	StateCompleted WorkflowState = "COMPLETED"
	StateFailed    WorkflowState = "FAILED"
	StateCancelled WorkflowState = "CANCELLED"
	StateTimedOut  WorkflowState = "TIMED_OUT"
)

type RoomSnapshotCoordinator struct {
	factory SessionFactory
	reply   ReplySink
	outcome OutcomeSink
	parser  SnapshotParser
	timeout time.Duration
	mu      sync.Mutex
	active  map[string]*workflow
	states  map[string]WorkflowState
}
type workflow struct {
	coordinator *RoomSnapshotCoordinator
	request     RoomSnapshotRequest
	session     Session
	timer       *time.Timer
	mu          sync.Mutex
	state       WorkflowState
}

func NewRoomSnapshotCoordinator(factory SessionFactory, reply ReplySink, parser SnapshotParser, timeout time.Duration) *RoomSnapshotCoordinator {
	return NewRoomSnapshotCoordinatorWithOutcome(factory, reply, parser, timeout, nil)
}
func NewRoomSnapshotCoordinatorWithOutcome(factory SessionFactory, reply ReplySink, parser SnapshotParser, timeout time.Duration, outcome OutcomeSink) *RoomSnapshotCoordinator {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	c := &RoomSnapshotCoordinator{factory: factory, reply: reply, outcome: outcome, parser: parser, timeout: timeout, active: make(map[string]*workflow), states: make(map[string]WorkflowState)}
	if binder, ok := factory.(interface {
		BindCoordinator(interface {
			OnTransportError(string, error) bool
			OnClosed(string, int, string) bool
		})
	}); ok {
		binder.BindCoordinator(c)
	}
	return c
}
func (c *RoomSnapshotCoordinator) Submit(request RoomSnapshotRequest) error {
	if c.factory == nil || c.parser == nil {
		return errors.New("coordinator dependencies cannot be nil")
	}
	if err := request.validate(); err != nil {
		return err
	}
	w := &workflow{coordinator: c, request: request, state: StatePending}
	c.mu.Lock()
	if _, exists := c.active[request.WorkflowID]; exists {
		c.mu.Unlock()
		return fmt.Errorf("workflow %q already active", request.WorkflowID)
	}
	c.active[request.WorkflowID], c.states[request.WorkflowID] = w, StatePending
	c.mu.Unlock()

	session, err := c.factory.Create(request, SnapshotSink(func(payload string) { w.receive(payload) }))
	if err == nil && session == nil {
		err = errors.New("session factory returned nil session")
	}
	if err == nil {
		w.session = session
		w.setState(StateRunning)
		w.timer = time.AfterFunc(c.timeout, func() { w.fail(StateTimedOut, errors.New("snapshot workflow timed out")) })
		err = session.Start()
	}
	if err != nil {
		w.fail(StateFailed, err)
		return err
	}
	return nil
}
func (c *RoomSnapshotCoordinator) OnSnapshot(sessionID, payload string) bool {
	w := c.find(sessionID)
	return w != nil && w.receive(payload)
}
func (c *RoomSnapshotCoordinator) OnTransportError(sessionID string, err error) bool {
	w := c.find(sessionID)
	return w != nil && w.fail(StateFailed, err)
}
func (c *RoomSnapshotCoordinator) OnClosed(sessionID string, _ int, reason string) bool {
	w := c.find(sessionID)
	return w != nil && w.fail(StateFailed, errors.New(reason))
}
func (c *RoomSnapshotCoordinator) Cancel(workflowID, reason string) bool {
	c.mu.Lock()
	w := c.active[workflowID]
	c.mu.Unlock()
	return w != nil && w.fail(StateCancelled, errors.New(reason))
}
func (c *RoomSnapshotCoordinator) State(workflowID string) WorkflowState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.states[workflowID]
}
func (c *RoomSnapshotCoordinator) ActiveWorkflowCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.active)
}
func (c *RoomSnapshotCoordinator) find(sessionID string) *workflow {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range c.active {
		if w.session != nil && w.session.ID() == sessionID {
			return w
		}
	}
	return nil
}
func (w *workflow) setState(state WorkflowState) {
	w.mu.Lock()
	w.state = state
	w.mu.Unlock()
	w.coordinator.mu.Lock()
	w.coordinator.states[w.request.WorkflowID] = state
	w.coordinator.mu.Unlock()
}
func (w *workflow) receive(payload string) bool {
	w.mu.Lock()
	if w.state != StateRunning {
		w.mu.Unlock()
		return false
	}
	w.mu.Unlock()
	snapshot, err := w.coordinator.parser(payload)
	if err != nil {
		return w.fail(StateFailed, err)
	}
	w.mu.Lock()
	if w.state != StateRunning {
		w.mu.Unlock()
		return false
	}
	w.state = StateCompleted
	w.mu.Unlock()
	w.coordinator.mu.Lock()
	w.coordinator.states[w.request.WorkflowID] = StateCompleted
	delete(w.coordinator.active, w.request.WorkflowID)
	w.coordinator.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	ctx := RoomSnapshotContext{WorkflowID: w.request.WorkflowID, Author: w.request.Author, SourceChannel: w.request.SourceChannel, TargetChannel: w.request.TargetChannel, DestinationChannel: w.request.DestinationChannel, Reply: func(s string) {
		if w.coordinator.reply != nil {
			w.coordinator.reply(w.request, s)
		}
	}, SendRaw: func(s string) error { return w.session.SendRaw(s) }}
	result, opErr := w.request.Operation.Apply(ctx, snapshot)
	if opErr != nil {
		w.mu.Lock()
		w.state = StateFailed
		w.mu.Unlock()
		w.coordinator.mu.Lock()
		w.coordinator.states[w.request.WorkflowID] = StateFailed
		w.coordinator.mu.Unlock()
		result = Failed()
		if w.coordinator.outcome != nil {
			w.coordinator.outcome(w.request, result)
		}
		w.publishFailure()
	} else {
		w.publish(result)
		_ = w.session.Flush()
	}
	_ = w.session.Close()
	return true
}
func (w *workflow) fail(state WorkflowState, err error) bool {
	w.mu.Lock()
	if w.state == StateCompleted || w.state == StateFailed || w.state == StateCancelled || w.state == StateTimedOut {
		w.mu.Unlock()
		return false
	}
	w.state = state
	w.mu.Unlock()
	w.coordinator.mu.Lock()
	w.coordinator.states[w.request.WorkflowID] = state
	delete(w.coordinator.active, w.request.WorkflowID)
	w.coordinator.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	if w.coordinator.reply != nil && w.request.ReplyMessage != "" {
		w.coordinator.reply(w.request, "Unable to complete room operation.")
	}
	if w.coordinator.outcome != nil {
		w.coordinator.outcome(w.request, OperationResult{Outcome: OutcomeFailed, Reply: errString(err)})
	}
	if w.session != nil {
		_ = w.session.Close()
	}
	return true
}
func (w *workflow) publish(result OperationResult) {
	if w.coordinator.outcome != nil {
		w.coordinator.outcome(w.request, result)
	}
	if result.Reply != "" && w.coordinator.reply != nil {
		w.coordinator.reply(w.request, result.Reply)
	}
}
func (w *workflow) publishFailure() {
	if w.coordinator.reply != nil && w.request.ReplyMessage != "" {
		w.coordinator.reply(w.request, "Unable to complete room operation.")
	}
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
