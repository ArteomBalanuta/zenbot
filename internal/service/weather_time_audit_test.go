package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"zenbot/internal/service"
)

const auditGeo = `{"geonames":[{"name":"City","countryName":"Country","lat":"48.8","lng":"2.3"}]}`
const auditForecast = `{"timezone":"UTC","current_weather":{"temperature":21,"windspeed":7,"weathercode":0,"time":"2026-09-11T12:15"},"daily":{"time":["2026-09-10","2026-09-11"],"uv_index_max":["99","4.2"],"sunrise":["2026-09-10T06:00","2026-09-11T06:30"]},"hourly":{"time":["2026-09-10T12:00","2026-09-11T12:00"],"apparent_temperature":["999","20.1"],"relative_humidity_2m":["99",null]}}`
const auditSun = `{"status":"OK","results":{"date":"2026-09-11","timezone":"UTC","utc_offset":0}}`
const auditClock = `{"dateTime":"2026-09-11T12:15:00.1234567","timeZone":"UTC"}`

func auditProviderClient(geo, forecast, sun, clock string) *http.Client {
	return &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/geo":
			return auditJSON(geo), nil
		case "/forecast":
			return auditJSON(forecast), nil
		case "/sun":
			return auditJSON(sun), nil
		case "/clock":
			return auditJSON(clock), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
	})}
}

func auditWeather(client *http.Client) *service.WeatherService {
	return &service.WeatherService{HTTP: client, GeoURL: "https://test/geo", ForecastURL: "https://test/forecast", Now: func() time.Time { return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) }}
}

func auditTimeService(client *http.Client) *service.TimeService {
	return &service.TimeService{HTTP: client, GeoURL: "https://test/geo", SunriseURL: "https://test/sun?lat=%s&lng=%s", TimezoneURL: "https://test/clock?lat=%s&lng=%s"}
}

func TestWeatherUsesProviderTimeAxesAndMarksMissingMetricsUnavailable(t *testing.T) {
	got, err := auditWeather(auditProviderClient(auditGeo, auditForecast, auditSun, auditClock)).Get(context.Background(), "City")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"20.1", "4.2", "11 Sep 2026 06-30", "unavailable"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "999") || strings.Contains(got, "0001") {
		t.Errorf("fabricated or wrong-time output: %s", got)
	}
}

func TestWeatherDoesNotRequestProcessDate(t *testing.T) {
	client := auditProviderClient(auditGeo, auditForecast, auditSun, auditClock)
	transport := client.Transport
	client.Transport = auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/forecast" && (r.URL.Query().Has("start_date") || r.URL.Query().Has("end_date")) {
			t.Errorf("forecast constrained to process date: %s", r.URL)
		}
		return transport.RoundTrip(r)
	})
	_, _ = auditWeather(client).Get(context.Background(), "City")
}

func TestWeatherRejectsMalformedProviders(t *testing.T) {
	for _, tc := range []struct{ name, forecast string }{
		{"empty", `{}`}, {"null", `null`}, {"provider error", `{"error":true,"reason":"bad coordinates"}`},
		{"missing current", `{"timezone":"UTC"}`},
		{"bad current time", strings.Replace(auditForecast, "2026-09-11T12:15", "yesterday", 1)},
		{"bad sunrise", strings.Replace(auditForecast, "2026-09-11T06:30", "bad-sunrise", 1)},
		{"bad hourly axis", strings.Replace(auditForecast, "2026-09-11T12:00", "bad-hour", 1)},
		{"bad daily axis", strings.Replace(auditForecast, `"2026-09-11"`, `"bad-day"`, 1)},
		{"trailing JSON", auditForecast + ` {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if v := recover(); v != nil {
					t.Errorf("provider panic: %v", v)
				}
			}()
			got, err := auditWeather(auditProviderClient(auditGeo, tc.forecast, auditSun, auditClock)).Get(context.Background(), "City")
			if err == nil || got != "" {
				t.Fatalf("invalid forecast returned %q err=%v", got, err)
			}
		})
	}
}

func TestWeatherMissingCurrentMetricsAreUnavailable(t *testing.T) {
	forecast := `{"timezone":"UTC","current_weather":{"temperature":null,"time":"2026-09-11T12:15"}}`
	got, err := auditWeather(auditProviderClient(auditGeo, forecast, auditSun, auditClock)).Get(context.Background(), "City")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "☀️") || strings.Contains(got, "0001") || !strings.Contains(got, "unavailable") {
		t.Fatalf("missing data fabricated: %s", got)
	}
}

func TestTimeServicePreservesSignedMinuteOffsetsAndParsesLocalTimestamp(t *testing.T) {
	for _, tc := range []struct {
		offset     int
		want, zone string
	}{{330, "+05:30", "Asia/Kolkata"}, {345, "+05:45", "Asia/Kathmandu"}, {-210, "-03:30", "America/St_Johns"}, {0, "+00:00", "UTC"}} {
		sun := fmt.Sprintf(`{"results":{"date":"2026-01-11","timezone":%q,"utc_offset":%d}}`, tc.zone, tc.offset)
		clock := fmt.Sprintf(`{"dateTime":"2026-01-11T12:15:00.1234567","timeZone":%q}`, tc.zone)
		got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, sun, clock)).Get(context.Background(), "City")
		if err != nil || !strings.Contains(got, tc.want) || !strings.Contains(got, "Sun, 11 Jan 2026 12:15:00") || !strings.Contains(got, "unavailable") {
			t.Errorf("offset=%d got=%s err=%v", tc.offset, got, err)
		}
	}
}

func TestTimeServiceRejectsMalformedProviderData(t *testing.T) {
	for _, tc := range []struct{ name, sun, clock string }{
		{"empty clock", auditSun, `{}`}, {"invalid zone", auditSun, strings.Replace(auditClock, `"UTC"`, `"invalid/zone"`, 1)},
		{"invalid timestamp", auditSun, strings.Replace(auditClock, "2026-09-11T12:15:00.1234567", "invalid", 1)},
		{"empty sun", `{}`, auditClock}, {"sun business error", `{"status":"INVALID_REQUEST","results":null}`, auditClock},
		{"invalid day", strings.Replace(auditSun, "2026-09-11", "yesterday", 1), auditClock},
		{"invalid sunrise", `{"results":{"date":"2026-09-11","timezone":"UTC","sunrise":"soon"}}`, auditClock},
		{"invalid duration", `{"results":{"date":"2026-09-11","timezone":"UTC","day_length":"all day"}}`, auditClock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, tc.sun, tc.clock)).Get(context.Background(), "City")
			if err == nil || got != "" {
				t.Fatalf("invalid time data returned %q err=%v", got, err)
			}
		})
	}
}

func TestWeatherAndTimeServicePropagateHTTPAndDecodeFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{503, `{"error":"unavailable"}`}, {200, `not-json`}, {200, ``}} {
		client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
			response := auditJSON(tc.body)
			response.StatusCode = tc.status
			return response, nil
		})}
		for _, get := range []func(context.Context, string) (string, error){auditWeather(client).Get, auditTimeService(client).Get} {
			if got, err := get(context.Background(), "City"); err == nil || got != "" {
				t.Errorf("HTTP=%d body=%q returned %q err=%v", tc.status, tc.body, got, err)
			}
		}
	}
}

func TestWeatherAndTimeServiceValidateSecondaryURLs(t *testing.T) {
	client := auditProviderClient(auditGeo, auditForecast, auditSun, auditClock)
	w := auditWeather(client)
	w.ForecastURL = "http://[invalid"
	tm := auditTimeService(client)
	tm.SunriseURL = "http://[invalid"
	tz := auditTimeService(client)
	tz.TimezoneURL = "http://[invalid"
	for _, get := range []func(context.Context, string) (string, error){w.Get, tm.Get, tz.Get} {
		if got, err := get(context.Background(), "City"); err == nil || got != "" {
			t.Errorf("invalid secondary URL returned %q err=%v", got, err)
		}
	}
}

func TestWeatherAndTimeServiceValidateGeocoderAndURLs(t *testing.T) {
	for _, geo := range []string{`{}`, `{"status":{"value":18,"message":"quota exceeded"}}`, `{"geonames":[{"lat":"oops","lng":"2"}]}`} {
		client := auditProviderClient(geo, auditForecast, auditSun, auditClock)
		for _, get := range []func(context.Context, string) (string, error){auditWeather(client).Get, auditTimeService(client).Get} {
			if got, err := get(context.Background(), "City"); err == nil || got != "" {
				t.Errorf("invalid geocoder %s returned %q err=%v", geo, got, err)
			}
		}
	}
	for _, endpoint := range []string{"http://[invalid", "://bad", "ftp://test/geo"} {
		t.Run(endpoint, func(t *testing.T) {
			defer func() {
				if v := recover(); v != nil {
					t.Errorf("invalid URL panic: %v", v)
				}
			}()
			w := auditWeather(auditProviderClient(auditGeo, auditForecast, auditSun, auditClock))
			w.GeoURL = endpoint
			tm := auditTimeService(w.HTTP)
			tm.GeoURL = endpoint
			for _, get := range []func(context.Context, string) (string, error){w.Get, tm.Get} {
				if got, err := get(context.Background(), "City"); err == nil || got != "" {
					t.Errorf("invalid URL returned %q err=%v", got, err)
				}
			}
		})
	}
}

func TestWeatherAndTimeServiceReadsDoNotMutateConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"geonames":[]}`)) }))
	defer srv.Close()
	w := &service.WeatherService{GeoURL: srv.URL}
	tm := &service.TimeService{GeoURL: srv.URL}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = w.Get(context.Background(), "City")
			_, _ = tm.Get(context.Background(), "City")
		}()
	}
	wg.Wait()
	if w.HTTP != nil || tm.HTTP != nil {
		t.Fatal("Get mutated shared HTTP configuration")
	}
}

func TestWeatherAndTimeServicePropagateCancellation(t *testing.T) {
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	for _, get := range []func(context.Context, string) (string, error){auditWeather(client).Get, auditTimeService(client).Get} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got, err := get(ctx, "City"); !errors.Is(err, context.Canceled) || got != "" {
			t.Errorf("cancellation got=%q err=%v", got, err)
		}
	}
}

type auditCanceledReader struct{}

func (auditCanceledReader) Read([]byte) (int, error) { return 0, context.Canceled }

func TestWeatherAndTimeServicePropagateCancellationWhileReadingBody(t *testing.T) {
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		response := auditJSON(auditGeo)
		response.Body = io.NopCloser(io.MultiReader(strings.NewReader(auditGeo), auditCanceledReader{}))
		return response, nil
	})}
	for _, get := range []func(context.Context, string) (string, error){auditWeather(client).Get, auditTimeService(client).Get} {
		if got, err := get(context.Background(), "City"); !errors.Is(err, context.Canceled) || got != "" {
			t.Errorf("body cancellation got=%q err=%v", got, err)
		}
	}
}
