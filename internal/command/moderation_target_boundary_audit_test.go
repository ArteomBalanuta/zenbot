package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/transport"
)

// Keep real core serialization at the last boundary; the ordinary command fake
// alone cannot detect a second normalization after a target has been resolved.
type targetBoundaryAuditEngine struct {
	*commandEngineStub
	wire         *core.EngineImpl
	afterLookup  func()
	operationErr error
}

func (e *targetBoundaryAuditEngine) GetActiveUserByName(name string) *model.User {
	user := e.commandEngineStub.GetActiveUserByName(name)
	if e.afterLookup != nil {
		e.afterLookup()
		e.afterLookup = nil
	}
	return user
}

func (e *targetBoundaryAuditEngine) KickNick(ctx context.Context, target common.NickTarget) error {
	return e.wire.KickNick(ctx, target)
}
func (e *targetBoundaryAuditEngine) BanNick(ctx context.Context, target common.NickTarget) error {
	if e.operationErr != nil {
		return e.operationErr
	}
	return e.wire.BanNick(ctx, target)
}
func (e *targetBoundaryAuditEngine) OverflowNick(ctx context.Context, target common.NickTarget) error {
	if e.operationErr != nil {
		return e.operationErr
	}
	return e.wire.OverflowNick(ctx, target)
}

func newTargetBoundaryAuditEngine(users map[string]*model.User) *targetBoundaryAuditEngine {
	return &targetBoundaryAuditEngine{
		commandEngineStub: &commandEngineStub{users: users},
		wire:              &core.EngineImpl{OutMessageQueue: make(chan string, 8)},
	}
}

func targetBoundaryAuditNicks(t *testing.T, engine *targetBoundaryAuditEngine) []string {
	return targetBoundaryAuditWireNicks(t, engine.wire)
}

func targetBoundaryAuditWireNicks(t *testing.T, engine *core.EngineImpl) []string {
	t.Helper()
	var nicks []string
	for len(engine.OutMessageQueue) > 0 {
		var payload struct {
			Nick string `json:"nick"`
		}
		if err := json.Unmarshal([]byte(<-engine.OutMessageQueue), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Nick != "" {
			nicks = append(nicks, payload.Nick)
		}
	}
	return nicks
}

type targetBoundaryAuditManagedEngine struct {
	*core.EngineImpl
	afterLookup func()
}

func (e *targetBoundaryAuditManagedEngine) StartContext(context.Context) error { return nil }
func (e *targetBoundaryAuditManagedEngine) StopContext(context.Context) error  { return nil }
func (e *targetBoundaryAuditManagedEngine) GetActiveUserByName(name string) *model.User {
	user := e.EngineImpl.GetActiveUserByName(name)
	if e.afterLookup != nil {
		e.afterLookup()
		e.afterLookup = nil
	}
	return user
}

func newTargetBoundaryAuditWire(channel string, names ...string) *core.EngineImpl {
	users := make(map[*model.User]struct{}, len(names))
	for _, name := range names {
		users[&model.User{Name: name}] = struct{}{}
	}
	return &core.EngineImpl{Channel: channel, Prefix: "!", ActiveUsers: users, OutMessageQueue: make(chan string, 8)}
}

func installTargetBoundaryAuditReplica(t *testing.T, host *core.EngineImpl, replica *targetBoundaryAuditManagedEngine) {
	t.Helper()
	manager := core.NewReplicaManager(host.Channel)
	controller := core.NewManagedReplicaController(manager, func(context.Context, string) (core.ManagedEngine, error) {
		return replica, nil
	})
	host.SetReplicaController(controller)
	if err := controller.AddReplica(context.Background(), replica.Channel); err != nil {
		t.Fatal(err)
	}
}

func executeTargetBoundaryAuditMove(t *testing.T, ctx context.Context, engine common.Engine, text string) (model.Status, error) {
	t.Helper()
	definition, ok := commandDefinitionFor("move")
	if !ok {
		t.Fatal("move definition is missing")
	}
	return definition.New(engine, &model.ChatMessage{Name: "mod", Text: text}).Execute(ctx)
}

type targetBoundaryAuditFailingTransport struct {
	calls int
	err   error
}

func (t *targetBoundaryAuditFailingTransport) Start(context.Context) error { return nil }
func (t *targetBoundaryAuditFailingTransport) Messages() <-chan transport.InboundMessage {
	return nil
}
func (t *targetBoundaryAuditFailingTransport) Errors() <-chan error        { return nil }
func (t *targetBoundaryAuditFailingTransport) Connected() bool             { return true }
func (t *targetBoundaryAuditFailingTransport) Close(context.Context) error { return nil }
func (t *targetBoundaryAuditFailingTransport) SendRaw(context.Context, []byte) error {
	return nil
}
func (t *targetBoundaryAuditFailingTransport) SendText(context.Context, string) error {
	t.calls++
	return t.err
}

func TestModerationTargetBoundaryAuditCanonicalNameSurvivesCore(t *testing.T) {
	for _, input := range []string{"!kick @@alice", "!kick -c @alice"} {
		t.Run(input, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{
				"alice": {Name: "alice", Hash: "plain"}, "@alice": {Name: "@alice", Hash: "marked"},
			})
			definition, _ := commandDefinitionFor("kick")
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: input}).Execute(context.Background())
			nicks := targetBoundaryAuditNicks(t, engine)
			if status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "@alice" {
				t.Fatalf("resolved identity changed before transport: status=%s err=%v nicks=%v", status, err, nicks)
			}
		})
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowResolveActiveCanonicalUser(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		t.Run(canonical, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " @merc"}).Execute(context.Background())
			nicks := targetBoundaryAuditNicks(t, engine)
			if status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "Merc" {
				t.Fatalf("active canonical target not retained: status=%s err=%v nicks=%v", status, err, nicks)
			}
		})
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowRejectAbsentOrExtraTargets(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		for _, arguments := range []string{"absent", "Merc another"} {
			t.Run(canonical+"/"+arguments, func(t *testing.T) {
				engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
				definition, _ := commandDefinitionFor(canonical)
				status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " " + arguments}).Execute(context.Background())
				if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || len(nicks) != 0 {
					t.Fatalf("invalid selection sent a request: status=%s err=%v nicks=%v", status, err, nicks)
				}
			})
		}
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowHonorCancellationAroundLookup(t *testing.T) {
	for _, canonical := range []string{"ban", "overflow"} {
		for _, phase := range []string{"before lookup", "after lookup"} {
			t.Run(canonical+"/"+phase, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
				if phase == "before lookup" {
					cancel()
				} else {
					engine.afterLookup = cancel
				}
				definition, _ := commandDefinitionFor(canonical)
				status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " @Merc"}).Execute(ctx)
				if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || !errors.Is(err, context.Canceled) || len(nicks) != 0 || len(engine.chats) != 0 {
					t.Fatalf("cancelled selection had effects: status=%s err=%v nicks=%v chats=%v", status, err, nicks, engine.chats)
				}
			})
		}
	}
}

func TestModerationTargetBoundaryAuditBanAndOverflowPropagateSendFailure(t *testing.T) {
	sendErr := errors.New("send failed")
	for _, canonical := range []string{"ban", "overflow"} {
		t.Run(canonical, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{"Merc": {Name: "Merc"}})
			engine.operationErr = sendErr
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " Merc"}).Execute(context.Background())
			if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || !errors.Is(err, sendErr) || len(nicks) != 0 || len(engine.chats) != 0 {
				t.Fatalf("send failure fabricated confirmation: status=%s err=%v nicks=%v chats=%v", status, err, nicks, engine.chats)
			}
		})
	}
}

func TestModerationTargetBoundaryAuditMoveResolvesCanonicalUserOnSelectedSource(t *testing.T) {
	t.Run("host", func(t *testing.T) {
		host := newTargetBoundaryAuditWire("host", "@Merc")
		status, err := executeTargetBoundaryAuditMove(t, context.Background(), host, "!move @@merc host destination")
		if nicks := targetBoundaryAuditWireNicks(t, host); status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "@Merc" {
			t.Fatalf("host canonical target changed: status=%s err=%v nicks=%v", status, err, nicks)
		}
	})

	t.Run("managed replica", func(t *testing.T) {
		host := newTargetBoundaryAuditWire("host")
		replica := &targetBoundaryAuditManagedEngine{EngineImpl: newTargetBoundaryAuditWire("source", "@Merc")}
		installTargetBoundaryAuditReplica(t, host, replica)
		status, err := executeTargetBoundaryAuditMove(t, context.Background(), host, "!move @@merc source destination")
		if nicks := targetBoundaryAuditWireNicks(t, replica.EngineImpl); status != model.SUCCESSFUL || err != nil || len(nicks) != 1 || nicks[0] != "@Merc" || len(host.OutMessageQueue) != 0 {
			t.Fatalf("replica canonical target changed: status=%s err=%v nicks=%v host=%d", status, err, nicks, len(host.OutMessageQueue))
		}
	})
}

func TestModerationTargetBoundaryAuditMoveRejectsMissingUserOnOwnedSource(t *testing.T) {
	for _, source := range []string{"host", "source"} {
		t.Run(source, func(t *testing.T) {
			host := newTargetBoundaryAuditWire("host", "Merc")
			wire := host
			if source == "source" {
				replica := &targetBoundaryAuditManagedEngine{EngineImpl: newTargetBoundaryAuditWire("source", "Merc")}
				installTargetBoundaryAuditReplica(t, host, replica)
				wire = replica.EngineImpl
			}
			status, err := executeTargetBoundaryAuditMove(t, context.Background(), host, "!move absent "+source+" destination")
			if nicks := targetBoundaryAuditWireNicks(t, wire); status != model.FAILED || !errors.Is(err, repository.ErrNotFound) || len(nicks) != 0 || len(host.OutMessageQueue) != 0 {
				t.Fatalf("missing owned-source user escaped to a request: status=%s err=%v nicks=%v host=%d", status, err, nicks, len(host.OutMessageQueue))
			}
		})
	}
}

func TestModerationTargetBoundaryAuditMoveHonorsCancellationAfterSourceLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	host := newTargetBoundaryAuditWire("host")
	replica := &targetBoundaryAuditManagedEngine{EngineImpl: newTargetBoundaryAuditWire("source", "Merc"), afterLookup: cancel}
	installTargetBoundaryAuditReplica(t, host, replica)
	status, err := executeTargetBoundaryAuditMove(t, ctx, host, "!move @merc source destination")
	if nicks := targetBoundaryAuditWireNicks(t, replica.EngineImpl); status != model.FAILED || !errors.Is(err, context.Canceled) || len(nicks) != 0 || len(host.OutMessageQueue) != 0 {
		t.Fatalf("post-lookup cancellation emitted a move: status=%s err=%v nicks=%v host=%d", status, err, nicks, len(host.OutMessageQueue))
	}
}

func TestModerationTargetBoundaryAuditMovePropagatesTransportFailure(t *testing.T) {
	sendErr := errors.New("send failed")
	transport := &targetBoundaryAuditFailingTransport{err: sendErr}
	host := newTargetBoundaryAuditWire("host", "Merc")
	host.Transport = transport
	status, err := executeTargetBoundaryAuditMove(t, context.Background(), host, "!move @merc host destination")
	if status != model.FAILED || !errors.Is(err, sendErr) || transport.calls != 1 || len(host.OutMessageQueue) != 0 {
		t.Fatalf("transport failure changed: status=%s err=%v calls=%d queued=%d", status, err, transport.calls, len(host.OutMessageQueue))
	}
}

func TestModerationTargetBoundaryAuditRawBlankAndBareMarkerSendNothing(t *testing.T) {
	for _, test := range []struct {
		name, canonical, text string
	}{
		{name: "ban blank", canonical: "ban", text: "!ban"},
		{name: "ban bare marker", canonical: "ban", text: "!ban @"},
		{name: "overflow blank", canonical: "overflow", text: "!overflow"},
		{name: "overflow bare marker", canonical: "overflow", text: "!overflow @"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := newTargetBoundaryAuditEngine(map[string]*model.User{"@": {Name: "@"}})
			definition, _ := commandDefinitionFor(test.canonical)
			status, _ := definition.New(engine, &model.ChatMessage{Name: "mod", Text: test.text}).Execute(context.Background())
			if nicks := targetBoundaryAuditNicks(t, engine); status != model.FAILED || len(nicks) != 0 {
				t.Fatalf("invalid raw target sent a request: status=%s nicks=%v", status, nicks)
			}
		})
	}

	for _, text := range []string{"!move source destination", "!move @ host destination"} {
		t.Run(text, func(t *testing.T) {
			host := newTargetBoundaryAuditWire("host", "@")
			status, _ := executeTargetBoundaryAuditMove(t, context.Background(), host, text)
			if nicks := targetBoundaryAuditWireNicks(t, host); status != model.FAILED || len(nicks) != 0 {
				t.Fatalf("invalid move target sent a request: status=%s nicks=%v", status, nicks)
			}
		})
	}
}
