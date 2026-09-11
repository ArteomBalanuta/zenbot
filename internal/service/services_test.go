package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
		_, _ = w.Write([]byte(`{"timezone":"UTC","current_weather":{"temperature":21,"windspeed":7,"weathercode":0,"time":"2026-03-26T12:00"},"current_weather_units":{"temperature":"C","windspeed":"km/h"},"daily":{"time":["2026-03-26"],"sunrise":["2026-03-26T06:30"],"sunset":["2026-03-26T18:45"],"uv_index_max":["4"],"shortwave_radiation_sum":["12"]},"daily_units":{"uv_index_max":"idx","shortwave_radiation_sum":"MJ/m2"},"hourly":{"time":["2026-03-26T12:00"],"apparent_temperature":["20"],"relative_humidity_2m":["35"],"surface_pressure":["1008"],"pressure_msl":["1014"],"shortwave_radiation":["500"],"diffuse_radiation":["120"],"soil_temperature_18cm":["15"],"soil_moisture_3_to_9cm":["0.22"]},"hourly_units":{"apparent_temperature":"C","relative_humidity_2m":"%","surface_pressure":"hPa","pressure_msl":"hPa","shortwave_radiation":"W/m2","diffuse_radiation":"W/m2","soil_temperature_18cm":"C","soil_moisture_3_to_9cm":"m3/m3"}}`))
	}))
	defer srv.Close()
	got, e := (&WeatherService{HTTP: srv.Client(), GeoURL: srv.URL, ForecastURL: srv.URL, Now: func() time.Time { return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC) }}).Get(context.Background(), "Paris")
	if e != nil {
		t.Fatalf("got %q err %v", got, e)
	}
	lines := nonEmptyLiteralLines(got)
	if len(lines) != 17 {
		t.Fatalf("weather lines=%d: %q", len(lines), got)
	}
	assertAlignedSeparator(t, lines)
	if !strings.Contains(got, "\u2009Temperature:") || !strings.Contains(got, "Weather forecast for today:") {
		t.Fatalf("weather output is not thin-space aligned: %q", got)
	}
}

func TestTimeGetUsesSingleSeparatorsAndThinSpaceAlignment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/geo":
			_, _ = w.Write([]byte(`{"geonames":[{"countryName":"Japan","lat":"35.6","lng":"139.6"}]}`))
		case "/sun":
			_, _ = w.Write([]byte(`{"results":{"date":"2026-03-26","timezone":"Asia/Tokyo","sunrise":"6:00 AM","sunset":"6:00 PM","first_light":"5:30 AM","last_light":"6:30 PM","dawn":"5:45 AM","dusk":"6:15 PM","solar_noon":"12:00 PM","golden_hour":"5:15 PM","day_length":"12:00:00","utc_offset":540}}`))
		case "/time":
			_, _ = w.Write([]byte(`{"dateTime":"2026-03-26T12:00:00+09:00","timeZone":"Asia/Tokyo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	got, err := (&TimeService{
		HTTP:        srv.Client(),
		GeoURL:      srv.URL + "/geo",
		SunriseURL:  srv.URL + "/sun?lat=%s&lng=%s",
		TimezoneURL: srv.URL + "/time?latitude=%s&longitude=%s",
	}).Get(context.Background(), "Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("time output contains blank formatted rows: %q", got)
	}
	parts := strings.Split(got, "\n")
	if len(parts) != 16 || parts[0] != "" || !strings.HasPrefix(parts[2], " today") || parts[len(parts)-1] != "" {
		t.Fatalf("time framing=%q", got)
	}
	aligned := append([]string{strings.TrimPrefix(parts[2], " ")}, parts[3:len(parts)-1]...)
	assertAlignedSeparator(t, aligned)
	if !strings.Contains(got, "today\u2009") {
		t.Fatalf("time labels are not thin-space aligned: %q", got)
	}
}

func nonEmptyLiteralLines(value string) []string {
	parts := strings.Split(value, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			lines = append(lines, part)
		}
	}
	return lines
}

func assertAlignedSeparator(t *testing.T, lines []string) {
	t.Helper()
	want := -1
	for _, line := range lines {
		index := strings.IndexRune(line, ':')
		if index < 0 {
			t.Fatalf("line has no separator: %q", line)
		}
		width := utf8.RuneCountInString(line[:index])
		if want < 0 {
			want = width
		} else if width != want {
			t.Fatalf("separator width=%d, want %d in line %q", width, want, line)
		}
	}
}
func TestSearchUsesDuckDuckGoCompatibleEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"AbstractText":"answer"}`))
	}))
	defer srv.Close()
	got, e := (&SearchService{HTTP: srv.Client(), Endpoint: srv.URL}).Search(context.Background(), "a b")
	if e != nil || got != `{"AbstractText":"answer"}` {
		t.Fatalf("got %q err %v", got, e)
	}
}

func TestYouTubePreviewExtractsSupportedLinksAndFormatsMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://youtube.com/watch?v=abc123" {
			t.Errorf("url=%q", r.URL.Query().Get("url"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"A title"}`))
	}))
	defer srv.Close()

	service := &YouTubeService{HTTP: srv.Client(), Endpoint: srv.URL}
	for _, message := range []string{
		"watch https://www.youtube.com/watch?v=abc123&list=queue",
		"watch https://youtu.be/abc123?list=queue",
	} {
		preview, found, err := service.Preview(context.Background(), message)
		if err != nil || !found {
			t.Fatalf("message=%q found=%v err=%v", message, found, err)
		}
		want := "Title: A title\n![A title](https://i.ytimg.com/vi/abc123/hqdefault.jpg)"
		if preview != want {
			t.Fatalf("preview=%q, want %q", preview, want)
		}
	}

	if preview, found, err := service.Preview(context.Background(), "ordinary text"); err != nil || found || preview != "" {
		t.Fatalf("ordinary preview=%q found=%v err=%v", preview, found, err)
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
	if e != nil || d < 0 {
		t.Fatalf("duration=%v err=%v", d, e)
	}
}
