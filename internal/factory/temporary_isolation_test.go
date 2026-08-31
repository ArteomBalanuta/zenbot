package factory

import (
	"testing"

	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

func TestTemporaryOnlineSetIsolatedFromPermanentEngineState(t *testing.T) {
	cfg := &config.Config{Channel: "temporary", Name: "bot", CmdPrefix: "!"}
	e, err := NewEngineWithOptions(model.REPLICA, cfg, &repository.DummyImpl{}, EngineOptions{ListenerProfile: core.TemporaryOnlineSet})
	if err != nil {
		t.Fatal(err)
	}
	original := &model.User{Name: "existing", Trip: "trip"}
	e.ActiveUsers[original] = struct{}{}
	for _, payload := range []string{
		`{"cmd":"onlineSet","users":[{"nick":"replacement"}]}`,
		`{"cmd":"onlineAdd","nick":"added"}`,
		`{"cmd":"onlineRemove","nick":"existing"}`,
		`{"cmd":"chat","nick":"x","text":"!replica room"}`,
		`{"cmd":"info","nick":"x","text":"info"}`,
	} {
		e.DispatchMessage(payload)
	}
	users := e.GetActiveUsers()
	if len(*users) != 1 {
		t.Fatalf("temporary active users changed: %v", *users)
	}
	if _, ok := (*users)[original]; !ok {
		t.Fatal("temporary engine lost original active user")
	}
}
