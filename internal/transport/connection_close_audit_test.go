package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
)

type closeAuditConn struct {
	net.Conn
	closeError error
	closeCalls atomic.Int32
}

func (c *closeAuditConn) Close() error {
	c.closeCalls.Add(1)
	return errors.Join(c.Conn.Close(), c.closeError)
}

func TestConnectionCloseAuditPreservesResultForRepeatedAndConcurrentCallers(t *testing.T) {
	for _, closeFailure := range []error{nil, errors.New("underlying socket close failed")} {
		name := "success"
		if closeFailure != nil {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
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
			var socket *closeAuditConn
			dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				raw, err := (&net.Dialer{}).DialContext(ctx, network, address)
				if err != nil {
					return nil, err
				}
				socket = &closeAuditConn{Conn: raw, closeError: closeFailure}
				return socket, nil
			}}
			connection := NewConnection(Config{URL: "ws" + strings.TrimPrefix(server.URL, "http"), Dialer: dialer})
			if err := connection.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = connection.Close(context.Background()) })
			const callers = 16
			ready := make(chan struct{})
			results := make(chan error, callers)
			var wg sync.WaitGroup
			for range callers {
				wg.Add(1)
				go func() { defer wg.Done(); <-ready; results <- connection.Close(context.Background()) }()
			}
			close(ready)
			wg.Wait()
			close(results)
			for err := range results {
				if !errors.Is(err, closeFailure) {
					t.Errorf("concurrent close lost original result: error=%v want=%v", err, closeFailure)
				}
			}
			if err := connection.Close(context.Background()); !errors.Is(err, closeFailure) {
				t.Errorf("repeated close lost original result: error=%v want=%v", err, closeFailure)
			}
			if calls := socket.closeCalls.Load(); calls != 1 {
				t.Fatalf("underlying socket close replayed: calls=%d", calls)
			}
			if connection.Connected() {
				t.Fatal("closed transport remains connected")
			}
		})
	}
}
