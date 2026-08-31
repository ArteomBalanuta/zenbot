package factory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"zenbot/internal/agent/live"
	"zenbot/internal/agent/moderation"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/listener"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/relay"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/transport"
)

type EngineOptions struct {
	Transport           transport.Config
	ListenerProfile     core.ListenerProfile
	ReplicaManager      *core.ReplicaManager
	LifecycleErrors     chan<- error
	SnapshotCoordinator *snapshot.RoomSnapshotCoordinator
	SessionRegistry     *snapshot.TemporarySessionRegistry
	// HostRelay is required only for an AGENT child and is installed once at creation.
	HostRelay relay.HostRelay
}

// NewEngine preserves the original broad compatibility contract.
func NewEngine(etype model.EngineType, c *config.Config, repo repository.Repository) common.Engine {
	e, err := NewEngineWithOptions(etype, c, repo, EngineOptions{Transport: transport.Config{URL: c.WebsocketUrl}, ListenerProfile: core.Permanent})
	if err != nil {
		return &core.EngineImpl{Type: etype, Channel: c.Channel, Name: c.Name, Prefix: c.CmdPrefix, Password: c.Password, EngineWg: new(sync.WaitGroup), OutMessageQueue: make(chan string, 256), ActiveUsers: map[*model.User]struct{}{}, AfkUsers: map[*model.User]string{}, EnabledCommands: map[string]common.CommandMetadata{}}
	}
	return e
}

func NewEngineWithOptions(etype model.EngineType, c *config.Config, repo repository.Repository, opts EngineOptions) (*core.EngineImpl, error) {
	if c == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if etype == model.AGENT && opts.HostRelay == nil {
		return nil, fmt.Errorf("AGENT engine requires a host relay")
	}
	if opts.ListenerProfile != core.TemporaryOnlineSet {
		opts.ListenerProfile = core.Permanent
	}
	if opts.Transport.URL == "" {
		opts.Transport.URL = c.WebsocketUrl
	}
	e := core.NewEngineImpl(core.EngineImpl{Type: etype, Prefix: c.CmdPrefix, Channel: c.Channel, Name: c.Name, Password: c.Password,
		EngineWg: new(sync.WaitGroup), EnabledCommands: make(map[string]common.CommandMetadata), OutMessageQueue: make(chan string, 256),
		ActiveUsers: make(map[*model.User]struct{}), AfkUsers: make(map[*model.User]string), Transport: transport.NewConnection(opts.Transport), Profile: opts.ListenerProfile, LifecycleErrors: opts.LifecycleErrors}, opts.HostRelay)
	e.CoreListener = listener.NewCoreListener(e)
	e.Repository = repo
	if auth, ok := repo.(repository.AuthorizationRepository); ok {
		e.SecurityService = service.NewSecurityService(c, auth)
	} else {
		e.SecurityService = service.NewSecurityService(c)
	}
	if dbp, ok := repo.(interface{ SQLDB() *sql.DB }); ok {
		db := dbp.SQLDB()
		var q repository.UserQueryRepository
		if x, ok := repo.(repository.UserQueryRepository); ok {
			q = x
		}
		identity, _ := repo.(repository.IdentityRepository)
		groupB, _ := repo.(repository.SqlUtilGroupBRepository)
		e.Services = &service.Bundle{Security: e.SecurityService, Mail: &service.MailService{DB: db, GroupB: groupB}, Notes: &service.NoteService{DB: db}, Users: &service.UserService{Queries: q, Identity: identity, GroupB: groupB}, Ping: &service.PingService{}, Weather: &service.WeatherService{}, Time: &service.TimeService{}, Search: &service.SearchService{}, SCP: &service.SCPService{}}
		if dbz, ok := repo.(repository.DBZRepository); ok {
			e.Services.DBZ = &service.DBZService{Repo: dbz}
		}
	}
	if opts.ListenerProfile == core.TemporaryOnlineSet {
		e.OnlineSetListener = listener.NewSnapshotOnlineSetListener(nil)
	} else {
		e.OnlineSetListener = listener.NewOnlineSetListener(e, nil)
	}
	if opts.ListenerProfile == core.TemporaryOnlineSet {
		e.UserChatListener, e.UserInfoListener, e.UserJoinedListener, e.UserLeftListener = common.NewDummyListener(), common.NewDummyListener(), common.NewDummyListener(), common.NewDummyListener()
	} else {
		e.UserChatListener, e.UserInfoListener = listener.NewUserChatListener(e), listener.NewInfoChatListener(e)
		e.UserJoinedListener, e.UserLeftListener = listener.NewUserJoinedListenerWithAutomation(e, composeJoinAutomation(e, c)), listener.NewUserLeftListener(e)
	}
	if etype == model.ZOMBIE {
		e.Repository = &repository.DummyImpl{}
		e.UserChatListener, e.UserJoinedListener, e.UserLeftListener = common.NewDummyListener(), common.NewDummyListener(), common.NewDummyListener()
	}
	return e, nil
}

func composeJoinAutomation(e *core.EngineImpl, c *config.Config) listener.JoinAutomation {
	if e == nil || c == nil || !c.Agent.ModerationEnabled {
		return nil
	}
	if _, ok := e.Repository.(repository.ShadowBanRepository); !ok {
		return nil
	}
	a := c.Agent
	if a.ModerationJoinBurstCount <= 0 || a.ModerationJoinWindowSeconds <= 0 || a.ModerationSameHashCount <= 0 || a.ModerationSameHashWindowSeconds <= 0 || a.ModerationNameClusterCount <= 0 || a.ModerationNameClusterWindowSeconds <= 0 || a.ModerationPostKickWindowSeconds <= 0 || a.ModerationActionCooldownSeconds <= 0 {
		return nil
	}
	protectedTrips := map[string]struct{}{}
	for _, trip := range append(append([]string{}, c.AdminTrips...), a.CreatorTrip) {
		if trip = strings.TrimSpace(trip); trip != "" {
			protectedTrips[trip] = struct{}{}
		}
	}
	protected := func(u *model.User) bool {
		if u == nil || u.Isme || u.IsBot || strings.EqualFold(strings.TrimSpace(u.Name), e.GetName()) {
			return true
		}
		_, ok := protectedTrips[u.Trip]
		return ok
	}
	cfg := moderation.JoinConfig{Enabled: true, JoinBurstCount: a.ModerationJoinBurstCount, JoinBurstWindow: time.Duration(a.ModerationJoinWindowSeconds) * time.Second, SameHashJoinCount: a.ModerationSameHashCount, SameHashJoinWindow: time.Duration(a.ModerationSameHashWindowSeconds) * time.Second, SuspiciousNameJoinCount: a.ModerationNameClusterCount, SuspiciousNameJoinWindow: time.Duration(a.ModerationNameClusterWindowSeconds) * time.Second, PostKickWindow: time.Duration(a.ModerationPostKickWindowSeconds) * time.Second, ActionCooldown: time.Duration(a.ModerationActionCooldownSeconds) * time.Second}
	return moderation.NewAutomation(moderation.NewJoinMonitor(cfg, time.Now, protected), moderation.NewEngineActionExecutor(e), 2*time.Second)
}

// ComposeMessageAutomation fails closed unless every detector setting and each
// fixed authoritative operation is available at composition time.
func ComposeMessageAutomation(e *core.EngineImpl, c *config.Config) *live.MessageAutomation {
	if e == nil || c == nil || !c.Agent.ModerationEnabled {
		return nil
	}
	a := c.Agent
	if a.ModerationMessageBurstCount <= 0 || a.ModerationMessageBurstWindowSeconds <= 0 || a.ModerationRepeatedMessageCount <= 0 || a.ModerationRepeatedMessageWindowSeconds <= 0 || a.ModerationSecondBreachWindowSeconds <= 0 || a.ModerationPostKickWindowSeconds <= 0 || a.ModerationActionCooldownSeconds <= 0 {
		return nil
	}
	if _, ok := e.Repository.(repository.ShadowBanRepository); !ok {
		return nil
	}
	protectedTrips := map[string]struct{}{}
	for _, trip := range append(append([]string{}, c.AdminTrips...), a.CreatorTrip) {
		if trip = strings.TrimSpace(trip); trip != "" {
			protectedTrips[trip] = struct{}{}
		}
	}
	protected := func(message model.ChatMessage) bool {
		if strings.EqualFold(strings.TrimSpace(message.Name), e.GetName()) {
			return true
		}
		if user := e.GetActiveUserByName(message.Name); user != nil {
			if user.Isme || user.IsBot {
				return true
			}
			_, ok := protectedTrips[user.Trip]
			return ok
		}
		return false
	}
	cfg := moderation.MessageConfig{Enabled: true, BurstCount: a.ModerationMessageBurstCount, BurstWindow: time.Duration(a.ModerationMessageBurstWindowSeconds) * time.Second, RepeatedCount: a.ModerationRepeatedMessageCount, RepeatedWindow: time.Duration(a.ModerationRepeatedMessageWindowSeconds) * time.Second, SecondBreachWindow: time.Duration(a.ModerationSecondBreachWindowSeconds) * time.Second, PostKickWindow: time.Duration(a.ModerationPostKickWindowSeconds) * time.Second, ActionCooldown: time.Duration(a.ModerationActionCooldownSeconds) * time.Second}
	return &live.MessageAutomation{Monitor: moderation.NewMessageMonitor(cfg, time.Now, protected), Executor: moderation.NewMessageActionExecutor(e), Timeout: 2 * time.Second}
}

// NewCoordinatedSessionFactory wires the real shared transport into the
// coordinator without registering the temporary session as a replica.
func NewCoordinatedSessionFactory(cfg transport.Config, registry *snapshot.TemporarySessionRegistry, coordinator *snapshot.RoomSnapshotCoordinator) *snapshot.CoordinatedSessionFactory {
	f := &snapshot.CoordinatedSessionFactory{Registry: registry, NewTransport: func(ctx context.Context, _ snapshot.RoomSnapshotRequest) (snapshot.TemporaryTransport, error) {
		return transport.NewConnection(cfg), nil
	}}
	if coordinator != nil {
		f.BindCoordinator(coordinator)
	}
	return f
}

// StartManaged is a convenience adapter for lifecycle callers.
func StartManaged(ctx context.Context, e *core.EngineImpl) error { return e.StartContext(ctx) }
