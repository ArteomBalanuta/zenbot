package profiling

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPprofServerExposesRuntimeProfilesAndClosesCleanly(t *testing.T) {
	server, err := StartServer(context.Background(), ServerConfig{
		Address:              "127.0.0.1:0",
		BlockProfileRate:     1,
		MutexProfileFraction: 1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + server.Address() + "/debug/pprof/goroutine?debug=1")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "goroutine profile") {
		t.Fatalf("status=%d body=%q", response.StatusCode, body)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
