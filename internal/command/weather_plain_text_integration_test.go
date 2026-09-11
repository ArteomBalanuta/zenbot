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
		_, _ = w.Write([]byte(`{
			"timezone":"Europe/Chisinau",
			"current_weather":{"temperature":21,"windspeed":7,"weathercode":1,"time":"2026-09-11T12:15"},
			"current_weather_units":{"temperature":"°C","windspeed":"km/h"},
			"hourly":{"time":["2026-09-11T12:00"],"apparent_temperature":[20],"relative_humidity_2m":[55],"wind_direction_10m":[270],"wind_gusts_10m":[35.2],"cloud_cover":[42],"visibility":[12000],"dew_point_2m":[-1.5]},
			"hourly_units":{"apparent_temperature":"°C","relative_humidity_2m":"%","wind_direction_10m":"°","wind_gusts_10m":"km/h","cloud_cover":"%","visibility":"m","dew_point_2m":"°C"},
			"daily":{"time":["2026-09-11"],"sunrise":["2026-09-11T06:36"],"sunset":["2026-09-11T19:24"],"temperature_2m_min":[-2.5],"temperature_2m_max":[21.5],"apparent_temperature_min":[-4],"apparent_temperature_max":[23],"precipitation_probability_max":[75],"precipitation_sum":[3.2],"snowfall_sum":[0],"daylight_duration":[46079.9],"sunshine_duration":[28980]},
			"daily_units":{"temperature_2m_min":"°C","temperature_2m_max":"°C","apparent_temperature_min":"°C","apparent_temperature_max":"°C","precipitation_probability_max":"%","precipitation_sum":"mm","snowfall_sum":"cm","daylight_duration":"s","sunshine_duration":"s"}
		}`))
	}))
	defer srv.Close()
	weather := &service.WeatherService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", ForecastURL: srv.URL + "/forecast"}
	for _, alias := range []string{"weather", "w", "today"} {
		t.Run(alias, func(t *testing.T) {
			got := runWeatherTextCommand(t, weather, "*"+alias+" chisinau")
			lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
			if len(lines) != 31 {
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
			for _, want := range []string{
				"Temperature min today: -2.5 °C", "Temperature max today: 21.5 °C",
				"Feels min today: -4 °C", "Feels max today: 23 °C",
				"Precip. chance max today: 75 %", "Precipitation total today: 3.2 mm", "Snowfall total today: 0 cm",
				"Daylight today: 12h 47m", "Sunshine today: 8h 03m",
				"Wind direction (hourly): 270 °", "Wind gusts (prev hour max): 35.2 km/h",
				"Cloud cover (hourly): 42 %", "Visibility (hourly): 12000 m", "Dew point (hourly): -1.5 °C",
			} {
				if !strings.Contains(got, want+"\n") {
					t.Errorf("missing extended metric %q from %q", want, got)
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
	if !strings.Contains(got, "Chișinău, Moldova") || strings.Count(got, "\n") != 31 {
		t.Fatalf("unexpected live output %q", got)
	}
	t.Log(got)
}
