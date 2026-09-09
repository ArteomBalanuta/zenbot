package live

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/model"
)

type capturedAgentService struct {
	calls int
	inv   api.Invocation
}

func (s *capturedAgentService) Submit(inv api.Invocation) error {
	s.calls++
	s.inv = inv
	return nil
}
func (s *capturedAgentService) Close() {}

type recordingInvocationFactory struct {
	creates int
}

func (f *recordingInvocationFactory) Create(participation.TrustedSnapshot, model.ChatMessage, string, api.InvocationMode, bool) (api.Invocation, error) {
	f.creates++
	return api.Invocation{}, nil
}

func TestDirectSubmissionAdapterCreatesTrustedDirectInvocation(t *testing.T) {
	users := []string{"alice", "bob"}
	service := &capturedAgentService{}
	adapter := DirectSubmissionAdapter{
		Service: service,
		Factory: participation.NewInvocationFactory(nil),
		Snapshot: func() participation.TrustedSnapshot {
			return participation.TrustedSnapshot{Room: "room", Users: users, CreatorTrip: "creator"}
		},
	}
	message := &model.ChatMessage{Name: "alice", Trip: "creator", Hash: "hash", Whisper: true, Text: "!l question"}
	if err := adapter.Submit(context.Background(), message, "question"); err != nil {
		t.Fatal(err)
	}
	users[0] = "changed"
	if service.calls != 1 {
		t.Fatalf("submissions=%d", service.calls)
	}
	inv := service.inv
	if inv.Mode() != api.DIRECT || !inv.CommandOriginated() || inv.Prompt() != "question" {
		t.Fatalf("invocation=%#v", inv)
	}
	ctx := inv.Context()
	if ctx.Room() != "room" || ctx.Nick() != "alice" || *ctx.Trip() != "creator" || *ctx.Hash() != "hash" || !ctx.Whisper() {
		t.Fatalf("context=%#v", ctx)
	}
	if got := ctx.RoomUsers(); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("users=%v", got)
	}
	if !ctx.HasCapability(api.DynamicSQL) || !ctx.HasCapability(api.ModerationCommands) || !ctx.HasCapability(api.PermanentBan) || !ctx.HasCapability(api.AdminCommands) {
		t.Fatalf("capabilities=%v", ctx.Capabilities())
	}
}

func TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name       string
		ctx        context.Context
		message    *model.ChatMessage
		prompt     string
		room       string
		nilService bool
	}{
		{name: "cancelled context", ctx: cancelled, message: &model.ChatMessage{Name: "alice"}, prompt: "question", room: "room"},
		{name: "nil message", ctx: context.Background(), prompt: "question", room: "room"},
		{name: "blank prompt", ctx: context.Background(), message: &model.ChatMessage{Name: "alice"}, prompt: " 	\n ", room: "room"},
		{name: "blank trusted room", ctx: context.Background(), message: &model.ChatMessage{Name: "alice"}, prompt: "question", room: " 	\n "},
		{name: "nil service", ctx: context.Background(), message: &model.ChatMessage{Name: "alice"}, prompt: "question", room: "room", nilService: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory := &recordingInvocationFactory{}
			service := &capturedAgentService{}
			adapter := DirectSubmissionAdapter{
				Service: service,
				Factory: factory,
				Snapshot: func() participation.TrustedSnapshot {
					return participation.TrustedSnapshot{Room: tc.room}
				},
			}
			if tc.nilService {
				adapter.Service = nil
			}

			if err := adapter.Submit(tc.ctx, tc.message, tc.prompt); err == nil {
				t.Fatal("expected invalid input to be rejected")
			}
			if factory.creates != 0 {
				t.Fatalf("factory creations=%d", factory.creates)
			}
			if service.calls != 0 {
				t.Fatalf("service submissions=%d", service.calls)
			}
		})
	}
}

type directRuntimeOrderingRunner struct {
	result  runtime.Result
	started chan struct{}
	after   chan struct{}
	mu      sync.Mutex
	order   []string
	runs    int
}

func (r *directRuntimeOrderingRunner) Run(context.Context, runtime.Invocation) (runtime.Result, error) {
	r.mu.Lock()
	r.runs++
	r.mu.Unlock()
	close(r.started)
	return r.result, nil
}

func (r *directRuntimeOrderingRunner) AfterDelivery(context.Context, runtime.Invocation, runtime.Result) error {
	r.mu.Lock()
	r.order = append(r.order, "after-delivery")
	r.mu.Unlock()
	close(r.after)
	return nil
}

func (r *directRuntimeOrderingRunner) recorded() ([]string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...), r.runs
}

func TestAcceptedDirectSubmissionRunsUnderRuntimeContextAfterCallerCancellation(t *testing.T) {
	caller, cancelCaller := context.WithCancel(context.Background())
	defer cancelCaller()
	started := make(chan context.Context, 1)
	rt, err := runtime.New(runtime.Config{MaxConcurrent: 1, QueueCapacity: 0}, runtime.RunnerFunc(func(ctx context.Context, _ runtime.Invocation) (runtime.Result, error) {
		started <- ctx
		<-ctx.Done()
		return runtime.Result{}, ctx.Err()
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	service := RuntimeService{Runtime: rt}
	adapter := DirectSubmissionAdapter{Service: service, Factory: participation.NewInvocationFactory(nil), Snapshot: func() participation.TrustedSnapshot {
		return participation.TrustedSnapshot{Room: "room", Users: []string{"alice"}}
	}}
	if err := adapter.Submit(caller, &model.ChatMessage{Name: "alice", Text: "!l question"}, "question"); err != nil {
		rt.Close()
		t.Fatal(err)
	}
	var runtimeCtx context.Context
	select {
	case runtimeCtx = <-started:
	case <-time.After(time.Second):
		rt.Close()
		t.Fatal("accepted direct invocation did not start")
	}
	cancelCaller()
	if err := runtimeCtx.Err(); err != nil {
		rt.Close()
		t.Fatalf("caller cancellation leaked into accepted runtime work: %v", err)
	}
	rt.Close()
	if err := runtimeCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("runtime close did not cancel accepted work: %v", err)
	}
}

func TestDirectSubmissionAdapterDefersPersistenceToRuntimeDelivery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    runtime.Result
		sinkErr   error
		wantAfter bool
		wantOrder []string
	}{
		{name: "visible reply", result: runtime.NewResult("direct-visible", "answer", true), wantAfter: true, wantOrder: []string{"sink", "after-delivery"}},
		{name: "sink failure", result: runtime.NewResult("direct-sink-failure", "answer", true), sinkErr: errors.New("sink failure"), wantOrder: []string{"sink"}},
		{name: "no reply", result: runtime.NewResult("direct-no-reply", "", false), wantOrder: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &directRuntimeOrderingRunner{result: tc.result, started: make(chan struct{}), after: make(chan struct{})}
			rt, err := runtime.New(runtime.Config{MaxConcurrent: 1, QueueCapacity: 0}, runner, runtime.SinkFunc(func(context.Context, runtime.Invocation, runtime.Result) error {
				runner.mu.Lock()
				runner.order = append(runner.order, "sink")
				runner.mu.Unlock()
				return tc.sinkErr
			}))
			if err != nil {
				t.Fatal(err)
			}
			service := RuntimeService{Runtime: rt}
			adapter := DirectSubmissionAdapter{Service: service, Factory: participation.NewInvocationFactory(nil), Snapshot: func() participation.TrustedSnapshot {
				return participation.TrustedSnapshot{Room: "room", Users: []string{"alice"}}
			}}
			if err := adapter.Submit(context.Background(), &model.ChatMessage{Name: "alice", Text: "!l question"}, "question"); err != nil {
				rt.Close()
				t.Fatal(err)
			}
			select {
			case <-runner.started:
			case <-time.After(time.Second):
				rt.Close()
				t.Fatal("direct invocation did not reach the runtime")
			}
			if tc.wantAfter {
				select {
				case <-runner.after:
				case <-time.After(time.Second):
					rt.Close()
					t.Fatal("successful direct delivery did not reach runtime post-delivery")
				}
			}
			rt.Close()
			order, runs := runner.recorded()
			if runs != 1 {
				t.Fatalf("runtime runs=%d, want 1", runs)
			}
			if len(order) != len(tc.wantOrder) {
				t.Fatalf("delivery ordering=%v, want %v", order, tc.wantOrder)
			}
			for i := range order {
				if order[i] != tc.wantOrder[i] {
					t.Fatalf("delivery ordering=%v, want %v", order, tc.wantOrder)
				}
			}
		})
	}
}
