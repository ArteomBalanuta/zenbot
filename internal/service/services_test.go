package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWeatherGetUsesSaturnEndpointsAndFormatsForecast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("q") == "Paris" {
			if r.URL.Query().Get("maxRows") != "1" || r.URL.Query().Get("username") != "dev1" {
				t.Errorf("query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"geonames":[{"name":"Paris","countryName":"France","lat":"48.8","lng":"2.3"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"timezone":"UTC","current_weather":{"temperature":21,"windspeed":7,"weathercode":0,"time":"2026-03-26T12:00"},"current_weather_units":{"temperature":"C","windspeed":"km/h"},"daily":{"sunrise":["2026-03-26T06:30"],"sunset":["2026-03-26T18:45"],"uv_index_max":["4"],"shortwave_radiation_sum":["12"]},"daily_units":{"uv_index_max":"idx","shortwave_radiation_sum":"MJ/m2"},"hourly":{"apparent_temperature":["20"],"relative_humidity_2m":["35"],"surface_pressure":["1008"],"pressure_msl":["1014"],"shortwave_radiation":["500"],"diffuse_radiation":["120"],"soil_temperature_18cm":["15"],"soil_moisture_3_to_9cm":["0.22"]},"hourly_units":{"apparent_temperature":"C","relative_humidity_2m":"%","surface_pressure":"hPa","pressure_msl":"hPa","shortwave_radiation":"W/m2","diffuse_radiation":"W/m2","soil_temperature_18cm":"C","soil_moisture_3_to_9cm":"m3/m3"}}`))
	}))
	defer srv.Close()
	got, e := (&WeatherService{HTTP: srv.Client(), GeoURL: srv.URL, ForecastURL: srv.URL, Now: func() time.Time { return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC) }}).Get(context.Background(), "Paris")
	if e != nil || !strings.HasPrefix(got, "Weather forecast for today: **Paris, France**\\nTemperature: 21 C") {
		t.Fatalf("got %q err %v", got, e)
	}
}
func TestSearchUsesDuckDuckGoCompatibleEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"AbstractText":"answer"}`))
	}))
	defer srv.Close()
	got, e := (&SearchService{HTTP: srv.Client(), Endpoint: srv.URL}).Search(context.Background(), "a b")
	if e != nil || got != `{\\"AbstractText\\":\\"answer\\"}` {
		t.Fatalf("got %q err %v", got, e)
	}
}
func TestPingHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&PingService{Address: "127.0.0.1:1"}).Ping(ctx)
	if err == nil {
		t.Fatal("expected canceled ping to fail")
	}
}

func TestPingUsesInjectedAddress(t *testing.T) {
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
	d, e := (&PingService{Address: ln.Addr().String()}).Ping(context.Background())
	if e != nil || d <= 0 {
		t.Fatalf("duration=%v err=%v", d, e)
	}
}
