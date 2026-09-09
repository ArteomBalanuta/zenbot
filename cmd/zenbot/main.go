package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/llm/openai"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/turn"
	"zenbot/internal/command"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/listener"
	"zenbot/internal/listener/message"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/repository/h2"
	"zenbot/internal/transport"
)

type lifecycleEngine struct{ e *core.EngineImpl }

type liveAgent struct {
	Service         live.AgentService
	Participation   message.Participation
	DirectSubmitter command.DirectAgentSubmitter
}

type agentRepositories interface {
	repository.AgentConversationRepository
	repository.AgentUserMessageHistoryRepository
	repository.AgentMemoryRepository
	repository.AgentToolEvidenceRepository
}

func newRoomSnapshotEngineOptions(c *config.Config, lifecycleErrors chan<- error, reply snapshot.ReplySink) factory.EngineOptions {
	transportConfig := transport.Config{}
	if c != nil {
		transportConfig.URL = c.WebsocketUrl
	}
	registry := snapshot.NewTemporarySessionRegistry()
	sessions := factory.NewCoordinatedSessionFactory(transportConfig, registry, nil)
	coordinator := snapshot.NewRoomSnapshotCoordinator(sessions, reply, func(raw string) (snapshot.Snapshot, error) { return snapshot.Parse(raw, false) }, 30*time.Second)
	return factory.EngineOptions{Transport: transportConfig, LifecycleErrors: lifecycleErrors, SnapshotCoordinator: coordinator, SessionRegistry: registry}
}

func roomSnapshotReplySink(send func(string, string, bool) (string, error)) snapshot.ReplySink {
	return func(request snapshot.RoomSnapshotRequest, reply string) {
		if _, err := send(request.Author, reply, request.Whisper); err != nil {
			log.Printf("room snapshot reply delivery: %v", err)
		}
	}
}

func newAgentMemory(resolved config.ResolvedAgentConfig, db agentRepositories) (*turn.TurnMemory, error) {
	memory, err := turn.NewTurnMemory(live.PersistentMemoryStore{Repository: db, ToolEvidenceRepository: db, Turns: resolved.MemoryTurns, TTL: resolved.MemoryTTL})
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func outputFinalizer(resolved config.ResolvedAgentConfig) (live.OutputFinalizer, error) {
	return live.NewOutputFinalizer(resolved.NoReplyMarker, resolved.MaxOutputChars)
}

func newAgentToolLoop(resolved config.ResolvedAgentConfig, db repository.AgentUserMessageHistoryRepository, assembler *assemble.Assembler, client llm.LlmClient, directory tool.RoomUserDirectory, gateway commandgateway.Gateway) (*live.ToolLoop, error) {
	if db == nil || assembler == nil || client == nil || directory == nil || gateway == nil {
		return nil, fmt.Errorf("agent tool composition is incomplete")
	}
	limit := resolved.ContextMessageLimit
	if limit > 60 {
		limit = 60
	}
	if limit < 1 {
		limit = 1
	}
	return live.NewBoundedToolLoop(assembler, client, []tool.Tool{
		tool.UserMessageHistory{Repository: db, Limit: limit},
		tool.RoomUsers{Directory: directory},
		tool.RunCommand{Gateway: gateway},
	}, []string{"user_message_history", "room_users", "run_command"})
}

func (a *liveAgent) Close() {
	if a != nil && a.Service != nil {
		a.Service.Close()
	}
}

type masterEngineProvider interface{ Current() *core.EngineImpl }

func resolveCurrentEngine(source any) (func() common.Engine, error) {
	switch value := source.(type) {
	case masterEngineProvider:
		return func() common.Engine { return value.Current() }, nil
	case common.Engine:
		return func() common.Engine { return value }, nil
	default:
		return nil, fmt.Errorf("live agent master resolver is incomplete")
	}
}

func newLiveAgent(c *config.Config, engine any, conversationRepository agentRepositories, directory tool.RoomUserDirectory) (*liveAgent, error) {
	resolveEngine, resolveErr := resolveCurrentEngine(engine)
	if c == nil || resolveErr != nil {
		return nil, fmt.Errorf("live agent configuration is incomplete")
	}
	values := map[string]string{}
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	resolved, err := c.Agent.Resolve(config.ValueReader{Environment: values})
	if err != nil {
		return nil, fmt.Errorf("agent configuration: %w", err)
	}
	if !resolved.Enabled {
		return &liveAgent{Participation: message.PassParticipation{}}, nil
	}
	if directory == nil {
		return nil, fmt.Errorf("agent room directory is incomplete")
	}
	memory, err := newAgentMemory(resolved, conversationRepository)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %w", err)
	}
	conversationContext, err := live.NewRepositoryConversationContextProvider(conversationRepository, resolved.ContextMessageLimit)
	if err != nil {
		return nil, fmt.Errorf("agent conversation context: %w", err)
	}
	client, err := openai.New(openai.Config{Endpoint: resolved.Endpoint, Token: resolved.APIKey, Model: resolved.Model, MaxTokens: resolved.MaxTokens, Timeout: resolved.Timeout}, nil)
	if err != nil {
		return nil, fmt.Errorf("agent provider: %w", err)
	}
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		return nil, fmt.Errorf("agent prompts: %w", err)
	}
	assembler, err := assemble.New(assemble.Config{CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker}, catalog)
	if err != nil {
		return nil, fmt.Errorf("agent assembler: %w", err)
	}
	toolLoop, err := newAgentToolLoop(resolved, conversationRepository, assembler, client, directory, command.NewResolvingAgentCommandGateway(resolveEngine))
	if err != nil {
		return nil, fmt.Errorf("agent history tool: %w", err)
	}
	sink := runtime.SinkFunc(func(ctx context.Context, inv runtime.Invocation, result runtime.Result) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := resolveEngine()
		if current == nil {
			return fmt.Errorf("current master is not constructed")
		}
		_, err := current.SendChatMessage(inv.Context().Nick(), "\n"+result.Text(), inv.Context().Whisper())
		return err
	})
	failure := runtime.FailureSinkFunc(func(ctx context.Context, inv runtime.Invocation, _ error) {
		if ctx.Err() != nil {
			return
		}
		current := resolveEngine()
		if current == nil {
			return
		}
		if _, err := current.SendChatMessage(inv.Context().Nick(), "failed: the agent could not answer that request.", inv.Context().Whisper()); err != nil {
			log.Printf("agent failure delivery: %v", err)
		}
	})
	finalizer, err := outputFinalizer(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent verified quotes: %w", err)
	}
	rt, err := runtime.NewWithFailureSink(runtime.Config{MaxConcurrent: resolved.MaxConcurrentRequests, QueueCapacity: resolved.QueueCapacity}, live.Runner{Assembler: assembler, Client: client, Finalizer: finalizer, ConversationContext: conversationContext, ToolLoop: toolLoop, Memory: memory}, sink, failure)
	if err != nil {
		return nil, err
	}
	service := live.RuntimeService{Runtime: rt}
	snapshot := func() participation.TrustedSnapshot {
		current := resolveEngine()
		if current == nil {
			return participation.TrustedSnapshot{CreatorTrip: resolved.CreatorTrip, AdminTrips: append([]string(nil), c.AdminTrips...)}
		}
		users := []string{}
		if safe, ok := current.(interface{ ActiveUserNames() []string }); ok {
			users = safe.ActiveUserNames()
		} else {
			for u := range *current.GetActiveUsers() {
				users = append(users, u.Name)
			}
		}
		return participation.TrustedSnapshot{Room: current.GetChannel(), Users: append([]string(nil), users...), CreatorTrip: resolved.CreatorTrip, AdminTrips: append([]string(nil), c.AdminTrips...)}
	}
	p := live.RoomParticipation{Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Quiet: participation.NewQuietRegistry(time.Duration(resolved.QuietMinutes) * time.Minute), Parser: participation.MentionParser{}, Submit: service, SemanticModerationReady: participation.SemanticModerationIngressReady()}, Snapshot: func(_ *message.Context) participation.TrustedSnapshot { return snapshot() }, AmbientEnabled: resolved.Ambient, AmbientEvery: uint64(resolved.AmbientEveryMessages)}
	if authoritative, ok := resolveEngine().(*core.EngineImpl); ok {
		if automation := factory.ComposeMessageAutomation(authoritative, c); automation != nil {
			p.Pipeline.Monitor = func(event participation.Event) { automation.Observe(context.Background(), event) }
		}
	}
	return &liveAgent{
		Service:         service,
		Participation:   p,
		DirectSubmitter: live.DirectSubmissionAdapter{Service: service, Factory: participation.NewInvocationFactory(nil), Snapshot: snapshot},
	}, nil
}

func (x lifecycleEngine) Start(ctx context.Context) error { return x.e.StartContext(ctx) }
func (x lifecycleEngine) Stop(ctx context.Context) error  { return x.e.StopContext(ctx) }
func (x lifecycleEngine) Healthy() bool                   { return x.e.Healthy() }

func newAutoMoveProductionOptions(c *config.Config, master factory.EngineOptions) (*core.AutoMoveState, factory.EngineOptions, factory.EngineOptions) {
	state := core.NewAutoMoveState()
	master.AutoMoveState = state
	return state, master, factory.EngineOptions{Transport: master.Transport, LifecycleErrors: master.LifecycleErrors, AutoMoveState: state}
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	c := config.SetupConfig()
	db, err := h2.Open(ctx, h2.Config{DatabaseStem: c.DbPath, H2Jar: os.Getenv("H2_JAR"), Java: os.Getenv("JAVA"), Port: 5435})
	if err != nil {
		log.Fatal("Can't connect to db: ", err)
	}
	defer func() {
		if db.DB != nil {
			_ = db.DB.Close()
		}
		if db.Server != nil {
			stop, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			_ = db.Server.Stop(stop)
		}
	}()

	transportErrors := make(chan error, 16)
	binding := newMasterBinding(nil)
	snapshotOptions := newRoomSnapshotEngineOptions(c, transportErrors, roomSnapshotReplySink(masterReplySender(binding)))
	_, snapshotOptions, replicaOptions := newAutoMoveProductionOptions(c, snapshotOptions)
	manager := core.NewReplicaManager(c.Channel)
	directory := core.NewEngineRoomUserDirectory(nil, manager)
	rf := factory.ReplicaFactory{Config: c, Repository: db, Options: replicaOptions}
	// The room-agent runtime is process-owned. Its master-facing callbacks resolve
	// binding on each use, while each master receives a fresh listener.
	roomAgent, err := newLiveAgent(c, binding, db, directory)
	if err != nil {
		log.Fatal("Can't configure room agent: ", err)
	}

	var hostLifecycle *core.HostLifecycle
	buildMasterGraph := func(context.Context) (any, error) {
		e, err := factory.NewEngineWithOptions(model.MASTER, c, db, snapshotOptions)
		if err != nil {
			return nil, err
		}
		e.SetReplicaController(core.NewManagedReplicaController(manager, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
			return rf.NewReplica(ctx, channel)
		}))
		e.SetSupportReplicaRelay(core.NewSupportReplicaRelay(manager, c.AdminTrips))
		e.SetHostLifecycleController(hostLifecycle)
		e.UserChatListener = listener.NewUserChatListenerWithChain(e, message.DefaultChainWithParticipation(roomAgent.Participation))
		if err := command.RegisterUserUtilitiesWithDirectAgent(core.BindCredentialedRoomSnapshotMaster(e), roomAgent.DirectSubmitter); err != nil {
			return nil, err
		}
		return e, nil
	}
	supervisor := NewHostSupervisor(nil, func(stopCtx context.Context, master any) error {
		host, ok := master.(*core.EngineImpl)
		if !ok {
			return fmt.Errorf("master host has unexpected type %T", master)
		}
		return host.StopContext(stopCtx)
	}, buildMasterGraph, func(startCtx context.Context, candidate any) error {
		host, ok := candidate.(*core.EngineImpl)
		if !ok {
			return fmt.Errorf("fresh master has unexpected type %T", candidate)
		}
		return host.StartContext(startCtx)
	}, func(candidate any) {
		host, ok := candidate.(*core.EngineImpl)
		if !ok {
			return
		}
		binding.Rebind(host)
		directory.RebindHost(host)
	})
	hostLifecycle = newProductionHostLifecycle(supervisor)
	defer hostLifecycle.Close()
	go func() {
		for err := range transportErrors {
			log.Printf("transport: %v", err)
		}
	}()
	if err := supervisor.StartInitial(ctx); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
	stopCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	supervisor.SignalTeardown(func(teardownCtx context.Context) error {
		roomAgent.Close()
		var first error
		if current := supervisor.Master(); current != nil {
			if err := supervisor.Shutdown(teardownCtx); err != nil {
				first = err
			}
		}
		if err := manager.StopAll(teardownCtx); err != nil && first == nil {
			first = err
		}
		return first
	}, stopCtx)
}
