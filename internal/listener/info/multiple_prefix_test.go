package info

import (
	"context"
	"testing"
)

type multiplePrefixWhisperEngine struct{ *auditWhisperDispatchEngine }

func (*multiplePrefixWhisperEngine) GetPrefixes() []string { return []string{".", "*", ".."} }

func TestWhisperDispatchMatchesAllPrefixesLongestFirst(t *testing.T) {
	for _, text := range []string{".probe", "*probe", "..probe"} {
		engine, inv := newAuditWhisperDispatch()
		inv.Engine = &multiplePrefixWhisperEngine{engine}
		inv.ChatMessage.Text = text
		_, err := (DispatchWhisperCommand{}).Handle(context.Background(), inv)
		if err != nil || engine.command.calls != 1 {
			t.Fatalf("%s: calls=%d err=%v", text, engine.command.calls, err)
		}
	}
}
