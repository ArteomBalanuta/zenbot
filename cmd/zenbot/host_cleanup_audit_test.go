package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"

	"zenbot/internal/core"
	"zenbot/internal/transport"
)

// Keep the production websocket, connection, engine and supervisor in the test.
// Inject failures only at the underlying socket after its successful handshake.
type hostCleanupAuditSocket struct {
	net.Conn
	failWrites             atomic.Bool
	closeCalls             atomic.Int32
	writeError, closeError error
}

func (c *hostCleanupAuditSocket) Write(data []byte) (int, error) {
	if c.failWrites.Load() {
		return 0, c.writeError
	}
	return c.Conn.Write(data)
}
func (c *hostCleanupAuditSocket) Close() error {
	c.closeCalls.Add(1)
	return errors.Join(c.Conn.Close(), c.closeError)
}

type hostCleanupAuditDialer struct {
	dialer *websocket.Dialer
	socket **hostCleanupAuditSocket
}

func (d hostCleanupAuditDialer) DialContext(ctx context.Context, url string, headers http.Header) (*websocket.Conn, *http.Response, error) {
	ws, response, err := d.dialer.DialContext(ctx, url, headers)
	if err == nil {
		(*d.socket).failWrites.Store(true)
	}
	return ws, response, err
}

func TestHostCleanupAuditFailedStartPreservesPrimaryAndCleanupErrors(t *testing.T) {
	primary, cleanup := errors.New("join socket write failed"), errors.New("socket cleanup failed")
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	var socket *hostCleanupAuditSocket
	dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		raw, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		socket = &hostCleanupAuditSocket{Conn: raw, writeError: primary, closeError: cleanup}
		return socket, nil
	}}
	connection := transport.NewConnection(transport.Config{
		URL:    "ws" + strings.TrimPrefix(server.URL, "http"),
		Dialer: hostCleanupAuditDialer{dialer: dialer, socket: &socket},
	})
	t.Cleanup(func() { _ = connection.Close(context.Background()) })
	reports := make(chan error, 1)
	engine := &core.EngineImpl{Channel: "test", Name: "host", Transport: connection, LifecycleErrors: reports}
	builds := 0
	s := NewHostSupervisor(nil, func(ctx context.Context, candidate any) error {
		return candidate.(*core.EngineImpl).StopContext(ctx)
	}, func(context.Context) (any, error) { builds++; return engine, nil }, func(ctx context.Context, candidate any) error {
		return candidate.(*core.EngineImpl).StartContext(ctx)
	}, nil)
	err := s.StartInitial(context.Background())
	if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
		t.Errorf("source result lost primary or cleanup error: %v", err)
	}
	select {
	case report := <-reports:
		if !errors.Is(report, primary) || !errors.Is(report, cleanup) {
			t.Errorf("failed-start report lost primary or cleanup error: %v", report)
		}
	default:
		t.Error("failed-start source was not reported")
	}
	if err := engine.StopContext(context.Background()); !errors.Is(err, cleanup) {
		t.Errorf("repeated engine cleanup manufactured success: %v", err)
	}
	if s.Master() != nil || s.unretired != engine {
		t.Errorf("failed candidate ownership lost: master=%v retained=%v", s.Master(), s.unretired)
	}
	if retry := s.Restart(context.Background()); retry == nil || builds != 1 {
		t.Errorf("uncertain cleanup allowed replacement replay: retry=%v builds=%d", retry, builds)
	}
	if err := s.SignalTeardown(s.Shutdown, context.Background()); !errors.Is(err, cleanup) {
		t.Errorf("final teardown erased cleanup failure: %v", err)
	}
	if calls := socket.closeCalls.Load(); calls != 1 {
		t.Errorf("underlying socket close replayed: %d", calls)
	}
	if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
		t.Errorf("later teardown changed original error: %v", err)
	}
}
