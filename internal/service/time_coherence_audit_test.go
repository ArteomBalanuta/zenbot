package service_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTimeCoherenceRequestsClockDayAndZoneAcrossMidnight(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/geo":
			return auditJSON(auditGeo), nil
		case "/clock":
			return auditJSON(`{"dateTime":"2026-09-12T00:00:00+05:45","timeZone":"Asia/Kathmandu"}`), nil
		case "/sun":
			date := r.URL.Query().Get("date")
			if date != "2026-09-12" || r.URL.Query().Get("timezone") != "Asia/Kathmandu" {
				t.Errorf("solar request is not tied to clock: %s", r.URL)
				date = "2026-09-11" // Provider's default day just before local midnight.
			}
			return auditJSON(fmt.Sprintf(`{"status":"OK","tzid":"Asia/Kathmandu","results":{"date":%q,"timezone":"Asia/Kathmandu","utc_offset":345}}`, date)), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
	})}
	got, err := auditTimeService(client).Get(context.Background(), "City")
	if err != nil || !strings.Contains(got, "2026-09-12") || strings.Contains(got, "2026-09-11") || !strings.Contains(got, "+05:45") {
		t.Fatalf("inconsistent day/clock: %q err=%v", got, err)
	}
	if strings.Join(paths, ",") != "/geo,/clock,/sun" {
		t.Errorf("request order: %v", paths)
	}
}

func TestTimeCoherenceRejectsConflictingSolarMetadata(t *testing.T) {
	for _, tc := range []struct{ name, sun string }{
		{"previous day", `{"results":{"date":"2026-09-10","timezone":"UTC","utc_offset":0}}`},
		{"different zone same offset", `{"results":{"date":"2026-09-11","timezone":"Africa/Abidjan","utc_offset":0}}`},
		{"missing zone", `{"results":{"date":"2026-09-11","utc_offset":0}}`},
		{"conflicting wrapper zone", `{"tzid":"Asia/Kolkata","results":{"date":"2026-09-11","timezone":"UTC","utc_offset":0}}`},
		{"conflicting offset", `{"results":{"date":"2026-09-11","timezone":"UTC","utc_offset":330}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, tc.sun, auditClock)).Get(context.Background(), "City")
			if err == nil || got != "" {
				t.Fatalf("conflicting solar metadata accepted: %q err=%v", got, err)
			}
		})
	}
}

func TestTimeCoherenceDerivesMissingSolarOffsetFromCurrentClock(t *testing.T) {
	sun := `{"results":{"date":"2026-09-11","timezone":"Asia/Kathmandu"}}`
	clock := `{"dateTime":"2026-09-11T12:15:00+05:45","timeZone":"Asia/Kathmandu"}`
	got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, sun, clock)).Get(context.Background(), "City")
	if err != nil || !strings.Contains(got, "+05:45") {
		t.Fatalf("current offset unavailable despite valid clock: %q err=%v", got, err)
	}
}

func TestTimeCoherenceAllowsSolarDayOffsetAcrossDSTTransition(t *testing.T) {
	sun := `{"results":{"date":"2026-03-08","timezone":"America/New_York","utc_offset":-240,"sunrise":"7:18:00 AM"}}`
	clock := `{"dateTime":"2026-03-08T00:15:00-05:00","timeZone":"America/New_York"}`
	got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, sun, clock)).Get(context.Background(), "City")
	if err != nil || !strings.Contains(got, "-05:00") || strings.Contains(got, "-04:00") || !strings.Contains(got, "7:18:00 AM") {
		t.Fatalf("valid transition-day solar data rejected or current offset changed: %q err=%v", got, err)
	}
}

func TestTimeCoherenceValidatesSolarOffsetWhenLocalMidnightIsSkipped(t *testing.T) {
	clock := `{"dateTime":"2026-09-06T12:15:00-03:00","timeZone":"America/Santiago"}`
	for _, tc := range []struct {
		name   string
		offset int
		valid  bool
	}{
		{"previous date offset", -240, false},
		{"requested date offset", -180, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sun := fmt.Sprintf(`{"results":{"date":"2026-09-06","timezone":"America/Santiago","utc_offset":%d}}`, tc.offset)
			got, err := auditTimeService(auditProviderClient(auditGeo, auditForecast, sun, clock)).Get(context.Background(), "City")
			if tc.valid {
				if err != nil || !strings.Contains(got, "2026-09-06") || !strings.Contains(got, "-03:00") {
					t.Fatalf("valid offset rejected or wrong current date/offset: %q err=%v", got, err)
				}
			} else if err == nil || got != "" {
				t.Fatalf("offset %d never occurs on requested date but was accepted: err=%v", tc.offset, err)
			}
		})
	}
}

type auditCountedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *auditCountedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *auditCountedBody) Close() error { b.closed = true; return nil }

func TestUtilityHTTPRejectsOversizedBodiesWithBoundedRead(t *testing.T) {
	for _, path := range []string{"/geo", "/forecast", "/clock", "/sun"} {
		t.Run(path, func(t *testing.T) {
			client := auditProviderClient(auditGeo, auditForecast, auditSun, auditClock)
			transport := client.Transport
			var body *auditCountedBody
			client.Transport = auditRoundTripper(func(r *http.Request) (*http.Response, error) {
				response, err := transport.RoundTrip(r)
				if err == nil && r.URL.Path == path {
					original, _ := io.ReadAll(response.Body)
					response.Body.Close()
					payload := string(original[:len(original)-1]) + `,"padding":"` + strings.Repeat("x", 2<<20) + `"}`
					body = &auditCountedBody{Reader: strings.NewReader(payload)}
					response.Body = body
					response.ContentLength = -1 // Streaming responses must also be bounded.
				}
				return response, err
			})
			get := auditTimeService(client).Get
			if path == "/forecast" {
				get = auditWeather(client).Get
			}
			got, err := get(context.Background(), "City")
			if err == nil || got != "" {
				t.Errorf("oversized body accepted: err=%v", err)
			}
			if body == nil || body.read > (1<<20)+1 || !body.closed {
				t.Errorf("response was not read within 1 MiB limit and closed: %+v", body)
			}
		})
	}
}
