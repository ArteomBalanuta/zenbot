package listener_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"zenbot/internal/command"
	"zenbot/internal/common"
	"zenbot/internal/core"
	"zenbot/internal/listener"
	"zenbot/internal/model"
)

type lifecycleCommandController struct {
	restarts  int
	shutdowns int
}

func (c *lifecycleCommandController) RequestRestart(context.Context) error {
	c.restarts++
	return nil
}

func (c *lifecycleCommandController) RequestShutdown(context.Context) error {
	c.shutdowns++
	return nil
}

type lifecycleCommandEngine struct {
	common.Engine
	commands           map[string]common.CommandMetadata
	active             map[*model.User]struct{}
	allowed            bool
	chats              int
	controller         *lifecycleCommandController
	dispatchController *dispatchAwareFailingLifecycleController
}

func (e *lifecycleCommandEngine) HostLifecycleController() common.HostLifecycleController {
	if e.dispatchController != nil {
		return e.dispatchController
	}
	return e.controller
}

type dispatchAwareFailingLifecycleController struct {
	worker  *core.HostLifecycle
	started <-chan struct{}
	failure error
}

func (c *dispatchAwareFailingLifecycleController) BeginDispatch() func() {
	return c.worker.BeginDispatch()
}

func (c *dispatchAwareFailingLifecycleController) RequestRestart(ctx context.Context) error {
	if err := c.worker.RequestRestart(ctx); err != nil {
		return err
	}
	select {
	case <-c.started:
		return errors.New("restart ran before listener dispatch returned")
	case <-time.After(100 * time.Millisecond):
	}
	return c.failure
}

func (c *dispatchAwareFailingLifecycleController) RequestShutdown(ctx context.Context) error {
	return c.worker.RequestShutdown(ctx)
}

func (e *lifecycleCommandEngine) GetPrefix() string  { return "!" }
func (e *lifecycleCommandEngine) GetName() string    { return "zenbot" }
func (e *lifecycleCommandEngine) GetChannel() string { return "test" }
func (e *lifecycleCommandEngine) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &e.commands
}
func (e *lifecycleCommandEngine) GetActiveUsers() *map[*model.User]struct{} { return &e.active }
func (e *lifecycleCommandEngine) GetActiveUserByName(name string) *model.User {
	for user := range e.active {
		if strings.EqualFold(user.Name, name) {
			return user
		}
	}
	return nil
}
func (e *lifecycleCommandEngine) RegisterCommand(c common.Command) {
	for _, alias := range c.GetAliases() {
		registered := c
		e.commands[strings.ToLower(alias)] = common.CommandMetadata{
			Alias: alias,
			Command: func(message *model.ChatMessage) common.Command {
				return registered.NewInstance(e, message)
			},
		}
	}
}
func (e *lifecycleCommandEngine) IsUserAuthorized(*model.User, *model.Role) bool { return e.allowed }
func (e *lifecycleCommandEngine) SendChatMessage(string, string, bool) (string, error) {
	e.chats++
	return "", nil
}
func (*lifecycleCommandEngine) LogMessage(string, string, string, string, string) (int64, error) {
	return 0, nil
}
func (*lifecycleCommandEngine) RemoveIfAfk(*model.User)                 {}
func (*lifecycleCommandEngine) NotifyAfkIfMentioned(*model.ChatMessage) {}

func newLifecycleCommandEngine(allowed bool) *lifecycleCommandEngine {
	return &lifecycleCommandEngine{
		commands:   map[string]common.CommandMetadata{},
		active:     map[*model.User]struct{}{},
		allowed:    allowed,
		controller: &lifecycleCommandController{},
	}
}

func TestUserChatListenerLifecycleCommandsInvokeControllerWithoutReplies(t *testing.T) {
	for _, test := range []struct {
		alias        string
		wantRestarts int
		wantShutdown int
	}{
		{alias: "restart", wantRestarts: 1},
		{alias: "reload", wantRestarts: 1},
		{alias: "re", wantRestarts: 1},
		{alias: "exit", wantShutdown: 1},
		{alias: "quit", wantShutdown: 1},
		{alias: "shutdown", wantShutdown: 1},
	} {
		t.Run(test.alias, func(t *testing.T) {
			engine := newLifecycleCommandEngine(true)
			user := &model.User{Name: "alice"}
			engine.active[user] = struct{}{}
			if err := command.RegisterUserUtilities(engine); err != nil {
				t.Fatal(err)
			}

			listener.NewUserChatListener(engine).Notify(`{"cmd":"chat","nick":"alice","text":"!` + test.alias + ` ignored arguments"}`)

			if engine.controller.restarts != test.wantRestarts || engine.controller.shutdowns != test.wantShutdown || engine.chats != 0 {
				t.Fatalf("restart calls=%d shutdown calls=%d chats=%d", engine.controller.restarts, engine.controller.shutdowns, engine.chats)
			}
		})
	}

	t.Run("unauthorized", func(t *testing.T) {
		engine := newLifecycleCommandEngine(false)
		user := &model.User{Name: "alice"}
		engine.active[user] = struct{}{}
		if err := command.RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}

		listener.NewUserChatListener(engine).Notify(`{"cmd":"chat","nick":"alice","text":"!restart ignored"}`)

		if engine.controller.restarts != 0 || engine.controller.shutdowns != 0 || engine.chats != 1 {
			t.Fatalf("restart calls=%d shutdown calls=%d chats=%d", engine.controller.restarts, engine.controller.shutdowns, engine.chats)
		}
	})

	t.Run("cancelled direct execution", func(t *testing.T) {
		engine := newLifecycleCommandEngine(true)
		if err := command.RegisterUserUtilities(engine); err != nil {
			t.Fatal(err)
		}
		cmd := engine.commands["restart"].Command(&model.ChatMessage{Name: "alice", Text: "!restart ignored"})
		contextual, ok := cmd.(interface{ ExecuteContext(context.Context) })
		if !ok {
			t.Fatalf("command %T does not accept dispatch context", cmd)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		contextual.ExecuteContext(ctx)

		if engine.controller.restarts != 0 || engine.controller.shutdowns != 0 || engine.chats != 0 {
			t.Fatalf("restart calls=%d shutdown calls=%d chats=%d", engine.controller.restarts, engine.controller.shutdowns, engine.chats)
		}
	})
}

func TestUserChatListenerLifecycleFailureIsLoggedAndWorkerWaitsForDispatchReturn(t *testing.T) {
	started := make(chan struct{})
	worker := core.NewHostLifecycle(func(context.Context) error {
		close(started)
		return nil
	}, nil)
	defer worker.Close()
	engine := newLifecycleCommandEngine(true)
	engine.dispatchController = &dispatchAwareFailingLifecycleController{
		worker: worker, started: started, failure: errors.New("controller unavailable"),
	}
	user := &model.User{Name: "alice"}
	engine.active[user] = struct{}{}
	if err := command.RegisterUserUtilities(engine); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	listener.NewUserChatListener(engine).Notify(`{"cmd":"chat","nick":"alice","text":"!restart"}`)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("lifecycle worker did not begin after listener dispatch returned")
	}
	worker.Wait()
	if engine.chats != 0 {
		t.Fatalf("chats=%d, want no lifecycle reply", engine.chats)
	}
	if !strings.Contains(logs.String(), "controller unavailable") {
		t.Fatalf("controller failure was not logged: %q", logs.String())
	}
	if !strings.Contains(logs.String(), `Saturn command "restart" failed with status FAILED`) {
		t.Fatalf("listener did not preserve the typed lifecycle failure: %q", logs.String())
	}
}
