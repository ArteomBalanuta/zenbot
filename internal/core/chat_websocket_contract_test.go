package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"zenbot/internal/service"
	"zenbot/internal/transport"
)

func TestMultilineExternalTextSurvivesWebSocketAndNextMessage(t *testing.T) {
	const title = "video\nmultiline \\n description\r\n\"quoted\"\t\x00 Chișinău 🌦️"
	frames := make(chan string, 2)
	failures := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metadata" {
			_ = json.NewEncoder(w).Encode(map[string]string{"title": title})
			return
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			failures <- err
			return
		}
		defer ws.Close()
		_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		for i := 0; i < 2; i++ {
			_, frame, err := ws.ReadMessage()
			if err != nil {
				failures <- err
				return
			}
			var message struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(frame, &message); err != nil {
				failures <- err
				return
			}
			frames <- message.Text
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := transport.NewConnection(transport.Config{URL: "ws" + strings.TrimPrefix(srv.URL, "http")})
	if err := conn.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	e := &EngineImpl{Transport: conn}
	preview, found, err := (&service.YouTubeService{HTTP: srv.Client(), Endpoint: srv.URL + "/metadata"}).Preview(ctx, "https://youtu.be/id")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if _, err := e.SendChatMessage("merc", preview, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.SendChatMessage("", "still connected", false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@merc Title: " + title + "\n![" + title + "](https://i.ytimg.com/vi/id/hqdefault.jpg)", "still connected"} {
		select {
		case got := <-frames:
			if got != want {
				t.Fatalf("got=%q want=%q", got, want)
			}
		case err := <-failures:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
