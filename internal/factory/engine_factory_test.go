package factory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/relay"
	"zenbot/internal/repository"
)

type hostRelayStub struct{}

func (hostRelayStub) RelayAgentMessage(context.Context, string, string) error { return nil }

type shadowBanRepositoryStub struct{ repository.DummyImpl }

func (shadowBanRepositoryStub) PersistShadowBan(context.Context, model.User, string) error {
	return nil
}

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

func TestNewEngineRejectsAgentWithoutHostRelay(t *testing.T) {
	cfg := &config.Config{WebsocketUrl: "ws://example.test", Channel: "agent", Name: "bot"}
	if _, err := NewEngineWithOptions(model.AGENT, cfg, nil, EngineOptions{}); err == nil {
		t.Fatal("expected AGENT construction without a host relay to fail")
	}
}

func TestNewEngineInstallsAgentHostRelayOnce(t *testing.T) {
	cfg := &config.Config{WebsocketUrl: "ws://example.test", Channel: "agent", Name: "bot"}
	var host relay.HostRelay = hostRelayStub{}
	e, err := NewEngineWithOptions(model.AGENT, cfg, nil, EngineOptions{HostRelay: host})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.HostRelay(); got != host {
		t.Fatalf("host relay=%T %p, want %T %p", got, got, host, host)
	}
}

func TestComposeJoinAutomationFailsClosedWithoutShadowBanRepository(t *testing.T) {
	cfg := &config.Config{Agent: config.AgentConfig{
		ModerationEnabled: true, ModerationJoinBurstCount: 2, ModerationJoinWindowSeconds: 1,
		ModerationSameHashCount: 2, ModerationSameHashWindowSeconds: 1,
		ModerationNameClusterCount: 2, ModerationNameClusterWindowSeconds: 1,
		ModerationPostKickWindowSeconds: 1, ModerationActionCooldownSeconds: 1,
	}}
	e := &core.EngineImpl{Name: "bot", Repository: &repository.DummyImpl{}}
	if got := composeJoinAutomation(e, cfg); got != nil {
		t.Fatalf("automation composed without authoritative shadow-ban persistence: %T", got)
	}
}

func TestComposeMessageAutomationFailsClosedForIncompleteConfig(t *testing.T) {
	cfg := &config.Config{Agent: config.AgentConfig{ModerationEnabled: true}}
	e := &core.EngineImpl{Name: "bot", Repository: &repository.DummyImpl{}}
	if got := ComposeMessageAutomation(e, cfg); got != nil {
		t.Fatalf("message automation composed with incomplete authoritative configuration: %T", got)
	}
}

func TestComposeMessageAutomationProtectsCaseVariantResolvedAdmin(t *testing.T) {
	cfg := &config.Config{AdminTrips: []string{"admin-trip"}, Agent: config.AgentConfig{
		ModerationEnabled:        true,
		ModerationJoinBurstCount: 2, ModerationJoinWindowSeconds: 1,
		ModerationSameHashCount: 2, ModerationSameHashWindowSeconds: 1,
		ModerationNameClusterCount: 2, ModerationNameClusterWindowSeconds: 1,
		ModerationPostKickWindowSeconds: 1, ModerationActionCooldownSeconds: 1,
		ModerationMessageBurstCount: 1, ModerationMessageBurstWindowSeconds: 1,
		ModerationRepeatedMessageCount: 2, ModerationRepeatedMessageWindowSeconds: 1,
		ModerationSecondBreachWindowSeconds: 1,
	}}
	e := &core.EngineImpl{Name: "bot", Repository: &shadowBanRepositoryStub{}, ActiveUsers: map[*model.User]struct{}{}}
	e.AddActiveUser(&model.User{Name: "Admin", Trip: "admin-trip"})
	automation := ComposeMessageAutomation(e, cfg)
	if automation == nil {
		t.Fatal("expected complete authoritative message automation")
	}
	if decisions := automation.Monitor.OnMessage(model.ChatMessage{Name: "ADMIN", Text: "flood"}); len(decisions) != 0 {
		t.Fatalf("case variant of resolved protected admin was moderated: %#v", decisions)
	}
}
