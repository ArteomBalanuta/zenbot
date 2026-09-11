package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type TemporaryJoin struct {
	Channel  string
	Nick     string
	Password string
}

type RoomSnapshotRequest struct {
	// Context owns temporary construction, execution, and cancellation. Nil is
	// retained only for legacy callers and means context.Background().
	Context            context.Context
	onTransportError   func(error)
	WorkflowID         string
	Author             string
	Whisper            bool
	SourceChannel      string
	TargetChannel      string
	DestinationChannel string
	ReplyMessage       string
	RemoteMessage      string
	TemporaryJoin      *TemporaryJoin
	Operation          RoomSnapshotOperation
	// OnComplete observes the terminal operation outcome for callers that must
	// consume the result. It runs after any room reply has been published.
	OnComplete func(OperationResult)
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
	Context                                                              context.Context
	WorkflowID, Author, SourceChannel, TargetChannel, DestinationChannel string
	Reply                                                                func(string)
	SendRaw                                                              func(string) error
}

type RoomSnapshotOperation interface {
	Apply(RoomSnapshotContext, Snapshot) (OperationResult, error)
}

func executionContext(ctx RoomSnapshotContext) context.Context {
	if ctx.Context == nil {
		return context.Background()
	}
	return ctx.Context
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
	Data    json.RawMessage
	// Counts describe successful outward writes, including delivered replies.
	// Temporary-session join frames are setup and never contribute.
	ActionCount   int
	DeliveryCount int
	// OutcomeUnknown marks ambiguous writes/flushes or an interrupted partial
	// action. Error retains the operation and cleanup causes independently.
	OutcomeUnknown bool
	Error          error
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

type ReplySink func(RoomSnapshotRequest, string) error
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
	factory  SessionFactory
	reply    ReplySink
	outcome  OutcomeSink
	parser   SnapshotParser
	timeout  time.Duration
	mu       sync.Mutex
	active   map[string]*workflow
	states   map[string]WorkflowState
	retained []string
}
type workflow struct {
	coordinator  *RoomSnapshotCoordinator
	request      RoomSnapshotRequest
	session      Session
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	payload      chan string
	done         chan struct{}
	claimed      bool
	terminal     bool
	failureState WorkflowState
	failure      error
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
	parent := request.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	w := &workflow{coordinator: c, request: request, ctx: ctx, cancel: cancel, payload: make(chan string, 1), done: make(chan struct{})}
	w.request.Context = ctx
	w.request.onTransportError = func(err error) { w.fail(StateFailed, err) }
	c.mu.Lock()
	if _, exists := c.active[request.WorkflowID]; exists {
		c.mu.Unlock()
		cancel()
		return fmt.Errorf("workflow %q already active", request.WorkflowID)
	}
	c.active[request.WorkflowID], c.states[request.WorkflowID] = w, StatePending
	c.mu.Unlock()

	started := make(chan error, 1)
	go w.run(started)
	return <-started
}
func (c *RoomSnapshotCoordinator) OnSnapshot(sessionID, payload string) bool {
	w := c.find(sessionID)
	if w == nil || !w.receive(payload) {
		return false
	}
	<-w.done
	return true
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
		w.mu.Lock()
		matches := w.session != nil && w.session.ID() == sessionID
		w.mu.Unlock()
		if matches {
			return w
		}
	}
	return nil
}

// receive only queues work: it may be called synchronously during Start or by
// the transport reader. The execution owner never runs on either callback.
func (w *workflow) receive(payload string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.claimed || w.terminal || w.ctx.Err() != nil {
		return false
	}
	w.claimed = true
	w.payload <- payload
	return true
}
func (w *workflow) fail(state WorkflowState, err error) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.terminal || w.failure != nil {
		return false
	}
	if err == nil {
		err = errors.New("snapshot transport failed")
	}
	w.failureState, w.failure = state, err
	w.cancel()
	return true
}

func (w *workflow) run(started chan<- error) {
	result := Success()
	var session Session
	startSent := false
	actions, deliveries := 0, 0
	unknown := false
	var actionErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			result.Outcome = OutcomeFailed
			result.Error = fmt.Errorf("snapshot operation panic: %v", recovered)
			unknown = true
		}
		// Boundary counts also survive a panic before the operation returns.
		result.ActionCount = actions
		result.DeliveryCount += deliveries
		result.OutcomeUnknown = result.OutcomeUnknown || unknown
		result.Error = errors.Join(result.Error, actionErr)
		if session != nil {
			if err := session.Flush(); err != nil {
				result.Error = errors.Join(result.Error, err)
				result.OutcomeUnknown = true
			}
			if err := session.Close(); err != nil {
				result.Error = errors.Join(result.Error, err)
			}
		}
		w.mu.Lock()
		failure, state := w.failure, w.failureState
		w.terminal = true
		w.mu.Unlock()
		if failure == nil && w.ctx.Err() != nil {
			failure = w.ctx.Err()
			state = StateCancelled
			if errors.Is(failure, context.DeadlineExceeded) {
				state = StateTimedOut
			}
		}
		result.Error = errors.Join(result.Error, failure)
		if failure != nil && actions > 0 {
			result.OutcomeUnknown = true
		}
		if result.Error != nil {
			if result.Outcome != OutcomeFailed {
				result.Reply = errString(result.Error)
			}
			result.Outcome = OutcomeFailed
		}
		if result.Outcome == OutcomeFailed {
			if state == "" {
				state = StateFailed
			}
			if result.Reply == "" {
				result.Reply = errString(result.Error)
			}
		} else {
			state = StateCompleted
		}
		reply := result.Reply
		if result.Outcome == OutcomeFailed {
			reply = w.request.ReplyMessage
		}
		if reply != "" && w.coordinator.reply != nil {
			if err := w.coordinator.reply(w.request, reply); err != nil {
				result.Error = errors.Join(result.Error, err)
				result.Outcome, result.OutcomeUnknown = OutcomeFailed, true
				state = StateFailed
			} else {
				result.DeliveryCount++
				result.ActionCount++
			}
		}
		w.cancel()
		c := w.coordinator
		c.mu.Lock()
		delete(c.active, w.request.WorkflowID)
		c.states[w.request.WorkflowID] = state
		// Retain at most 256 terminal outcomes; active workflows remain queryable.
		for i, id := range c.retained {
			if id == w.request.WorkflowID {
				c.retained = append(c.retained[:i], c.retained[i+1:]...)
				break
			}
		}
		c.retained = append(c.retained, w.request.WorkflowID)
		if len(c.retained) > 256 {
			id := c.retained[0]
			c.retained = c.retained[1:]
			if c.active[id] == nil {
				delete(c.states, id)
			}
		}
		c.mu.Unlock()
		if c.outcome != nil {
			c.outcome(w.request, cloneOperationResult(result))
		}
		if w.request.OnComplete != nil {
			w.request.OnComplete(cloneOperationResult(result))
		}
		close(w.done)
		if !startSent {
			started <- result.Error
		}
	}()
	var err error
	session, err = w.coordinator.factory.Create(w.request, func(payload string) { w.receive(payload) })
	if err == nil && session == nil {
		err = errors.New("session factory returned nil session")
	}
	w.mu.Lock()
	w.session = session
	w.mu.Unlock()
	if err == nil {
		err = w.ctx.Err()
	}
	if err == nil {
		w.coordinator.mu.Lock()
		w.coordinator.states[w.request.WorkflowID] = StateRunning
		w.coordinator.mu.Unlock()
		err = session.Start()
	}
	if err != nil {
		result.Error = err
		return
	}
	started <- nil
	startSent = true
	select {
	case <-w.ctx.Done():
		return
	case payload := <-w.payload:
		if err = w.ctx.Err(); err != nil {
			result.Error = err
			return
		}
		parsed, parseErr := w.coordinator.parser(payload)
		if parseErr != nil {
			result.Error = parseErr
			return
		}
		operationContext := RoomSnapshotContext{Context: w.ctx, WorkflowID: w.request.WorkflowID, Author: w.request.Author, SourceChannel: w.request.SourceChannel, TargetChannel: w.request.TargetChannel, DestinationChannel: w.request.DestinationChannel}
		operationContext.SendRaw = func(raw string) error {
			if err := w.ctx.Err(); err != nil {
				return err
			}
			err := session.SendRaw(raw)
			if err != nil {
				unknown = true
				actionErr = errors.Join(actionErr, err)
			} else {
				actions++
			}
			return err
		}
		operationContext.Reply = func(reply string) {
			if w.coordinator.reply == nil {
				return
			}
			if err := w.coordinator.reply(w.request, reply); err != nil {
				unknown = true
				actionErr = errors.Join(actionErr, err)
			} else {
				deliveries++
				actions++
			}
		}
		result, err = w.request.Operation.Apply(operationContext, parsed)
		result.Error = errors.Join(result.Error, err)
	}
}

func cloneOperationResult(result OperationResult) OperationResult {
	result.Data = append(json.RawMessage(nil), result.Data...)
	return result
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
