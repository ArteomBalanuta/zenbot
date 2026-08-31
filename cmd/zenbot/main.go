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
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/repository/h2"
	"zenbot/internal/transport"
)

type lifecycleEngine struct{ e *core.EngineImpl }

type liveAgent struct {
	Runtime       *runtime.Runtime
	Participation message.Participation
}

type agentRepositories interface {
	repository.AgentConversationRepository
	repository.AgentUserMessageHistoryRepository
	repository.AgentMemoryRepository
	repository.AgentToolEvidenceRepository
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
	if a != nil && a.Runtime != nil {
		a.Runtime.Close()
	}
}

func newLiveAgent(c *config.Config, engine common.Engine, conversationRepository agentRepositories, directory tool.RoomUserDirectory) (*liveAgent, error) {
	if c == nil || engine == nil {
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
	toolLoop, err := newAgentToolLoop(resolved, conversationRepository, assembler, client, directory, command.NewAgentCommandGateway(engine))
	if err != nil {
		return nil, fmt.Errorf("agent history tool: %w", err)
	}
	sink := runtime.SinkFunc(func(ctx context.Context, inv runtime.Invocation, result runtime.Result) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := engine.SendChatMessage(inv.Context().Nick(), "\n"+result.Text(), inv.Context().Whisper())
		return err
	})
	failure := runtime.FailureSinkFunc(func(ctx context.Context, inv runtime.Invocation, _ error) {
		if ctx.Err() != nil {
			return
		}
		if _, err := engine.SendChatMessage(inv.Context().Nick(), "failed: the agent could not answer that request.", inv.Context().Whisper()); err != nil {
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
	snapshot := func(_ *message.Context) participation.TrustedSnapshot {
		users := []string{}
		if safe, ok := engine.(interface{ ActiveUserNames() []string }); ok {
			users = safe.ActiveUserNames()
		} else {
			for u := range *engine.GetActiveUsers() {
				users = append(users, u.Name)
			}
		}
		return participation.TrustedSnapshot{Room: engine.GetChannel(), Users: append([]string(nil), users...), CreatorTrip: resolved.CreatorTrip, AdminTrips: append([]string(nil), c.AdminTrips...)}
	}
	p := live.RoomParticipation{Pipeline: &participation.Pipeline{Factory: participation.NewInvocationFactory(nil), Quiet: participation.NewQuietRegistry(time.Duration(resolved.QuietMinutes) * time.Minute), Parser: participation.MentionParser{}, Submit: runtime.APIBridge{Runtime: rt}}, Snapshot: snapshot, AmbientEnabled: resolved.Ambient, AmbientEvery: uint64(resolved.AmbientEveryMessages)}
	if authoritative, ok := engine.(*core.EngineImpl); ok {
		if automation := factory.ComposeMessageAutomation(authoritative, c); automation != nil {
			p.Pipeline.Monitor = func(event participation.Event) { automation.Observe(context.Background(), event) }
		}
	}
	return &liveAgent{Runtime: rt, Participation: p}, nil
}

func (x lifecycleEngine) Start(ctx context.Context) error { return x.e.StartContext(ctx) }
func (x lifecycleEngine) Stop(ctx context.Context) error  { return x.e.StopContext(ctx) }
func (x lifecycleEngine) Healthy() bool                   { return x.e.Healthy() }

func directAgentInvoker(c *config.Config, engine common.Engine, conversationRepository agentRepositories, directory tool.RoomUserDirectory) (command.DirectAgentInvoker, error) {
	if c == nil {
		return nil, fmt.Errorf("application config is nil")
	}
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	resolved, err := c.Agent.Resolve(config.ValueReader{Environment: values})
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
	assembler, err := assemble.New(assemble.Config{}, catalog)
	if err != nil {
		return nil, fmt.Errorf("agent assembler: %w", err)
	}
	toolLoop, err := newAgentToolLoop(resolved, conversationRepository, assembler, client, directory, command.NewAgentCommandGateway(engine))
	if err != nil {
		return nil, fmt.Errorf("agent history tool: %w", err)
	}
	finalizer, err := outputFinalizer(resolved)
	if err != nil {
		return nil, fmt.Errorf("agent verified quotes: %w", err)
	}
	return live.DirectInvoker{Assembler: assembler, Client: client, ConversationContext: conversationContext, ToolLoop: toolLoop, Finalizer: finalizer, Memory: memory}, nil
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
	e, err := factory.NewEngineWithOptions(model.MASTER, c, db, factory.EngineOptions{Transport: transport.Config{URL: c.WebsocketUrl}, LifecycleErrors: transportErrors})
	if err != nil {
		log.Fatal(err)
	}
	manager := core.NewReplicaManager(c.Channel)
	directory := core.EngineRoomUserDirectory{Host: e, Replicas: manager}
	agentInvoker, err := directAgentInvoker(c, e, db, directory)
	if err != nil {
		log.Fatal("Can't configure direct agent: ", err)
	}
	roomAgent, err := newLiveAgent(c, e, db, directory)
	if err != nil {
		log.Fatal("Can't configure room agent: ", err)
	}
	e.UserChatListener = listener.NewUserChatListenerWithChain(e, message.DefaultChainWithParticipation(roomAgent.Participation))
	rf := factory.ReplicaFactory{Config: c, Repository: db, Options: factory.EngineOptions{Transport: transport.Config{URL: c.WebsocketUrl}, LifecycleErrors: transportErrors}}
	e.SetReplicaController(core.NewManagedReplicaController(manager, func(ctx context.Context, channel string) (core.ManagedEngine, error) {
		return rf.NewReplica(ctx, channel)
	}))
	if err := command.RegisterUserUtilitiesWithDirectAgent(e, agentInvoker); err != nil {
		log.Fatal("Can't register Saturn utility commands: ", err)
	}

	lifecycle := core.NewLifecycle(func() core.LifecycleEngine { return lifecycleEngine{e} }, core.RetryPolicy{MaxRetries: 3, StopTimeout: 10 * time.Second})
	go func() {
		for err := range lifecycle.Errors() {
			log.Printf("lifecycle: %v", err)
		}
	}()
	go func() {
		for err := range transportErrors {
			log.Printf("transport: %v", err)
		}
	}()
	if err := lifecycle.Start(ctx); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
	stopCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	roomAgent.Close()
	if err := lifecycle.Stop(stopCtx); err != nil {
		log.Printf("host stop: %v", err)
	}
	if err := manager.StopAll(stopCtx); err != nil {
		log.Printf("replica stop: %v", err)
	}
}
