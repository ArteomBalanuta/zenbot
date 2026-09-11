package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

func runWeatherTextCommand(t *testing.T, weather *service.WeatherService, text string) string {
	t.Helper()
	e := &core.EngineImpl{Prefix: "*", OutMessageQueue: make(chan string, 2), Services: &service.Bundle{Weather: weather}}
	if err := RegisterUserUtilities(e); err != nil {
		t.Fatal(err)
	}
	alias := strings.TrimPrefix(strings.Fields(text)[0], "*")
	cmd := common.BuildCommand(alias, e, &model.ChatMessage{Name: "merc", Text: text})
	if cmd == nil {
		t.Fatalf("unregistered alias %s", alias)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	status, err := common.InvokeCommand(ctx, e, cmd)
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("command=%q status=%v err=%v", text, status, err)
	}
	select {
	case frame := <-e.OutMessageQueue:
		var got struct {
			Cmd  string `json:"cmd"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(frame), &got); err != nil {
			t.Fatal(err)
		}
		if got.Cmd != "chat" || !strings.HasPrefix(got.Text, "@merc ") || strings.Contains(got.Text, `\n`) {
			t.Fatalf("invalid chat output %q", got.Text)
		}
		if len(e.OutMessageQueue) != 0 {
			t.Fatal("weather emitted extra messages")
		}
		return strings.TrimPrefix(got.Text, "@merc ")
	default:
		t.Fatal("weather completed without a chat frame")
	}
	return ""
}

func TestWeatherChisinauAliasesDeliverAlignedPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/geo" {
			if r.URL.Query().Get("q") != "chisinau" {
				t.Errorf("location query=%q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"geonames":[{"name":"Chișinău","countryName":"Moldova","lat":"47.00902","lng":"28.85938"}]}`))
			return
		}
		q := r.URL.Query()
		if q.Get("latitude") != "47.00902" || q.Get("longitude") != "28.85938" || q.Get("timezone") != "auto" || q.Get("current_weather") != "true" {
			t.Errorf("forecast query=%q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"timezone":"Europe/Chisinau","current_weather":{"temperature":21,"windspeed":7,"weathercode":1,"time":"2026-09-11T12:15"},"current_weather_units":{"temperature":"°C","windspeed":"km/h"},"hourly":{"time":["2026-09-11T12:00"],"apparent_temperature":[20],"relative_humidity_2m":[55]},"hourly_units":{"apparent_temperature":"°C","relative_humidity_2m":"%"},"daily":{"time":["2026-09-11"],"sunrise":["2026-09-11T06:36"],"sunset":["2026-09-11T19:24"]}}`))
	}))
	defer srv.Close()
	weather := &service.WeatherService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", ForecastURL: srv.URL + "/forecast"}
	for _, alias := range []string{"weather", "w", "today"} {
		t.Run(alias, func(t *testing.T) {
			got := runWeatherTextCommand(t, weather, "*"+alias+" chisinau")
			lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
			if len(lines) != 17 {
				t.Fatalf("rows=%d output=%q", len(lines), got)
			}
			for _, line := range lines {
				before, _, ok := strings.Cut(line, ":")
				if !ok || utf8.RuneCountInString(before) != 28 {
					t.Fatalf("misaligned row %q", line)
				}
			}
			for _, want := range []string{"Weather forecast for today: **Chișinău, Moldova**\n", "Temperature: 21 °C\n", "Feels temp: 20 °C\n", "Air Humidity: 55 %\n", "Precipitation: 🌤️\n", "Wind speed: 7 km/h\n", "Time: Fri, 11 Sep 2026 12-15-00 +0300\n", "Sun rise: Fri, 11 Sep 2026 06-36-00 +0300\n", "Sun set: Fri, 11 Sep 2026 19-24-00 +0300\n"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q from %q", want, got)
				}
			}
		})
	}
}

func TestWeatherChisinauLiveProviders(t *testing.T) {
	if os.Getenv("ZENBOT_TEST_LIVE_WEATHER") != "1" {
		t.Skip("opt-in read-only provider check; never sends to a room")
	}
	got := runWeatherTextCommand(t, &service.WeatherService{}, "*weather chisinau")
	if !strings.Contains(got, "Chișinău, Moldova") || strings.Count(got, "\n") != 17 {
		t.Fatalf("unexpected live output %q", got)
	}
	t.Log(got)
}
