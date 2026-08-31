package command

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zenbot/internal/listener"
	"zenbot/internal/model"
	"zenbot/internal/repository/h2"
	"zenbot/internal/service"
)

func TestUsersAndNicksDispatchAgainstRealH2(t *testing.T) {
	jar := os.Getenv("H2_JAR")
	if jar == "" {
		jar = "/Users/ab/.m2/repository/com/h2database/h2/2.3.232/h2-2.3.232.jar"
	}
	dir := t.TempDir()
	db, err := h2.Open(context.Background(), h2.Config{BaseDir: dir, DatabaseStem: filepath.Join(dir, "dispatch.db"), H2Jar: jar, Port: 55437, StartupTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"INSERT INTO trips(type,trip,created_on) VALUES('USER','trip-a',1)",
		"INSERT INTO names(name,created_on) VALUES('merc',1)",
		"INSERT INTO trip_names(trip_id,name_id) SELECT t.id,n.id FROM trips t,names n WHERE t.trip='trip-a' AND n.name='merc'",
		"INSERT INTO messages(trip,name,message,created_on,visibility) VALUES('trip-a','merc','hello',1,'PUBLIC')",
	} {
		if _, err := db.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice", Hash: "hash"}}, bundle: &service.Bundle{Users: &service.UserService{Queries: db}}}
	if err := RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		alias string
		text  string
		want  string
	}{
		{"whitelist", "!whitelist", "Users: \\n"},
		{"t2n", "!t2n  TRIP-A  ignored", "merc"},
	} {
		engine.chats = nil
		payload, _ := json.Marshal(model.ChatMessage{Name: "alice", Trip: "trip-a", Text: tc.text})
		listener.NewUserChatListener(engine).Notify(string(payload))
		if len(engine.chats) != 1 || !contains(engine.chats[0], tc.want) {
			t.Fatalf("%s chats=%v, want substring %q", tc.alias, engine.chats, tc.want)
		}
	}
}
