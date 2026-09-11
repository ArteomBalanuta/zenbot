package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type recentPlainTextQueries struct{ lastOnlineQueriesStub }

func (recentPlainTextQueries) RecentPresenceNames(context.Context, string, string, int64, int) ([]string, error) {
	return []string{"mer"}, nil
}

func TestSeenRecentlyProducesRealNewlines(t *testing.T) {
	s := &UserService{Queries: &recentPlainTextQueries{}}
	got, err := s.SeenRecently(context.Background(), &model.User{Name: "merc"})
	if want := "\n @merc, has been seen as: _mer_ recently. \n"; err != nil || got != want {
		t.Fatalf("got=%q err=%v want=%q", got, err, want)
	}
}

func TestLastOnlinePreservesPublicMessageText(t *testing.T) {
	message := "quote \" literal \\n actual\nnew 🌦️\t\x01"
	s := &UserService{Queries: &lastOnlineQueriesStub{record: repository.LastOnlineRecord{
		Found: true, LastMessage: sql.NullString{String: message, Valid: true},
		LastMessageMillis: sql.NullInt64{Int64: 0, Valid: true},
	}}}
	got, err := s.LastOnline(context.Background(), "merc")
	if err != nil || !strings.Contains(got, " — "+message+"\n") || !strings.HasPrefix(got, "\n Nick|Trip: merc\n") {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestExternalServicesProducePlainText(t *testing.T) {
	title := "first\nsecond \\n \"Chișinău\""
	search := "{\n  \"text\": \"literal \\\\n\"\n}"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtube" {
			_ = json.NewEncoder(w).Encode(map[string]string{"title": title})
			return
		}
		_, _ = w.Write([]byte(search))
	}))
	defer srv.Close()
	t.Run("youtube", func(t *testing.T) {
		got, found, err := (&YouTubeService{HTTP: srv.Client(), Endpoint: srv.URL + "/youtube"}).Preview(context.Background(), "https://youtu.be/id")
		want := "Title: " + title + "\n![" + title + "](https://i.ytimg.com/vi/id/hqdefault.jpg)"
		if err != nil || !found || got != want {
			t.Fatalf("got=%q err=%v want=%q", got, err, want)
		}
	})
	t.Run("search", func(t *testing.T) {
		got, err := (&SearchService{HTTP: srv.Client(), Endpoint: srv.URL}).Search(context.Background(), "query")
		if err != nil || got != search {
			t.Fatalf("got=%q err=%v want=%q", got, err, search)
		}
	})
}

func TestSearchPreservesQueryReservedCharacters(t *testing.T) {
	const query = "Chișinău &other=value +slash/ #fragment"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != query {
			t.Errorf("query=%q want=%q", got, query)
		}
		if r.URL.Query().Get("other") != "" {
			t.Error("query content injected a provider parameter")
		}
		_, _ = w.Write([]byte(`{"text":"ok"}`))
	}))
	defer srv.Close()
	if _, err := (&SearchService{HTTP: srv.Client(), Endpoint: srv.URL}).Search(context.Background(), query); err != nil {
		t.Fatal(err)
	}
}

func TestWeatherPreservesUnicodeAndReservedLocationQuery(t *testing.T) {
	const location = "São Paulo &country=GB +path/ #雪"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/geo" {
			if got := r.URL.Query().Get("q"); got != location || r.URL.Query().Has("country") {
				t.Errorf("location query corrupted: %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"geonames":[{"name":"São Paulo","countryName":"Brazil","lat":"-23.55","lng":"-46.63"}]}`))
			return
		}
		if r.URL.Query().Get("latitude") != "-23.55" || r.URL.Query().Get("longitude") != "-46.63" {
			t.Errorf("coordinates corrupted: %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"timezone":"America/Sao_Paulo","current_weather":{"time":"2026-09-11T12:00"}}`))
	}))
	defer srv.Close()
	got, err := (&WeatherService{HTTP: srv.Client(), GeoURL: srv.URL + "/geo", ForecastURL: srv.URL + "/forecast"}).Get(context.Background(), location)
	if err != nil || !strings.Contains(got, "**São Paulo, Brazil**\n") {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
