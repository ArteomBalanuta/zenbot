package factory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/config"
	"zenbot/internal/model"
)

func TestManagedMasterAndReplicaUseIndependentWebSockets(t *testing.T) {
	joins := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_, b, err := c.ReadMessage()
		if err == nil {
			joins <- string(b)
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"cmd":"onlineSet","users":[{"name":"alice","trip":"t","hash":"h"}]}`))
		<-r.Context().Done()
	}))
	defer srv.Close()
	url := "ws" + srv.URL[4:]
	cfg := &config.Config{WebsocketUrl: url, Channel: "master", Name: "bot", Password: "pw"}
	master, err := NewEngineWithOptions(model.MASTER, cfg, nil, EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	replicaCfg := *cfg
	replicaCfg.Channel = "replica"
	replica, err := NewEngineWithOptions(model.REPLICA, &replicaCfg, nil, EngineOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = master.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err = replica.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case j := <-joins:
			got[j] = true
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if !got[`{ "cmd": "join", "channel": "master", "nick": "bot#pw" }`] || !got[`{ "cmd": "join", "channel": "replica", "nick": "bot#pw" }`] {
		t.Fatalf("joins=%v", got)
	}
	stop, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	_ = master.StopContext(stop)
	_ = replica.StopContext(stop)
}
