package command_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/command"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/transport"
)

func TestRealInboundWebSocketReplicaCommandReachesManager(t *testing.T) {
	joins := make(chan string, 2)
	connections := make(chan int, 2)
	var n atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		connection := int(n.Add(1))
		_, join, err := ws.ReadMessage()
		if err != nil {
			return
		}
		joins <- string(join)
		connections <- connection
		if connection == 1 {
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"cmd":"onlineAdd","nick":"alice","trip":"x","hash":"h","uType":"user"}`))
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"cmd":"chat","nick":"alice","trip":"x","text":"!replica requested-room"}`))
		} else {
			_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"cmd":"onlineSet","users":[{"nick":"alice","trip":"x"}]}`))
		}
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	url := "ws" + server.URL[4:]
	cfg := &config.Config{WebsocketUrl: url, Channel: "host-room", Name: "bot", Password: "pw", CmdPrefix: "!", AdminTrips: []string{"x"}}
	manager := core.NewReplicaManager(cfg.Channel)
	options := factory.EngineOptions{Transport: transport.Config{URL: url}}
	master, err := factory.NewEngineWithOptions(model.MASTER, cfg, &repository.DummyImpl{}, options)
	if err != nil {
		t.Fatal(err)
	}
	replicas := factory.ReplicaFactory{Config: cfg, Repository: &repository.DummyImpl{}, Options: options}
	controller := core.NewManagedReplicaController(manager, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		return replicas.NewReplica(ctx, channel)
	})
	master.SetReplicaController(controller)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := master.StopContext(cleanup); err != nil {
			t.Error(err)
		}
		cancel()
		replicaCleanup, cancelReplicas := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelReplicas()
		if err := manager.StopAll(replicaCleanup); err != nil {
			t.Error(err)
		}
	}()
	if err := command.RegisterUserUtilities(master); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := master.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(manager.Channels()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := manager.Channels(); len(got) != 1 || got[0] != "requested-room" {
		t.Fatalf("manager channels=%v", got)
	}
	for {
		select {
		case connection := <-connections:
			if connection == 2 {
				return
			}
		case <-ctx.Done():
			t.Fatal("server did not consume the replica join frame")
		}
	}
}
