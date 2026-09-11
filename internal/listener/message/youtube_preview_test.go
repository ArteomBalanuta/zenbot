package message

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

type youtubePreviewEngine struct {
	common.Engine
	bundle *service.Bundle
	author string
	text   string
}

func (e *youtubePreviewEngine) ServiceBundle() *service.Bundle { return e.bundle }
func (e *youtubePreviewEngine) SendChatMessage(author, text string, _ bool) (string, error) {
	e.author, e.text = author, text
	return text, nil
}

func TestYoutubePreviewHandlerPublishesPreviewForAuthor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"Video"}`))
	}))
	defer srv.Close()
	engine := &youtubePreviewEngine{bundle: &service.Bundle{YouTube: &service.YouTubeService{HTTP: srv.Client(), Endpoint: srv.URL}}}

	next, err := (YoutubePreview{}).Handle(context.Background(), &Context{Engine: engine, Message: &model.ChatMessage{Name: "alice", Text: "https://youtu.be/id"}})

	if err != nil || !next || engine.author != "alice" || engine.text != "Title: Video\n![Video](https://i.ytimg.com/vi/id/hqdefault.jpg)" {
		t.Fatalf("next=%v err=%v author=%q text=%q", next, err, engine.author, engine.text)
	}
}
