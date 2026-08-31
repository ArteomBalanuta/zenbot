package main

import (
	"context"
	"testing"

	"zenbot/internal/agent/api"
	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/tool"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/repository"
)

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
