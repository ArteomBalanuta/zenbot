package message

import (
	"context"
	"testing"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type multiplePrefixEngine struct{ *dispatchTestEngine }

func (*multiplePrefixEngine) GetPrefixes() []string { return []string{".", "*", ".."} }

func TestDispatchMatchesAllPrefixesLongestFirst(t *testing.T) {
	for _, input := range []string{".auth", "*auth", "..auth", "  *auth  "} {
		t.Run(input, func(t *testing.T) {
			executed := false
			cmd := &dispatchTestCommand{executed: &executed}
			e := &multiplePrefixEngine{&dispatchTestEngine{allowed: true, commands: map[string]common.CommandMetadata{"auth": {Alias: "auth", Command: func(*model.ChatMessage) common.Command { return cmd }}}}}
			_, err := (DispatchUserCommand{}).Handle(context.Background(), &Context{Engine: e, Message: &model.ChatMessage{Name: "alice", Text: input}, Author: &model.User{Name: "alice"}})
			if err != nil || !executed {
				t.Fatalf("command not dispatched: executed=%v err=%v", executed, err)
			}
		})
	}
}
