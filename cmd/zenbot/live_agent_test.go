package main

import (
	"context"
	"testing"
	"time"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/tool"
	"zenbot/internal/command"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/listener/message"
	"zenbot/internal/repository"
)

func TestLifecyclePolicyUsesSaturnReconnectAndHealthSettings(t *testing.T) {
	enabled := lifecyclePolicy(&config.Config{AutoReconnect: true, ConnectionHeartbitIntervalMinutes: 5})
	if enabled.HealthInterval != 5*time.Minute || enabled.MaxRetries != 0 {
		t.Fatalf("enabled policy=%+v", enabled)
	}
	disabled := lifecyclePolicy(&config.Config{AutoReconnect: false, ConnectionHeartbitIntervalMinutes: 5})
	if !disabled.DisableHealthChecks || disabled.MaxRetries != 1 {
		t.Fatalf("disabled policy=%+v", disabled)
	}
}

func TestDirectAgentInvokerDisabledDoesNotBlockStartup(t *testing.T) {
	invoker, err := directAgentInvoker(&config.Config{}, nil, nil, roomDirectoryForMainTest{})
	if err != nil {
		t.Fatalf("disabled configuration must not block startup: %v", err)
	}
	if invoker != nil {
		t.Fatal("disabled configuration unexpectedly constructed a direct invoker")
	}
}

func TestOutputFinalizerUsesResolvedMarkerAndBound(t *testing.T) {
	resolved := config.ResolvedAgentConfig{AgentConfig: config.AgentConfig{NoReplyMarker: "marker", MaxOutputChars: 17}}
	got, err := outputFinalizer(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if got.NoReplyMarker != "marker" || got.MaxOutputChars != 17 || got.Catalog == nil {
		t.Fatalf("finalizer = %#v", got)
	}
}

type liveAgentTestEngine struct{ common.Engine }

func TestNewLiveAgentSharesRuntimeWithDirectSubmitter(t *testing.T) {
	repository := &liveAgentRepositoryStub{}
	enabled, err := newLiveAgent(&config.Config{Agent: config.AgentConfig{Enabled: true, Endpoint: "http://localhost:1", Model: "test"}}, liveAgentTestEngine{}, repository, roomDirectoryForMainTest{})
	if err != nil {
		t.Fatal(err)
	}
	defer enabled.Close()
	if enabled.Service == nil || enabled.DirectSubmitter == nil {
		t.Fatalf("enabled composition is incomplete: %#v", enabled)
	}
	service, ok := enabled.Service.(live.RuntimeService)
	if !ok || service.Runtime == nil {
		t.Fatalf("service must own the composed runtime: %#v", enabled.Service)
	}
	direct, ok := enabled.DirectSubmitter.(live.DirectSubmissionAdapter)
	if !ok || direct.Service != enabled.Service {
		t.Fatalf("direct submitter must use the composed service: %#v", enabled.DirectSubmitter)
	}
	enabledRegistration := &liveAgentRegistrationEngine{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(enabledRegistration, enabled.DirectSubmitter); err != nil {
		t.Fatal(err)
	}
	if !enabledRegistration.hasAlias("l") {
		t.Fatal("enabled direct submitter did not register l")
	}

	disabled, err := newLiveAgent(&config.Config{}, liveAgentTestEngine{}, nil, roomDirectoryForMainTest{})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Service != nil || disabled.DirectSubmitter != nil {
		t.Fatalf("disabled composition constructed live components: %#v", disabled)
	}
	if _, ok := disabled.Participation.(message.PassParticipation); !ok {
		t.Fatalf("disabled participation = %T, want pass-through", disabled.Participation)
	}
	disabledRegistration := &liveAgentRegistrationEngine{}
	if err := command.RegisterUserUtilitiesWithDirectAgent(disabledRegistration, disabled.DirectSubmitter); err != nil {
		t.Fatal(err)
	}
	if disabledRegistration.hasAlias("l") {
		t.Fatal("disabled composition registered l")
	}
}

type liveAgentRegistrationEngine struct {
	common.Engine
	commands []common.Command
}

func (e *liveAgentRegistrationEngine) RegisterCommand(command common.Command) {
	e.commands = append(e.commands, command)
}

func (e *liveAgentRegistrationEngine) hasAlias(alias string) bool {
	for _, command := range e.commands {
		for _, candidate := range command.GetAliases() {
			if candidate == alias {
				return true
			}
		}
	}
	return false
}

func TestNewLiveAgentWiresAmbientParticipationFromResolvedConfig(t *testing.T) {
	repository := &liveAgentRepositoryStub{}
	agent, err := newLiveAgent(&config.Config{Agent: config.AgentConfig{Enabled: true, Endpoint: "http://localhost:1", Model: "test", Ambient: true}}, liveAgentTestEngine{}, repository, roomDirectoryForMainTest{})
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	participation, ok := agent.Participation.(live.RoomParticipation)
	if !ok {
		t.Fatalf("participation = %T", agent.Participation)
	}
	if !participation.AmbientEnabled || participation.AmbientEvery == 0 || participation.Pipeline == nil || participation.Pipeline.Quiet == nil {
		t.Fatalf("ambient participation was not fully wired: %#v", participation)
	}
	if participation.SemanticCandidate != nil || participation.Pipeline.SemanticModerationReady {
		t.Fatalf("semantic ingress must stay uncomposed and fail-closed: %#v", participation)
	}
	if _, err := newLiveAgent(&config.Config{}, liveAgentTestEngine{}, nil, roomDirectoryForMainTest{}); err != nil {
		t.Fatal(err)
	}
}

type liveAgentRepositoryStub struct{}

func (liveAgentRepositoryStub) RecentPublicRoomMessages(context.Context, string, int) ([]repository.PublicRoomMessage, error) {
	return nil, nil
}
func (liveAgentRepositoryStub) RecentPublicRoomMessagesForNick(context.Context, string, string, int) ([]repository.PublicRoomMessage, error) {
	return nil, nil
}
func (liveAgentRepositoryStub) LoadAgentMemory(context.Context, string, int64, int) ([]repository.AgentMemoryMessage, error) {
	return []repository.AgentMemoryMessage{}, nil
}
func (liveAgentRepositoryStub) AppendAgentMemory(context.Context, string, string, string, int64, int64) error {
	return nil
}
func (liveAgentRepositoryStub) LoadAgentToolEvidence(context.Context, string, int64, int) ([]repository.AgentToolEvidence, error) {
	return []repository.AgentToolEvidence{}, nil
}
func (liveAgentRepositoryStub) AppendAgentToolEvidence(context.Context, string, string, string, int64, int64) error {
	return nil
}

type mainTestGateway struct{}

func (mainTestGateway) Execute(context.Context, api.Context, string, string) (commandgateway.Execution, error) {
	return commandgateway.Execution{}, nil
}

func TestNewAgentToolLoopFreezesHistoryAndRoomUsersAndRunCommand(t *testing.T) {
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	assembler, err := assemble.New(assemble.Config{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	directory := roomDirectoryForMainTest{}
	loop, err := newAgentToolLoop(config.ResolvedAgentConfig{AgentConfig: config.AgentConfig{ContextMessageLimit: 1}}, liveAgentRepositoryStub{}, assembler, mainTestClient{}, directory, mainTestGateway{})
	if err != nil {
		t.Fatal(err)
	}
	if len(loop.Tools) != 3 {
		t.Fatalf("tools=%#v", loop.Tools)
	}
}

type roomDirectoryForMainTest struct{}

type mainTestClient struct{}

func (mainTestClient) Complete(context.Context, llm.LlmRequest) (llm.LlmResponse, error) {
	return llm.LlmResponse{}, nil
}

func (roomDirectoryForMainTest) FindRoomUsers(string) (tool.RoomUserSnapshot, bool) {
	return tool.RoomUserSnapshot{}, false
}
