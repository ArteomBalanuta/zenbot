package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
)

type lifecycleCommandEngineStub struct {
	*commandEngineStub
	controller common.HostLifecycleController
}

func (s *lifecycleCommandEngineStub) HostLifecycleController() common.HostLifecycleController {
	return s.controller
}

type recordingLifecycleController struct {
	restartCalls  int
	shutdownCalls int
	restartErr    error
	shutdownErr   error
}

func (c *recordingLifecycleController) RequestRestart(context.Context) error {
	c.restartCalls++
	return c.restartErr
}

func (c *recordingLifecycleController) RequestShutdown(context.Context) error {
	c.shutdownCalls++
	return c.shutdownErr
}

func TestRestartShutdownDirectExecutionWithoutControllerFailsClosed(t *testing.T) {
	cases := []struct {
		canonical string
	}{
		{canonical: "restart"},
		{canonical: "shutdown"},
	}
	for _, tt := range cases {
		t.Run(tt.canonical, func(t *testing.T) {
			engine := &commandEngineStub{users: map[string]*model.User{}}
			command := newCommand(tt.canonical, nil, model.ADMIN, engine,
				&model.ChatMessage{Text: "!" + tt.canonical, Name: "alice"})

			status, err := command.Execute(context.Background())

			if status != model.FAILED {
				t.Errorf("status=%v, want FAILED", status)
			}
			if err == nil || err.Error() != "host lifecycle controller is not configured" {
				t.Errorf("err=%v, want host lifecycle controller is not configured", err)
			}
			if len(engine.chats) != 0 || len(engine.raws) != 0 {
				t.Errorf("chats=%d raws=%d, want no output", len(engine.chats), len(engine.raws))
			}
		})
	}
}

func TestRestartShutdownRegistrationIsCapabilityGatedAndConcrete(t *testing.T) {
	without := &commandEngineStub{users: map[string]*model.User{}}
	if err := RegisterUserUtilities(without); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"restart", "reload", "re", "exit", "quit", "shutdown"} {
		if _, found := (*without.GetEnabledCommands())[alias]; found {
			t.Errorf("missing lifecycle capability registered %q", alias)
		}
	}

	with := &lifecycleCommandEngineStub{
		commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
		controller:        &recordingLifecycleController{},
	}
	if err := RegisterUserUtilities(with); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		alias  string
		typeOf any
	}{
		{"restart", &restartCommand{}}, {"reload", &restartCommand{}}, {"re", &restartCommand{}},
		{"exit", &shutdownCommand{}}, {"quit", &shutdownCommand{}}, {"shutdown", &shutdownCommand{}},
	} {
		metadata, found := (*with.GetEnabledCommands())[want.alias]
		if !found {
			t.Errorf("lifecycle capability omitted %q", want.alias)
			continue
		}
		command := metadata.Command(&model.ChatMessage{})
		if command.GetRole() == nil || *command.GetRole() != model.ADMIN {
			t.Errorf("%q role=%v, want ADMIN", want.alias, command.GetRole())
		}
		switch want.typeOf.(type) {
		case *restartCommand:
			if _, ok := command.(*legacyAdapter).def.New(with, &model.ChatMessage{}).(*restartCommand); !ok {
				t.Errorf("%q is not concrete restart command", want.alias)
			}
		case *shutdownCommand:
			if _, ok := command.(*legacyAdapter).def.New(with, &model.ChatMessage{}).(*shutdownCommand); !ok {
				t.Errorf("%q is not concrete shutdown command", want.alias)
			}
		}
	}
}

func TestRestartShutdownDirectExecutionPreservesControllerBehavior(t *testing.T) {
	requestFailure := errors.New("request failed")
	cases := []struct {
		name          string
		canonical     string
		cancelled     bool
		controllerErr error
		wantStatus    model.Status
		wantErr       error
		wantRestart   int
		wantShutdown  int
	}{
		{name: "restart cancellation wins", canonical: "restart", cancelled: true, wantStatus: model.FAILED, wantErr: context.Canceled},
		{name: "shutdown cancellation wins", canonical: "shutdown", cancelled: true, wantStatus: model.FAILED, wantErr: context.Canceled},
		{name: "restart success", canonical: "restart", wantStatus: model.SUCCESSFUL, wantRestart: 1},
		{name: "shutdown success", canonical: "shutdown", wantStatus: model.SUCCESSFUL, wantShutdown: 1},
		{name: "restart request failure is swallowed", canonical: "restart", controllerErr: requestFailure, wantStatus: model.SUCCESSFUL, wantRestart: 1},
		{name: "shutdown request failure is swallowed", canonical: "shutdown", controllerErr: requestFailure, wantStatus: model.SUCCESSFUL, wantShutdown: 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			controller := &recordingLifecycleController{}
			if tt.canonical == "restart" {
				controller.restartErr = tt.controllerErr
			} else {
				controller.shutdownErr = tt.controllerErr
			}
			engine := &lifecycleCommandEngineStub{
				commandEngineStub: &commandEngineStub{users: map[string]*model.User{}},
				controller:        controller,
			}
			command := newCommand(tt.canonical, nil, model.ADMIN, engine,
				&model.ChatMessage{Text: "!" + tt.canonical, Name: "alice"})
			ctx := context.Background()
			if tt.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			status, err := command.Execute(ctx)

			if status != tt.wantStatus {
				t.Errorf("status=%v, want %v", status, tt.wantStatus)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err=%v, want %v", err, tt.wantErr)
			}
			if controller.restartCalls != tt.wantRestart || controller.shutdownCalls != tt.wantShutdown {
				t.Errorf("restart calls=%d shutdown calls=%d, want %d %d", controller.restartCalls, controller.shutdownCalls, tt.wantRestart, tt.wantShutdown)
			}
			if len(engine.chats) != 0 || len(engine.raws) != 0 {
				t.Errorf("chats=%d raws=%d, want no output", len(engine.chats), len(engine.raws))
			}
		})
	}
}
