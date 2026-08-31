package command_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/transport"
)

func TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry(t *testing.T) {
	baseline := runtime.NumGoroutine()
	joins := make(chan string, 2)
	replicaReady := make(chan struct{})
	closeReplica := make(chan struct{})
	replicaHandlerDone := make(chan struct{})
	masterHandlerDone := make(chan struct{})
	var connections atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		_, join, err := ws.ReadMessage()
		if err != nil {
			return
		}
		joins <- string(join)
		if strings.Contains(string(join), `"channel": "replica-room"`) {
			connections.Add(1)
			close(replicaReady)
			<-closeReplica
			_ = ws.Close()
			close(replicaHandlerDone)
			return
		}
		connections.Add(1)
		_, _, _ = ws.ReadMessage()
		close(masterHandlerDone)
	}))
	defer server.Close()

	url := "ws" + server.URL[4:]
	sink := make(chan error, 8)
	cfg := &config.Config{WebsocketUrl: url, Channel: "host-room", Name: "bot", Password: "pw"}
	manager := core.NewReplicaManager(cfg.Channel)
	options := factory.EngineOptions{Transport: transport.Config{URL: url}, LifecycleErrors: sink}
	master, err := factory.NewEngineWithOptions(model.MASTER, cfg, &repository.DummyImpl{}, options)
	if err != nil {
		t.Fatal(err)
	}
	replicas := factory.ReplicaFactory{Config: cfg, Repository: &repository.DummyImpl{}, Options: options}
	controller := core.NewManagedReplicaController(manager, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		return replicas.NewReplica(ctx, channel)
	}, sink)
	master.SetReplicaController(controller)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := master.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controller.AddReplica(ctx, "replica-room"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-replicaReady:
	case <-ctx.Done():
		t.Fatal("replica websocket did not connect")
	}
	if got := manager.Channels(); len(got) != 1 || got[0] != "replica-room" {
		t.Fatalf("manager channels before failure=%v", got)
	}

	close(closeReplica)
	waitUntil(t, ctx, func() bool { return len(manager.Channels()) == 0 })
	select {
	case <-replicaHandlerDone:
	case <-ctx.Done():
		t.Fatal("replica websocket handler did not exit")
	}
	if err := master.StopContext(context.Background()); err != nil {
		t.Fatalf("stop master: %v", err)
	}
	select {
	case <-masterHandlerDone:
	case <-ctx.Done():
		t.Fatal("master websocket handler did not exit")
	}

	var errors []string
	drain := true
	for drain {
		select {
		case err := <-sink:
			errors = append(errors, err.Error())
		default:
			drain = false
		}
	}
	if len(errors) != 1 {
		t.Fatalf("host error sink received %d errors: %v", len(errors), errors)
	}
	if !strings.Contains(errors[0], "replica replica-room runtime") || !strings.Contains(errors[0], "replica-room") {
		t.Fatalf("runtime error lacks replica channel context: %q", errors[0])
	}
	if strings.Contains(errors[0], "context canceled") || strings.Contains(errors[0], "transport is closed") {
		t.Fatalf("intentional cancellation/close noise reported: %q", errors[0])
	}

	waitUntil(t, ctx, func() bool { return runtime.NumGoroutine() <= baseline+2 })
	if got := connections.Load(); got != 2 {
		t.Fatalf("websocket connections=%d, want master and replica", got)
	}
}

func waitUntil(t *testing.T, ctx context.Context, condition func() bool) {
	t.Helper()
	for !condition() {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}
