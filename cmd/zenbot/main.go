package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"zenbot/internal/agent/assemble"
	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/agent/live"
	"zenbot/internal/agent/llm"
	"zenbot/internal/agent/llm/openai"
	"zenbot/internal/agent/observability"
	"zenbot/internal/agent/participation"
	"zenbot/internal/agent/prompt"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/agent/tool"
	"zenbot/internal/agent/turn"
	"zenbot/internal/command"
	commandcatalog "zenbot/internal/command/catalog"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/factory"
	"zenbot/internal/listener"
	"zenbot/internal/listener/message"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
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
	repository.AgentMemorySummaryRepository
}

func newRoomSnapshotEngineOptions(c *config.Config, lifecycleErrors chan<- error, reply snapshot.ReplySink, profiler *profiling.Profiler) factory.EngineOptions {
	transportConfig := transport.Config{Profiler: profiler}
	if c != nil {
		transportConfig.URL = c.WebsocketUrl
	}
	registry := snapshot.NewTemporarySessionRegistry()
	sessions := factory.NewCoordinatedSessionFactory(transportConfig, registry, nil)
	coordinator := snapshot.NewRoomSnapshotCoordinator(sessions, reply, func(raw string) (snapshot.Snapshot, error) { return snapshot.Parse(raw, false) }, 30*time.Second)
	return factory.EngineOptions{Transport: transportConfig, LifecycleErrors: lifecycleErrors, SnapshotCoordinator: coordinator, SessionRegistry: registry, Profiler: profiler}
}

func environmentValues() map[string]string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		if key, value, ok := strings.Cut(item, "="); ok {
			values[key] = value
		}
	}
	return values
}

func roomSnapshotReplySink(send func(string, string, bool) (string, error)) snapshot.ReplySink {
	return func(request snapshot.RoomSnapshotRequest, reply string) {
		if _, err := send(request.Author, reply, request.Whisper); err != nil {
			log.Printf("room snapshot reply delivery: %v", err)
		}
	}
}

func newAgentMemory(resolved config.ResolvedAgentConfig, db agentRepositories, client llm.LlmClient) (*turn.TurnMemory, error) {
	compactor := &turn.MemoryCompactor{Summarizer: live.ModelConversationSummarizer{Client: client, MaxOutputChars: resolved.MemorySummaryMaxChars}}
	memory, err := turn.NewTurnMemory(live.PersistentMemoryStore{
		Repository:             db,
		ToolEvidenceRepository: db,
		Turns:                  resolved.MemoryTurns,
		TTL:                    resolved.MemoryTTL,
		Compactor:              compactor,
		RawTurnLimit:           resolved.MemoryRawTurns,
	})
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func outputFinalizer(resolved config.ResolvedAgentConfig) (live.OutputFinalizer, error) {
	return live.NewOutputFinalizer(resolved.NoReplyMarker, resolved.MaxOutputChars)
}

func newAgentToolLoop(resolved config.ResolvedAgentConfig, db repository.AgentUserMessageHistoryRepository, assembler *assemble.Assembler, catalog *prompt.Catalog, client llm.LlmClient, directory tool.RoomUserDirectory, gateway commandgateway.Gateway) (*live.ToolLoop, error) {
	if db == nil || assembler == nil || catalog == nil || client == nil || directory == nil || gateway == nil {
		return nil, fmt.Errorf("agent tool composition is incomplete")
	}
	if err := commandcatalog.ValidateAgentContracts(); err != nil {
		return nil, fmt.Errorf("agent command catalog: %w", err)
	}
	baseTools := []tool.Tool{
		tool.UserMessageHistory{Repository: db, Limit: tool.MaxUserMessageHistory},
		tool.RoomUsers{Directory: directory},
	}
	allowed := []string{"user_message_history", "room_users"}
	tools := baseTools
	for _, definition := range commandcatalog.AgentEntries() {
		commandTool := tool.SaturnCommand{Definition: definition, Gateway: gateway}
		tools = append(tools, commandTool)
		allowed = append(allowed, commandTool.Name())
	}
	if resolved.SQL.Enabled {
		queries, okQueries := db.(repository.AgentNamedQueryRepository)
		schema, okSchema := db.(repository.AgentSchemaRepository)
		sqlRepo, okSQL := db.(repository.AgentSQLRepository)
		if !okQueries || !okSchema || !okSQL {
			return nil, fmt.Errorf("agent SQL repository composition is incomplete")
		}
		tools = append(tools, tool.DatabaseQuery{Repository: queries}, tool.DatabaseSchema{Repository: schema, Enabled: true}, tool.DatabaseSQL{Schema: schema, Repository: sqlRepo, Config: resolved.SQL})
		allowed = append(allowed, "database_query", "database_schema", "database_sql")
	}
	return live.NewRegistryToolLoop(assembler, client, tools, allowed, turn.ExecutionLimits{
		MaxSteps:        resolved.MaxSteps,
		MaxToolCalls:    resolved.MaxTools,
		MaxCallsPerTool: resolved.MaxCallsPerTool,
		MaxToolFailures: resolved.MaxToolFailures,
		ToolTimeout:     time.Duration(resolved.ToolTimeoutMillis) * time.Millisecond,
	})
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

func resolveAgentCommandEngine(resolve func() common.Engine) func() common.Engine {
	return func() common.Engine {
		if resolve == nil {
			return nil
		}
		engine := resolve()
		if master, ok := engine.(*core.EngineImpl); ok {
			return core.BindCredentialedRoomSnapshotMaster(master)
		}
		return engine
	}
}

func newAgentFailureSink(resolveEngine func() common.Engine) runtime.FailureSink {
	return runtime.FailureSinkFunc(func(ctx context.Context, inv runtime.Invocation, cause error) {
		if ctx.Err() != nil {
			return
		}
		observability.Error(ctx, "agent.failure_reply.started", cause)
		current := resolveEngine()
		if current == nil {
			observability.Error(ctx, "agent.failure_reply.failed", fmt.Errorf("current master is not constructed"))
			return
		}
		if _, err := current.SendChatMessage(inv.Context().Nick(), live.FailureReply(cause), inv.Context().Whisper()); err != nil {
			observability.Error(ctx, "agent.failure_reply.failed", err)
			return
		}
		observability.Info(ctx, "agent.failure_reply.completed")
	})
}

func trustedAgentSnapshot(engine common.Engine, creatorTrip string, adminTrips []string) participation.TrustedSnapshot {
	snapshot := participation.TrustedSnapshot{CreatorTrip: creatorTrip, AdminTrips: append([]string(nil), adminTrips...), Roles: map[string]participation.Role{}}
	if engine == nil {
		return snapshot
	}
	snapshot.Room = engine.GetChannel()
	activeUsers := engine.GetActiveUsers()
	if activeUsers == nil {
		return snapshot
	}
	admin, moderator := model.ADMIN, model.MODERATOR
	for user := range *activeUsers {
		if user == nil {
			continue
		}
		snapshot.Users = append(snapshot.Users, user.Name)
		if strings.TrimSpace(user.Trip) == "" {
			continue
		}
		switch {
		case engine.IsUserAuthorized(user, &admin):
			snapshot.Roles[user.Trip] = participation.RoleAdmin
		case engine.IsUserAuthorized(user, &moderator):
			snapshot.Roles[user.Trip] = participation.RoleModerator
		}
	}
	sort.Strings(snapshot.Users)
	return snapshot
}

func newLiveAgent(c *config.Config, engine any, conversationRepository agentRepositories, directory tool.RoomUserDirectory) (*liveAgent, error) {
	resolveEngine, resolveErr := resolveCurrentEngine(engine)
	if c == nil || resolveErr != nil {
		return nil, fmt.Errorf("live agent configuration is incomplete")
	}
	resolved, err := c.Agent.Resolve(config.ValueReader{Environment: environmentValues()})
	if err != nil {
		return nil, fmt.Errorf("agent configuration: %w", err)
	}
	if !resolved.Enabled {
		return &liveAgent{Participation: message.PassParticipation{}}, nil
	}
	if directory == nil {
		return nil, fmt.Errorf("agent room directory is incomplete")
	}
	conversationContext, err := live.NewRepositoryConversationContextProvider(conversationRepository, resolved.ContextMessageLimit)
	if err != nil {
		return nil, fmt.Errorf("agent conversation context: %w", err)
	}
	client, err := openai.New(openai.Config{Endpoint: resolved.Endpoint, Token: resolved.APIKey, Model: resolved.Model, MaxTokens: resolved.MaxTokens, ThinkingEnabled: resolved.ThinkingEnabled, MaxRetries: resolved.MaxRetries, RetryDelay: time.Duration(resolved.RetryBackoffMillis) * time.Millisecond, Timeout: resolved.Timeout}, nil)
	if err != nil {
		return nil, fmt.Errorf("agent provider: %w", err)
	}
	memory, err := newAgentMemory(resolved, conversationRepository, client)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %w", err)
	}
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		return nil, fmt.Errorf("agent prompts: %w", err)
	}
	assembler, err := assemble.New(assemble.Config{CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker, MaxPromptChars: resolved.MaxPromptChars, MaxContextTokens: resolved.MaxContextTokens, ContextReserveTokens: resolved.ContextReserveTokens}, catalog)
	if err != nil {
		return nil, fmt.Errorf("agent assembler: %w", err)
	}
	toolLoop, err := newAgentToolLoop(resolved, conversationRepository, assembler, catalog, client, directory, command.NewResolvingAgentCommandGateway(resolveAgentCommandEngine(resolveEngine)))
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
	failure := newAgentFailureSink(resolveEngine)
	finalizer, err := outputFinalizer(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent output finalizer: %w", err)
	}
	rt, err := runtime.NewWithFailureSink(runtime.Config{MaxConcurrent: resolved.MaxConcurrentRequests, QueueCapacity: resolved.QueueCapacity, RequestTimeout: time.Duration(resolved.RequestTimeoutMillis) * time.Millisecond}, live.Runner{Assembler: assembler, Client: client, Finalizer: finalizer, ConversationContext: conversationContext, ToolLoop: toolLoop, Memory: memory}, sink, failure)
	if err != nil {
		return nil, err
	}
	service := live.RuntimeService{Runtime: rt}
	snapshot := func() participation.TrustedSnapshot {
		current := resolveEngine()
		return trustedAgentSnapshot(current, resolved.CreatorTrip, c.AdminTrips)
	}
	semanticModerationReady := resolved.ModerationEnabled && participation.SemanticModerationIngressReady()
	p := live.RoomParticipation{
		Pipeline: &participation.Pipeline{
			Factory:                 participation.NewInvocationFactory(nil),
			Quiet:                   participation.NewQuietRegistry(time.Duration(resolved.QuietMinutes) * time.Minute),
			Parser:                  participation.MentionParser{},
			Submit:                  service,
			SemanticModerationReady: semanticModerationReady,
		},
		Snapshot:       func(_ *message.Context) participation.TrustedSnapshot { return snapshot() },
		AmbientEnabled: resolved.Ambient,
		AmbientEvery:   uint64(resolved.AmbientEveryMessages),
	}
	if semanticModerationReady {
		p.SemanticCandidate = participation.SemanticModerationCandidate
	}
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

func directAgentInvoker(c *config.Config, engine common.Engine, conversationRepository agentRepositories, directory tool.RoomUserDirectory) (command.DirectAgentInvoker, error) {
	if c == nil {
		return nil, fmt.Errorf("application config is nil")
	}
	resolved, err := c.Agent.Resolve(config.ValueReader{Environment: environmentValues()})
	if err != nil {
		return nil, fmt.Errorf("agent configuration: %w", err)
	}
	if !resolved.Enabled {
		return nil, nil
	}
	if engine == nil {
		return nil, fmt.Errorf("agent command gateway engine is incomplete")
	}
	if directory == nil {
		return nil, fmt.Errorf("agent room directory is incomplete")
	}
	conversationContext, err := live.NewRepositoryConversationContextProvider(conversationRepository, resolved.ContextMessageLimit)
	if err != nil {
		return nil, fmt.Errorf("agent conversation context: %w", err)
	}
	client, err := openai.New(openai.Config{Endpoint: resolved.Endpoint, Token: resolved.APIKey, Model: resolved.Model, MaxTokens: resolved.MaxTokens, ThinkingEnabled: resolved.ThinkingEnabled, MaxRetries: resolved.MaxRetries, RetryDelay: time.Duration(resolved.RetryBackoffMillis) * time.Millisecond, Timeout: resolved.Timeout}, nil)
	if err != nil {
		return nil, fmt.Errorf("agent provider: %w", err)
	}
	memory, err := newAgentMemory(resolved, conversationRepository, client)
	if err != nil {
		return nil, fmt.Errorf("agent memory: %w", err)
	}
	catalog, err := prompt.NewCatalog(nil)
	if err != nil {
		return nil, fmt.Errorf("agent prompts: %w", err)
	}
	assembler, err := assemble.New(assemble.Config{CreatorTrip: resolved.CreatorTrip, NoReplyMarker: resolved.NoReplyMarker, MaxPromptChars: resolved.MaxPromptChars, MaxContextTokens: resolved.MaxContextTokens, ContextReserveTokens: resolved.ContextReserveTokens}, catalog)
	if err != nil {
		return nil, fmt.Errorf("agent assembler: %w", err)
	}
	toolLoop, err := newAgentToolLoop(resolved, conversationRepository, assembler, catalog, client, directory, command.NewAgentCommandGateway(engine))
	if err != nil {
		return nil, fmt.Errorf("agent history tool: %w", err)
	}
	finalizer, err := outputFinalizer(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent output finalizer: %w", err)
	}
	return live.DirectInvoker{Assembler: assembler, Client: client, ConversationContext: conversationContext, ToolLoop: toolLoop, Finalizer: finalizer, Memory: memory}, nil
}

func newAutoMoveProductionOptions(c *config.Config, master factory.EngineOptions) (*core.AutoMoveState, factory.EngineOptions, factory.EngineOptions) {
	state := core.NewAutoMoveState()
	master.AutoMoveState = state
	replicaTransport := master.Transport
	if replicaTransport.Profiler == nil {
		replicaTransport.Profiler = master.Profiler
	}
	return state, master, factory.EngineOptions{Transport: replicaTransport, LifecycleErrors: master.LifecycleErrors, AutoMoveState: state, Profiler: master.Profiler}
}

func lifecyclePolicy(c *config.Config) core.RetryPolicy {
	p := core.RetryPolicy{Interval: time.Second, StopTimeout: 10 * time.Second}
	if c == nil || !c.AutoReconnect {
		p.DisableHealthChecks = true
		p.MaxRetries = 1
		return p
	}
	if c.ConnectionHeartbitIntervalMinutes > 0 {
		p.HealthInterval = time.Duration(c.ConnectionHeartbitIntervalMinutes) * time.Minute
	}
	// Saturn's scheduler keeps recovering a disconnected host until shutdown.
	p.MaxRetries = 0
	return p
}

// newFixedEngineLifecycle preserves the original fixed-master lifecycle for
// callers that do not use the replaceable production host graph.
func newFixedEngineLifecycle(e *core.EngineImpl, c *config.Config) *core.Lifecycle {
	lifecycle := core.NewLifecycle(func() core.LifecycleEngine { return lifecycleEngine{e} }, lifecyclePolicy(c))
	go func() {
		for err := range lifecycle.Errors() {
			log.Printf("lifecycle: %v", err)
		}
	}()
	return lifecycle
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	c := config.SetupConfig()
	resolvedProfiling, err := c.Profiling.Resolve(config.ValueReader{Environment: environmentValues()})
	if err != nil {
		log.Fatal("Invalid profiling configuration: ", err)
	}
	performanceProfiler := profiling.New(profiling.Settings{
		Enabled:                resolvedProfiling.Enabled,
		SlowCommandThreshold:   resolvedProfiling.SlowCommandThreshold,
		SlowStageThreshold:     resolvedProfiling.SlowStageThreshold,
		SlowTransportThreshold: resolvedProfiling.SlowTransportThreshold,
	}, slog.Default())
	if resolvedProfiling.Enabled {
		slog.Info("profiling.regular_commands.enabled",
			"slow_command_ms", resolvedProfiling.SlowCommandThresholdMillis,
			"slow_stage_ms", resolvedProfiling.SlowStageThresholdMillis,
			"slow_transport_ms", resolvedProfiling.SlowTransportThresholdMillis,
			"excluded_command", "l",
		)
		profileServer, profileErr := profiling.StartServer(ctx, profiling.ServerConfig{
			Address:              resolvedProfiling.ListenAddress,
			BlockProfileRate:     resolvedProfiling.BlockProfileRate,
			MutexProfileFraction: resolvedProfiling.MutexProfileFraction,
		}, slog.Default())
		if profileErr != nil {
			log.Fatal("Can't start profiling endpoint: ", profileErr)
		}
		defer func() {
			shutdownCtx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdown()
			if closeErr := profileServer.Close(shutdownCtx); closeErr != nil {
				log.Printf("profiling shutdown: %v", closeErr)
			}
		}()
	}
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
	snapshotOptions := newRoomSnapshotEngineOptions(c, transportErrors, roomSnapshotReplySink(masterReplySender(binding)), performanceProfiler)
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
		e.OnlineSetListener = listener.NewOnlineSetListener(e, newAutorunCallback(c))
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
	if err := supervisor.StartInitial(ctx); err != nil {
		log.Fatal(err)
	}
	go runHostRecovery(ctx, transportErrors, lifecyclePolicy(c), func() bool {
		master, ok := supervisor.Master().(*core.EngineImpl)
		return !ok || master == nil || master.Healthy()
	}, hostLifecycle.RequestRestart, func(err error) {
		log.Printf("transport: %v", err)
	})
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
