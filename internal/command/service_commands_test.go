package command

import (
	"context"
	"net"
	"strings"
	"testing"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestPingTimeWeatherUseConcreteRegistryCommands(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			c.Close()
		}
	}()
	srv := auditUtilityServer(t)
	stub := &commandEngineStub{bundle: &service.Bundle{Ping: &service.PingService{Address: ln.Addr().String()}, Time: &service.TimeService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", SunriseURL: srv.URL + "/sun?lat=%s&lng=%s", TimezoneURL: srv.URL + "/time?lat=%s&lng=%s"}}}
	for _, tc := range []struct {
		alias, text string
		want        model.Status
		whisper     bool
	}{{"ping", "!ping", model.SUCCESSFUL, false}, {"p", "!p", model.SUCCESSFUL, false}, {"time", "!time Tokyo", model.SUCCESSFUL, true}, {"t", "!t Tokyo", model.SUCCESSFUL, true}} {
		d, ok := commandDefinitionFor(tc.alias)
		if !ok || d.Role != model.REGULAR {
			t.Fatalf("bad definition %s", tc.alias)
		}
		stub.chats = nil
		st, err := d.New(stub, &model.ChatMessage{Name: "alice", Text: tc.text, IsWhisper: true}).Execute(context.Background())
		if err != nil || st != tc.want || len(stub.chats) != 1 || !strings.HasSuffix(stub.chats[0], "|"+boolString(tc.whisper)) {
			t.Fatalf("%s status=%v err=%v chat=%v", tc.alias, st, err, stub.chats)
		}
	}
}
