package transport

import (
	"context"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestConnectionLocalRoundTripAndConcurrentWrites(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	got := make(chan string, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := up.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer c.Close()
		for i := 0; i < 4; i++ {
			_, b, e := c.ReadMessage()
			if e != nil {
				return
			}
			if i == 0 {
				got <- string(b)
			}
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte("reply"))
	}))
	defer s.Close()
	u, _ := url.Parse(s.URL)
	u.Scheme = "ws"
	c := NewConnection(Config{URL: u.String(), PingInterval: time.Millisecond * 20})
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		go func() {
			if err := c.SendText(context.Background(), "hello"); err != nil {
				t.Error(err)
			}
		}()
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("server did not receive")
	}
	select {
	case m := <-c.Messages():
		if string(m) != "reply" {
			t.Fatalf("message=%q", m)
		}
	case <-time.After(time.Second):
		t.Fatal("reply timeout")
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestConnectionDialCancellation(t *testing.T) {
	c := NewConnection(Config{URL: "ws://127.0.0.1:1"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := c.Start(ctx); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestConnectionCloseBeforeStartPreventsDial(t *testing.T) {
	c := NewConnection(Config{URL: "ws://invalid"})
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != ErrClosed {
		t.Fatalf("start after close error=%v, want %v", err, ErrClosed)
	}
}
