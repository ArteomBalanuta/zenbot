package command

import (
	"context"
	"testing"

	"zenbot/internal/model"
)

type prefixControllerStub struct {
	*commandEngineStub
	prefix string
	calls  []string
	err    error
}

func (s *prefixControllerStub) GetPrefix() string { return s.prefix }
func (s *prefixControllerStub) UpdatePrefix(next string) (string, error) {
	previous := s.prefix
	s.calls = append(s.calls, next)
	if s.err != nil {
		return previous, s.err
	}
	s.prefix = next
	return previous, nil
}

func TestPrefixCommandIsAdminConcreteAndPropagatesTrimmedFirstArgument(t *testing.T) {
	e := &prefixControllerStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}, prefix: "*"}
	definition, ok := commandDefinitionFor("prefix")
	if !ok {
		t.Fatal("prefix definition missing")
	}
	command := definition.New(e, &model.ChatMessage{Text: "*prefix  $ ignored", Name: "admin", IsWhisper: true})
	if command.Role() != model.ADMIN || len(command.Aliases()) != 1 || command.Aliases()[0] != "prefix" {
		t.Fatalf("role=%v aliases=%v", command.Role(), command.Aliases())
	}
	if _, generic := command.(*saturnCommand); generic {
		t.Fatal("prefix must be a concrete command")
	}
	status, err := command.Execute(context.Background())
	if err != nil || status != model.SUCCESSFUL {
		t.Fatalf("status=%v err=%v", status, err)
	}
	if len(e.calls) != 1 || e.calls[0] != "$" || e.prefix != "$" {
		t.Fatalf("calls=%v prefix=%q", e.calls, e.prefix)
	}
	if len(e.chats) != 1 || e.chats[0] != "admin|prefix changed from * to $|true" {
		t.Fatalf("chats=%v", e.chats)
	}
}

func TestPrefixCommandMissingOrBlankArgumentUsesCurrentPrefixExampleWithoutMutation(t *testing.T) {
	for _, text := range []string{"*prefix", "*prefix    "} {
		t.Run(text, func(t *testing.T) {
			e := &prefixControllerStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}, prefix: "*"}
			definition, _ := commandDefinitionFor("prefix")
			status, err := definition.New(e, &model.ChatMessage{Text: text, Name: "admin"}).Execute(context.Background())
			if err != nil || status != model.FAILED || len(e.calls) != 0 || e.prefix != "*" {
				t.Fatalf("status=%v err=%v calls=%v prefix=%q", status, err, e.calls, e.prefix)
			}
			if len(e.chats) != 1 || e.chats[0] != "admin|Example: *prefix $|false" {
				t.Fatalf("chats=%v", e.chats)
			}
		})
	}
}

func TestPrefixCommandCancelledHasNoMutationOrReply(t *testing.T) {
	e := &prefixControllerStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}, prefix: "*"}
	definition, _ := commandDefinitionFor("prefix")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := definition.New(e, &model.ChatMessage{Text: "*prefix $", Name: "admin"}).Execute(ctx)
	if err == nil || status != model.FAILED || len(e.calls) != 0 || len(e.chats) != 0 || e.prefix != "*" {
		t.Fatalf("status=%v err=%v calls=%v chats=%v prefix=%q", status, err, e.calls, e.chats, e.prefix)
	}
}

func TestPrefixCommandWithoutCapabilityFailsWithoutSuccessReply(t *testing.T) {
	e := &commandEngineStub{users: map[string]*model.User{}}
	definition, _ := commandDefinitionFor("prefix")
	status, err := definition.New(e, &model.ChatMessage{Text: "!prefix $", Name: "admin"}).Execute(context.Background())
	if err == nil || status != model.FAILED || len(e.chats) != 0 {
		t.Fatalf("status=%v err=%v chats=%v", status, err, e.chats)
	}
}

func TestPrefixIsNotRegisteredWithoutPrefixController(t *testing.T) {
	without := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	if _, ok := without.commands["prefix"]; ok {
		t.Fatal("prefix registered without PrefixController")
	}

	with := &prefixControllerStub{commandEngineStub: &commandEngineStub{users: map[string]*model.User{}}, prefix: "*"}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	metadata, ok := with.commands["prefix"]
	if !ok {
		t.Fatal("prefix not registered with PrefixController")
	}
	if role := metadata.Command(&model.ChatMessage{}).GetRole(); role == nil || *role != model.ADMIN {
		t.Fatalf("role=%v", role)
	}
}
