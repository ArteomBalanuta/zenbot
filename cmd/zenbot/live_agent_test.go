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
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/tool/contract"
	"zenbot/internal/command"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/listener/message"
	"zenbot/internal/model"
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
	if got.NoReplyMarker != "marker" || got.MaxOutputChars != 17 {
		t.Fatalf("finalizer = %#v", got)
	}
}

type liveAgentTestEngine struct{ common.Engine }

type trustedSnapshotEngine struct {
	common.Engine
	users    map[*model.User]struct{}
	nilUsers bool
}

func (e trustedSnapshotEngine) GetChannel() string { return "programming" }
func (e trustedSnapshotEngine) GetActiveUsers() *map[*model.User]struct{} {
	if e.nilUsers {
		return nil
	}
	return &e.users
}
func (e trustedSnapshotEngine) IsUserAuthorized(user *model.User, role *model.Role) bool {
	if user == nil || role == nil {
		return false
	}
	return user.Trip == "admin-trip" || user.Trip == "mod-trip" && *role >= model.MODERATOR
}

func TestTrustedAgentSnapshotPropagatesPersistedModeratorAndAdminRoles(t *testing.T) {
	admin := &model.User{Name: "admin", Trip: "admin-trip"}
	moderator := &model.User{Name: "moderator", Trip: "mod-trip"}
	regular := &model.User{Name: "regular", Trip: "regular-trip"}
	engine := trustedSnapshotEngine{users: map[*model.User]struct{}{admin: {}, moderator: {}, regular: {}}}
	snapshot := trustedAgentSnapshot(engine, "creator", []string{"configured-admin"})
	if snapshot.Room != "programming" || len(snapshot.Users) != 3 || snapshot.Roles["admin-trip"] != participation.RoleAdmin || snapshot.Roles["mod-trip"] != participation.RoleModerator {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if _, ok := snapshot.Roles["regular-trip"]; ok {
		t.Fatal("regular caller received a privileged role")
	}
}

func TestTrustedAgentSnapshotToleratesUnavailableActiveUserSet(t *testing.T) {
	snapshot := trustedAgentSnapshot(trustedSnapshotEngine{nilUsers: true}, "creator", []string{"admin"})
	if snapshot.Room != "programming" || len(snapshot.Users) != 0 || snapshot.CreatorTrip != "creator" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
}

func TestAgentCommandEngineResolverBindsCredentialedSnapshotCapability(t *testing.T) {
	first := &core.EngineImpl{Type: model.MASTER, Channel: "first"}
	binding := newMasterBinding(first)
	resolve, err := resolveCurrentEngine(binding)
	if err != nil {
		t.Fatal(err)
	}
	commandEngine := resolveAgentCommandEngine(resolve)
	if _, ok := commandEngine().(common.CredentialedRoomSnapshotSubmitter); !ok {
		t.Fatalf("resolved command engine %T lacks credentialed snapshot capability", commandEngine())
	}

	second := &core.EngineImpl{Type: model.MASTER, Channel: "second"}
	binding.Rebind(second)
	resolved := commandEngine()
	if resolved.GetChannel() != "second" {
		t.Fatalf("resolver retained retired master: %#v", resolved)
	}
	if _, ok := resolved.(common.CredentialedRoomSnapshotSubmitter); !ok {
		t.Fatalf("rebound command engine %T lacks credentialed snapshot capability", resolved)
	}
}

func TestNewLiveAgentSharesRuntimeWithDirectSubmitter(t *testing.T) {
	repository := &liveAgentRepositoryStub{}
	enabled, err := newLiveAgent(&config.Config{Agent: config.AgentConfig{Enabled: true, Endpoint: "http://localhost:1", Model: "test", CreatorTrip: "creator"}}, liveAgentTestEngine{}, repository, roomDirectoryForMainTest{})
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
	agent, err := newLiveAgent(&config.Config{Agent: config.AgentConfig{Enabled: true, Endpoint: "http://localhost:1", Model: "test", CreatorTrip: "creator", Ambient: true}}, liveAgentTestEngine{}, repository, roomDirectoryForMainTest{})
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
		t.Fatalf("disabled semantic ingress must stay uncomposed: %#v", participation)
	}
	if _, err := newLiveAgent(&config.Config{}, liveAgentTestEngine{}, nil, roomDirectoryForMainTest{}); err != nil {
		t.Fatal(err)
	}
}

func TestNewLiveAgentWiresSemanticModerationOnlyWhenConfigured(t *testing.T) {
	repository := &liveAgentRepositoryStub{}
	agentConfig := config.AgentConfig{
		Enabled: true, Endpoint: "http://localhost:1", Model: "test", CreatorTrip: "creator", ModerationEnabled: true,
		ModerationJoinBurstCount: 1, ModerationJoinWindowSeconds: 1,
		ModerationSameHashCount: 1, ModerationSameHashWindowSeconds: 1,
		ModerationNameClusterCount: 1, ModerationNameClusterWindowSeconds: 1,
		ModerationPostKickWindowSeconds: 1, ModerationActionCooldownSeconds: 1,
		ModerationMessageBurstCount: 1, ModerationMessageBurstWindowSeconds: 1,
		ModerationRepeatedMessageCount: 1, ModerationRepeatedMessageWindowSeconds: 1,
		ModerationSecondBreachWindowSeconds: 1,
	}
	agent, err := newLiveAgent(&config.Config{Agent: agentConfig}, liveAgentTestEngine{}, repository, roomDirectoryForMainTest{})
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	room, ok := agent.Participation.(live.RoomParticipation)
	if !ok || room.SemanticCandidate == nil || room.Pipeline == nil || !room.Pipeline.SemanticModerationReady {
		t.Fatalf("semantic moderation composition is incomplete: %#v", agent.Participation)
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
func (liveAgentRepositoryStub) LoadAgentMemorySummary(context.Context, string, int64) (*repository.AgentMemorySummary, error) {
	return nil, nil
}
func (liveAgentRepositoryStub) UpsertAgentMemorySummary(context.Context, repository.AgentMemorySummary) error {
	return nil
}

type mainTestGateway struct{}

func (mainTestGateway) Execute(context.Context, api.Context, string, string) (commandgateway.Execution, error) {
	return commandgateway.Execution{}, nil
}

func TestNewAgentToolLoopRegistersEveryAgentCommandWithContextualVisibility(t *testing.T) {
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	assembler, err := assemble.New(assemble.Config{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	directory := roomDirectoryForMainTest{}
	loop, err := newAgentToolLoop(config.ResolvedAgentConfig{AgentConfig: config.AgentConfig{ContextMessageLimit: 1, MaxSteps: 5, MaxTools: 4}}, liveAgentRepositoryStub{}, assembler, mainTestClient{}, directory, mainTestGateway{})
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range commandcatalog.AgentEntries() {
		if _, ok := loop.Registry.Lookup("saturn_" + definition.Canonical); !ok {
			t.Fatalf("missing agent command tool for %q", definition.Canonical)
		}
	}
	if loop.Limits.MaxSteps != 5 || loop.Limits.MaxToolCalls != 4 {
		t.Fatalf("registry limits=%+v", loop.Limits)
	}
	public, _ := api.NewContext("room", "caller", "", "", false, []string{})
	if definitionNames(loop.Registry.Definitions(public))["saturn_prefix"] {
		t.Fatal("admin command exposed to public caller")
	}
	creator, _ := api.NewContextWithCapabilities("room", "creator", "trip", "", false, []string{}, []api.Capability{api.ModerationCommands, api.PermanentBan, api.AdminCommands})
	if !definitionNames(loop.Registry.Definitions(creator))["saturn_prefix"] || !definitionNames(loop.Registry.Definitions(creator))["saturn_ban"] {
		t.Fatal("creator command inventory is incomplete")
	}
}

func definitionNames(definitions []contract.Definition) map[string]bool {
	names := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		names[definition.Name] = true
	}
	return names
}

type roomDirectoryForMainTest struct{}

type mainTestClient struct{}

func (mainTestClient) Complete(context.Context, llm.LlmRequest) (llm.LlmResponse, error) {
	return llm.LlmResponse{}, nil
}

func (roomDirectoryForMainTest) FindRoomUsers(string) (tool.RoomUserSnapshot, bool) {
	return tool.RoomUserSnapshot{}, false
}
