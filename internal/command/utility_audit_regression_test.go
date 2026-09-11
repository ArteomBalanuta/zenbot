package command

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func TestUtilityAuditPingFailureIsNotZeroLatencySuccess(t *testing.T) {
	e := &commandEngineStub{bundle: &service.Bundle{Ping: &service.PingService{Address: "127.0.0.1:0"}}}
	d, _ := commandDefinitionFor("ping")
	status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!ping"}).Execute(context.Background())
	t.Logf("status=%v err=%v chats=%q", status, err, e.chats)
	if status == model.SUCCESSFUL || err == nil {
		t.Fatal("failed TCP dial was reported as successful measurement")
	}
}

type utilitySendErrorEngine struct {
	*commandEngineStub
	err error
}

func (e *utilitySendErrorEngine) SendChatMessage(string, string, bool) (string, error) {
	return "", e.err
}

func auditPingService(t *testing.T) *service.PingService {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return &service.PingService{Address: ln.Addr().String()}
}

func TestUtilityAuditWeatherTimePingRequireServices(t *testing.T) {
	for _, alias := range []string{"weather", "time", "ping"} {
		e := &commandEngineStub{}
		d, _ := commandDefinitionFor(alias)
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias + " City"}).Execute(context.Background())
		if err == nil || status == model.SUCCESSFUL || len(e.chats) != 0 {
			t.Errorf("%s status=%s err=%v chats=%v", alias, status, err, e.chats)
		}
	}
}

func auditUtilityServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch r.URL.Path {
		case "/geo":
			body = `{"geonames":[{"name":"City","countryName":"Country","lat":"1","lng":"2"}]}`
		case "/forecast":
			body = `{"timezone":"UTC","current_weather":{"time":"2026-09-11T12:00","temperature":21}}`
		case "/sun":
			body = `{"results":{"date":"2026-09-11","utc_offset":0}}`
		case "/time":
			body = `{"dateTime":"2026-09-11T12:00:00Z","timeZone":"UTC"}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUtilityAuditWeatherTimePingReturnDeliveryErrors(t *testing.T) {
	srv := auditUtilityServer(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			_ = c.Close()
		}
	}()
	b := &service.Bundle{
		Ping:    &service.PingService{Address: ln.Addr().String()},
		Weather: &service.WeatherService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", ForecastURL: srv.URL + "/forecast"},
		Time:    &service.TimeService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", SunriseURL: srv.URL + "/sun?lat=%s&lng=%s", TimezoneURL: srv.URL + "/time?lat=%s&lng=%s"},
	}
	wantErr := errors.New("send failed")
	for _, alias := range []string{"weather", "time", "ping"} {
		e := &utilitySendErrorEngine{commandEngineStub: &commandEngineStub{bundle: b}, err: wantErr}
		d, _ := commandDefinitionFor(alias)
		status, err := d.New(e, &model.ChatMessage{Name: "alice", Text: "!" + alias + " City"}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, wantErr) {
			t.Errorf("%s status=%s err=%v", alias, status, err)
		}
	}
}
