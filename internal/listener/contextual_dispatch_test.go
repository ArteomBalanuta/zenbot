package listener_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"zenbot/internal/command"
	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/listener/info"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
)

type contextualResultEngine struct {
	*lifecycleCommandEngine
	replyErr error
	auditErr error
	auditCtx context.Context
	audits   []model.CommandAuditRecord
	onAudit  func()
	worker   *core.HostLifecycle
}

func (e *contextualResultEngine) SendChatMessage(string, string, bool) (string, error) {
	e.chats++
	return "", e.replyErr
}
func (e *contextualResultEngine) RegisterCommand(c common.Command) error {
	for _, alias := range c.GetAliases() {
		e.commands[alias] = common.CommandMetadata{Alias: alias, Command: func(m *model.ChatMessage) common.Command { return c.NewInstance(e, m) }}
	}
	return nil
}
func (e *contextualResultEngine) LogCommand(ctx context.Context, record model.CommandAuditRecord) (int64, error) {
	e.auditCtx = ctx
	e.audits = append(e.audits, record)
	if e.onAudit != nil {
		e.onAudit()
	}
	return 0, e.auditErr
}
func (e *contextualResultEngine) HostLifecycleController() common.HostLifecycleController {
	if e.worker != nil {
		return e.worker
	}
	return e.lifecycleCommandEngine.HostLifecycleController()
}
func (*contextualResultEngine) UpdatePrefix(...string) (string, error) { return "!", nil }

func newContextualResultEngine(t *testing.T) *contextualResultEngine {
	t.Helper()
	e := &contextualResultEngine{lifecycleCommandEngine: newLifecycleCommandEngine(true)}
	e.active[&model.User{Name: "alice", Trip: "trip"}] = struct{}{}
	if err := command.RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	return e
}

func dispatchResultChain(ctx context.Context, e *contextualResultEngine, text string, whisper bool) error {
	if whisper {
		return info.NewChain(info.ConvertWhisperToChatMessage{}, info.DispatchWhisperCommand{}).Process(ctx, &model.InfoMessage{From: "alice", Text: "alice whispered: " + text}, e)
	}
	return message.NewChain(message.ResolveUserMetadata{}, message.DispatchUserCommand{}).Process(ctx, &model.ChatMessage{Name: "alice", Trip: "trip", Text: text}, e)
}

func TestContextualDispatchRegisteredAdapterReturnsFailureOnce(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "whisper"}[whisper], func(t *testing.T) {
			e := newContextualResultEngine(t)
			failure := errors.New("private transport failure")
			e.replyErr = failure
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := dispatchResultChain(ctx, e, "!version", whisper)
			if !errors.Is(err, failure) || e.chats != 1 || len(e.audits) != 1 || e.audits[0].Status != "FAILED" || e.auditCtx != ctx {
				t.Fatalf("typed result lost: err=%v sends=%d audit=%+v ctx=%v", err, e.chats, e.audits, e.auditCtx)
			}
		})
	}
}

func TestContextualDispatchRegisteredAdapterRejectsFailedNilError(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		e := newContextualResultEngine(t)
		err := dispatchResultChain(context.Background(), e, "!prefix \x00", whisper)
		var rejection *common.CommandRejectedError
		if !errors.As(err, &rejection) || rejection.Status != model.FAILED || e.chats != 1 || len(e.audits) != 1 || e.audits[0].Status != "FAILED" {
			t.Fatalf("rejection reported as success: whisper=%t err=%v sends=%d audit=%+v", whisper, err, e.chats, e.audits)
		}
	}
}

func TestContextualDispatchAuditFailurePreservesExecutionStatus(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		e := newContextualResultEngine(t)
		e.auditErr = errors.New("audit storage unavailable")
		err := dispatchResultChain(context.Background(), e, "!version", whisper)
		if err != nil || e.chats != 1 || len(e.audits) != 1 || e.audits[0].Status != "SUCCESSFUL" {
			t.Fatalf("audit failure changed execution: whisper=%t err=%v sends=%d audit=%+v", whisper, err, e.chats, e.audits)
		}
	}
}

func TestContextualDispatchWhisperLifecycleWaitsThroughAudit(t *testing.T) {
	started := make(chan struct{})
	worker := core.NewHostLifecycle(func(context.Context) error { close(started); return nil }, nil)
	defer worker.Close()
	e := newContextualResultEngine(t)
	e.worker = worker
	e.onAudit = func() {
		select {
		case <-started:
			t.Error("restart ran before whispered command audit returned")
		case <-time.After(30 * time.Millisecond):
		}
	}
	if err := dispatchResultChain(context.Background(), e, "!restart", true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("restart did not run after dispatch returned")
	}
	worker.Wait()
}

func TestContextualDispatchUnauthorizedReplyFailureReachesChain(t *testing.T) {
	e := newContextualResultEngine(t)
	e.allowed = false
	e.replyErr = errors.New("reply unavailable")
	err := dispatchResultChain(context.Background(), e, "!version", false)
	if !errors.Is(err, e.replyErr) || e.chats != 1 || len(e.audits) != 0 {
		t.Fatalf("err=%v sends=%d audit=%v", err, e.chats, e.audits)
	}
}

func TestContextualDispatchReleasesLifecycleLeaseOnPanic(t *testing.T) {
	for _, whisper := range []bool{false, true} {
		started := make(chan struct{})
		worker := core.NewHostLifecycle(func(context.Context) error { close(started); return nil }, nil)
		e := newContextualResultEngine(t)
		e.worker = worker
		e.onAudit = func() { panic("audit panic") }
		func() {
			defer func() {
				if recover() != "audit panic" {
					t.Error("command panic was swallowed")
				}
			}()
			_ = dispatchResultChain(context.Background(), e, "!version", whisper)
		}()
		if err := worker.RequestRestart(context.Background()); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Error("dispatch lease leaked after panic")
		}
		worker.Close()
	}
}
