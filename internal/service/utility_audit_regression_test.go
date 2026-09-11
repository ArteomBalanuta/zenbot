package service_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"zenbot/internal/service"
)

type auditRoundTripper func(*http.Request) (*http.Response, error)

func (f auditRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func auditJSON(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestUtilityAuditGeocoderRequestsJSON(t *testing.T) {
	for _, name := range []string{"weather", "time"} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
				t.Logf("geocoder URL: %s", r.URL)
				if r.URL.Path != "/searchJSON" && r.URL.Query().Get("type") != "json" {
					t.Errorf("JSON decoder requested geocoder XML endpoint: %s", r.URL)
				}
				return auditJSON(`{"geonames":[]}`), nil
			})}
			if name == "weather" {
				_, _ = (&service.WeatherService{HTTP: client}).Get(context.Background(), "Paris")
			} else {
				_, _ = (&service.TimeService{HTTP: client}).Get(context.Background(), "Paris")
			}
		})
	}
}

func TestUtilityAuditWeatherNumericPayload(t *testing.T) {
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("q") != "" {
			return auditJSON(`{"geonames":[{"name":"Paris","countryName":"France","lat":"48.8","lng":"2.3"}]}`), nil
		}
		return auditJSON(`{"timezone":"UTC","current_weather":{"temperature":21,"windspeed":7,"weathercode":0,"time":"2026-09-11T00:00"},"daily":{"time":["2026-09-11"],"sunrise":["2026-09-11T06:30"],"sunset":["2026-09-11T18:45"],"uv_index_max":[4.2],"shortwave_radiation_sum":[12]},"hourly":{"time":["2026-09-11T00:00"],"apparent_temperature":[20.1],"relative_humidity_2m":[35]}}`), nil
	})}
	got, err := (&service.WeatherService{HTTP: client}).Get(context.Background(), "Paris")
	if err != nil {
		t.Fatalf("numeric weather response failed: %v", err)
	}
	if !strings.Contains(got, "20.1") {
		t.Fatalf("missing numeric apparent temperature: %q", got)
	}
}

func TestUtilityAuditWeatherInvalidTimezoneDoesNotPanic(t *testing.T) {
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("q") != "" {
			return auditJSON(`{"geonames":[{"name":"Paris","countryName":"France","lat":"48.8","lng":"2.3"}]}`), nil
		}
		return auditJSON(`{"timezone":"invalid/zone","current_weather":{"temperature":21,"time":"2026-09-11T00:00"}}`), nil
	})}
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("weather formatter panic: %v", v)
		}
	}()
	if _, err := (&service.WeatherService{HTTP: client}).Get(context.Background(), "Paris"); err == nil {
		t.Error("invalid timezone must return an error")
	}
}

func TestUtilityAuditTimePreservesFractionalUTCOffset(t *testing.T) {
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/geo":
			return auditJSON(`{"geonames":[{"countryName":"India","lat":"28.6","lng":"77.2"}]}`), nil
		case "/sun":
			return auditJSON(`{"results":{"date":"2026-09-11","timezone":"Asia/Kolkata","utc_offset":330}}`), nil
		default:
			return auditJSON(`{"dateTime":"2026-09-11T12:00:00+05:30","timeZone":"Asia/Kolkata"}`), nil
		}
	})}
	got, err := (&service.TimeService{HTTP: client, GeoURL: "https://test/geo", SunriseURL: "https://test/sun?lat=%s&lng=%s", TimezoneURL: "https://test/time?lat=%s&lng=%s"}).Get(context.Background(), "Delhi")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("time output: %s", got)
	if !strings.Contains(got, "+05:30") && !strings.Contains(got, "+5:30") && !strings.Contains(got, "+5.5") {
		t.Fatal("fractional UTC offset lost")
	}
}
