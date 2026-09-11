package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/transport"
)

type proofTransport struct {
	msgs   chan transport.InboundMessage
	errs   chan error
	raws   []string
	closed int
}

func (p *proofTransport) Start(context.Context) error               { return nil }
func (p *proofTransport) Messages() <-chan transport.InboundMessage { return p.msgs }
func (p *proofTransport) Errors() <-chan error                      { return p.errs }
func (p *proofTransport) SendRaw(_ context.Context, raw []byte) error {
	p.raws = append(p.raws, string(raw))
	return nil
}
func (p *proofTransport) Close(context.Context) error { p.closed++; return nil }

func TestTemporaryFactoryPropagatesCallerContextAndClosedChannelsTerminate(t *testing.T) {
	for _, closed := range []string{"messages", "errors"} {
		t.Run(closed, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &proofTransport{msgs: make(chan transport.InboundMessage), errs: make(chan error)}
			registry := NewTemporarySessionRegistry()
			seen := make(chan OperationResult, 1)
			factory := &CoordinatedSessionFactory{Registry: registry, NewTransport: func(ctx context.Context, _ RoomSnapshotRequest) (TemporaryTransport, error) {
				if ctx == context.Background() {
					t.Error("factory lost caller context")
				}
				return p, nil
			}}
			c := NewRoomSnapshotCoordinator(factory, nil, ParseUsers, time.Second)
			req := request(&recordingOperation{})
			req.Context = parent
			req.OnComplete = func(r OperationResult) { seen <- r }
			if err := c.Submit(req); err != nil {
				t.Fatal(err)
			}
			if closed == "messages" {
				close(p.msgs)
			} else {
				close(p.errs)
			}
			select {
			case r := <-seen:
				if r.Outcome != OutcomeFailed || registry.Len() != 0 {
					t.Fatalf("result=%+v registry=%d", r, registry.Len())
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("closed channel did not terminate workflow")
			}
		})
	}
}

type failedStartupTransport struct {
	proofTransport
	err error
}

func (p *failedStartupTransport) Start(context.Context) error { return p.err }
func TestTemporarySessionCloseWorksWhenStartupNeverLaunchedReader(t *testing.T) {
	p := &failedStartupTransport{proofTransport: proofTransport{msgs: make(chan transport.InboundMessage), errs: make(chan error)}, err: errors.New("startup failed")}
	registry := NewTemporarySessionRegistry()
	f := &CoordinatedSessionFactory{Registry: registry, NewTransport: func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error) { return p, nil }}
	c := NewRoomSnapshotCoordinator(f, nil, ParseUsers, time.Second)
	begin := time.Now()
	if err := c.Submit(request(&recordingOperation{})); !errors.Is(err, p.err) {
		t.Fatalf("submit=%v", err)
	}
	if registry.Len() != 0 || p.closed != 1 || time.Since(begin) > 500*time.Millisecond {
		t.Fatalf("registry=%d closed=%d elapsed=%s", registry.Len(), p.closed, time.Since(begin))
	}
}
func TestTransportSessionCredentialedRemoteMessageJoin(t *testing.T) {
	transport := &proofTransport{msgs: make(chan transport.InboundMessage), errs: make(chan error)}
	factory := &CoordinatedSessionFactory{Registry: NewTemporarySessionRegistry(), NewTransport: func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error) { return transport, nil }}
	session, err := factory.Create(RoomSnapshotRequest{SourceChannel: "source", TemporaryJoin: &TemporaryJoin{Channel: "remote\" room", Nick: "msg-12345678", Password: "test-\\password"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	if len(transport.raws) != 1 {
		t.Fatalf("joins=%v", transport.raws)
	}
	var join struct {
		Cmd, Channel, Nick string
	}
	if err := json.Unmarshal([]byte(transport.raws[0]), &join); err != nil {
		t.Fatalf("invalid JSON %q: %v", transport.raws[0], err)
	}
	if join.Cmd != "join" || join.Channel != "remote\" room" || join.Nick != "msg-12345678#test-\\password" {
		t.Fatalf("join=%+v", join)
	}
}

func TestTransportSessionLegacyJoinRemainsCredentialFree(t *testing.T) {
	transport := &proofTransport{msgs: make(chan transport.InboundMessage), errs: make(chan error)}
	factory := &CoordinatedSessionFactory{Registry: NewTemporarySessionRegistry(), NewTransport: func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error) { return transport, nil }}
	session, err := factory.Create(RoomSnapshotRequest{SourceChannel: "legacy-source"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	var join map[string]string
	if len(transport.raws) != 1 || json.Unmarshal([]byte(transport.raws[0]), &join) != nil || join["channel"] != "legacy-source" || join["nick"] != "" {
		t.Fatalf("join=%v raw=%v", join, transport.raws)
	}
}

func TestCoordinatedTransportSessionRoutesSnapshotAndErrorOnce(t *testing.T) {
	r := NewTemporarySessionRegistry()
	p := &proofTransport{msgs: make(chan transport.InboundMessage, 1), errs: make(chan error, 2)}
	payloads := make(chan string, 1)
	errorSeen := make(chan error, 2)
	closedSeen := make(chan struct{}, 2)
	f := &CoordinatedSessionFactory{Registry: r, NewTransport: func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error) { return p, nil }, OnTransportError: func(_ string, e error) { errorSeen <- e }, OnClosed: func(_ string, _ int, _ string) { closedSeen <- struct{}{} }}
	s, err := f.Create(RoomSnapshotRequest{}, func(v string) { payloads <- v })
	if err != nil {
		t.Fatal(err)
	}
	if s.ID() == "" {
		t.Fatal("empty id")
	}
	if err = s.Start(); err != nil {
		t.Fatal(err)
	}
	p.msgs <- transport.InboundMessage{Payload: []byte(`{"cmd":"onlineSet","users":[{"nick":"a"}]}`), ReceivedAt: time.Now()}
	select {
	case got := <-payloads:
		if got == "" {
			t.Fatal("empty payload")
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot not routed")
	}
	p.errs <- errors.New("read failed")
	p.errs <- errors.New("duplicate")
	select {
	case <-errorSeen:
	case <-time.After(time.Second):
		t.Fatal("error not routed")
	}
	select {
	case e := <-errorSeen:
		t.Fatalf("duplicate error routed: %v", e)
	case <-time.After(20 * time.Millisecond):
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closedSeen:
	case <-time.After(time.Second):
		t.Fatal("close callback not routed")
	}
	select {
	case <-closedSeen:
		t.Fatal("duplicate close callback routed")
	case <-time.After(20 * time.Millisecond):
	}
	if r.Len() != 0 || p.closed != 1 {
		t.Fatalf("registry=%d closed=%d", r.Len(), p.closed)
	}
}

func TestRealCoordinatedSessionUsesWebSocketAndCoordinatorSink(t *testing.T) {
	payload := `{"cmd":"onlineSet","users":[{"nick":"exact","trip":"trip"}]}`
	joined := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
		close(joined)
		_ = ws.WriteMessage(websocket.TextMessage, []byte(payload))
		<-r.Context().Done()
	}))
	defer srv.Close()
	url := "ws" + srv.URL[4:]
	registry := NewTemporarySessionRegistry()
	outcomes := make(chan OperationResult, 1)
	var got Snapshot
	coordinator := NewRoomSnapshotCoordinatorWithOutcome(nil, nil, func(raw string) (Snapshot, error) { return Parse(raw, false) }, time.Second, func(_ RoomSnapshotRequest, result OperationResult) { outcomes <- result })
	factory := &CoordinatedSessionFactory{Registry: registry, NewTransport: func(context.Context, RoomSnapshotRequest) (TemporaryTransport, error) {
		return transport.NewConnection(transport.Config{URL: url}), nil
	}}
	coordinator.factory = factory
	factory.BindCoordinator(coordinator)
	req := RoomSnapshotRequest{WorkflowID: "real", Author: "admin", SourceChannel: "source", TargetChannel: "target", Operation: OperationFunc(func(_ RoomSnapshotContext, s Snapshot) (OperationResult, error) { got = s; return Success(), nil })}
	if err := coordinator.Submit(req); err != nil {
		t.Fatal(err)
	}
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("server did not receive join")
	}
	select {
	case result := <-outcomes:
		if result.Outcome != OutcomeSuccess {
			t.Fatalf("outcome=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator did not receive websocket snapshot")
	}
	if len(got.Users) != 1 || got.Users[0].Name != "exact" || got.Users[0].Trip != "trip" {
		t.Fatalf("snapshot=%+v", got)
	}
	deadline := time.Now().Add(time.Second)
	for (coordinator.ActiveWorkflowCount() != 0 || registry.Len() != 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if coordinator.ActiveWorkflowCount() != 0 || registry.Len() != 0 {
		t.Fatalf("active=%d registry=%d", coordinator.ActiveWorkflowCount(), registry.Len())
	}
}
