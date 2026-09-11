package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	agenttool "zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
)

type ownedSnapshotEngine struct {
	*gatewayEngine
	coordinator *snapshot.RoomSnapshotCoordinator
	operation   snapshot.RoomSnapshotOperation
}

func (e *ownedSnapshotEngine) SubmitCredentialedRoomSnapshot(req snapshot.RoomSnapshotRequest) error {
	if e.operation != nil {
		req.Operation = e.operation
	}
	return e.coordinator.Submit(req)
}
func (e *ownedSnapshotEngine) MoveFromServingRoom(context.Context, string, string, common.Channel) (bool, error) {
	return false, nil
}

type ownedSnapshotSession struct {
	ctx                        context.Context
	sink                       snapshot.SnapshotSink
	payload                    string
	sends, closed              int
	failAt                     int
	entered, release           chan struct{}
	closeEntered, closeRelease chan struct{}
}

func (s *ownedSnapshotSession) ID() string   { return "owned-snapshot" }
func (s *ownedSnapshotSession) Start() error { s.sink(s.payload); return nil }
func (s *ownedSnapshotSession) Flush() error { return nil }
func (s *ownedSnapshotSession) Close() error {
	if s.closeEntered != nil {
		close(s.closeEntered)
		<-s.closeRelease
	}
	s.closed++
	return nil
}
func (s *ownedSnapshotSession) SendRaw(string) error {
	s.sends++
	if s.entered != nil && s.sends == 2 {
		close(s.entered)
		<-s.ctx.Done()
		<-s.release
		return s.ctx.Err()
	}
	if s.failAt == s.sends {
		return errors.New("write outcome unknown")
	}
	return nil
}
func snapshotToolFixture(session *ownedSnapshotSession, replyErr error, operation snapshot.RoomSnapshotOperation) *ownedSnapshotEngine {
	coordinator := snapshot.NewRoomSnapshotCoordinator(snapshot.SessionFactoryFunc(func(req snapshot.RoomSnapshotRequest, sink snapshot.SnapshotSink) (snapshot.Session, error) {
		session.ctx, session.sink = req.Context, sink
		return session, nil
	}), func(snapshot.RoomSnapshotRequest, string) error { return replyErr }, func(raw string) (snapshot.Snapshot, error) { return snapshot.Parse(raw, false) }, time.Second)
	return &ownedSnapshotEngine{gatewayEngine: &gatewayEngine{commandEngineStub: commandEngineStub{users: map[string]*model.User{"caller": {Name: "caller"}}}, authorized: true}, coordinator: coordinator, operation: operation}
}
func executeSnapshotTool(t *testing.T, ctx context.Context, engine *ownedSnapshotEngine, name, args string) contract.Result {
	t.Helper()
	caller, err := api.NewContextWithCapabilities("programming", "caller", "trip", "hash", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan})
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := commandcatalog.AgentEntry(name)
	if !ok {
		t.Fatal(name)
	}
	result, err := (agenttool.SaturnCommand{Definition: definition, Gateway: NewAgentCommandGateway(engine)}).Execute(ctx, caller, json.RawMessage(args))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSnapshotToolIntegrationReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, command, args, payload, code string
		actions, deliveries, failAt        int
		replyErr                           error
	}{
		{name: "silent nuke", command: "nuke", args: `{"room":"lounge"}`, payload: `{"cmd":"onlineSet","users":[{"nick":"alice"},{"nick":"bob"}]}`, actions: 3},
		{name: "silent resurrect", command: "resurrect", args: `{"nick":"alice","source":"lounge","destination":"room"}`, payload: `{"cmd":"onlineSet","users":[{"nick":"ALIce"}]}`, actions: 1},
		{name: "absent resurrect", command: "resurrect", args: `{"nick":"alice","source":"lounge","destination":"room"}`, payload: `{"cmd":"onlineSet","users":[]}`, code: "NOT_FOUND", actions: 1, deliveries: 1},
		{name: "empty message room", command: "msgchannel", args: `{"room":"lounge","message":"hello"}`, payload: `{"cmd":"onlineSet","users":[]}`, code: "COMMAND_REJECTED", actions: 1, deliveries: 1},
		{name: "delivered remote message", command: "msgchannel", args: `{"room":"lounge","message":"hello"}`, payload: `{"cmd":"onlineSet","users":[{"nick":"alice"}]}`, actions: 2, deliveries: 2},
		{name: "partial nuke", command: "nuke", args: `{"room":"lounge"}`, payload: `{"cmd":"onlineSet","users":[{"nick":"alice"},{"nick":"bob"}]}`, code: "ACTION_OUTCOME_UNKNOWN", actions: 2, deliveries: 1, failAt: 2},
		{name: "failed reply", command: "list", args: `{"room":"lounge"}`, payload: `{"cmd":"onlineSet","users":[]}`, code: "ACTION_OUTCOME_UNKNOWN", replyErr: errors.New("reply failed")},
		{name: "empty list data", command: "list", args: `{"room":"lounge"}`, payload: `{"cmd":"onlineSet","users":[]}`, actions: 1, deliveries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &ownedSnapshotSession{payload: tc.payload, failAt: tc.failAt}
			var operation snapshot.RoomSnapshotOperation
			if tc.command == "nuke" {
				operation = snapshot.NewNukeRoomOperation(0)
			}
			engine := snapshotToolFixture(session, tc.replyErr, operation)
			r := executeSnapshotTool(t, context.Background(), engine, tc.command, tc.args)
			if r.ErrorCode != tc.code || r.ActionCount != tc.actions || r.DeliveryCount != tc.deliveries || session.closed != 1 {
				t.Fatalf("result=%+v closed=%d", r, session.closed)
			}
			if tc.name == "empty list data" && !strings.Contains(r.Content, `"count":0`) {
				t.Fatalf("list=%s", r.Content)
			}
		})
	}
}

func TestSnapshotToolCancellationWaitsForSendAndCloseAndRetainsPartialEvidence(t *testing.T) {
	session := &ownedSnapshotSession{payload: `{"cmd":"onlineSet","users":[{"nick":"alice"},{"nick":"bob"}]}`, entered: make(chan struct{}), release: make(chan struct{}), closeEntered: make(chan struct{}), closeRelease: make(chan struct{})}
	engine := snapshotToolFixture(session, nil, snapshot.NewNukeRoomOperation(0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan contract.Result, 1)
	go func() { done <- executeSnapshotTool(t, ctx, engine, "nuke", `{"room":"lounge"}`) }()
	<-session.entered
	cancel()
	select {
	case r := <-done:
		t.Fatalf("returned before send stopped: %+v", r)
	case <-time.After(20 * time.Millisecond):
	}
	close(session.release)
	<-session.closeEntered
	select {
	case r := <-done:
		t.Fatalf("returned before close stopped: %+v", r)
	case <-time.After(20 * time.Millisecond):
	}
	close(session.closeRelease)
	r := <-done
	if r.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || r.EffectState != contract.EffectUnknown || r.ActionCount != 2 || r.DeliveryCount != 1 || !strings.Contains(r.Content, "do not repeat") || session.sends != 2 || session.closed != 1 {
		t.Fatalf("result=%+v sends=%d closed=%d", r, session.sends, session.closed)
	}
}

func TestSnapshotToolPanicAfterWriteRetainsActionAndStopsOwner(t *testing.T) {
	session := &ownedSnapshotSession{payload: `{"cmd":"onlineSet","users":[]}`}
	operation := snapshot.OperationFunc(func(ctx snapshot.RoomSnapshotContext, _ snapshot.Snapshot) (snapshot.OperationResult, error) {
		_ = ctx.SendRaw(`{"cmd":"lockroom"}`)
		panic("after effect")
	})
	r := executeSnapshotTool(t, context.Background(), snapshotToolFixture(session, nil, operation), "nuke", `{"room":"lounge"}`)
	if r.ErrorCode != "ACTION_OUTCOME_UNKNOWN" || r.ActionCount != 2 || r.DeliveryCount != 1 || session.closed != 1 {
		t.Fatalf("result=%+v closed=%d", r, session.closed)
	}
}
