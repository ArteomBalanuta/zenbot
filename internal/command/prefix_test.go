package command

import (
	"context"
	"testing"

	"zenbot/internal/model"
)

type prefixEngineStub struct {
	commandEngineStub
	prefix string
}

func (e *prefixEngineStub) GetPrefix() string       { return e.prefix }
func (e *prefixEngineStub) SetPrefix(prefix string) { e.prefix = prefix }

func TestPrefixCommandUpdatesSupportedEngineAndReplies(t *testing.T) {
	engine := &prefixEngineStub{prefix: "!"}
	message := &model.ChatMessage{Name: "admin", Text: "prefix $"}
	command := &prefixCommand{commandBase: commandBase{engine: engine, message: message}}
	status, err := command.Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL || engine.prefix != "$" {
		t.Fatalf("status=%v err=%v prefix=%q", status, err, engine.prefix)
	}
}

func TestPrefixCommandRejectsBlankPrefix(t *testing.T) {
	engine := &prefixEngineStub{prefix: "!"}
	message := &model.ChatMessage{Name: "admin", Text: "prefix  "}
	command := &prefixCommand{commandBase: commandBase{engine: engine, message: message}}
	status, err := command.Execute(context.Background())
	if err != nil || status != model.FAILED || engine.prefix != "!" {
		t.Fatalf("status=%v err=%v prefix=%q", status, err, engine.prefix)
	}
}
