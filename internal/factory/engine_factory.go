package factory

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/listener"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
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
	if opts.ListenerProfile != core.TemporaryOnlineSet {
		opts.ListenerProfile = core.Permanent
	}
	if opts.Transport.URL == "" {
		opts.Transport.URL = c.WebsocketUrl
	}
	e := &core.EngineImpl{Type: etype, Prefix: c.CmdPrefix, Channel: c.Channel, Name: c.Name, Password: c.Password,
		EngineWg: new(sync.WaitGroup), EnabledCommands: make(map[string]common.CommandMetadata), OutMessageQueue: make(chan string, 256),
		ActiveUsers: make(map[*model.User]struct{}), AfkUsers: make(map[*model.User]string), Transport: transport.NewConnection(opts.Transport), Profile: opts.ListenerProfile, LifecycleErrors: opts.LifecycleErrors}
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
		e.UserJoinedListener, e.UserLeftListener = listener.NewUserJoinedListener(e), listener.NewUserLeftListener(e)
	}
	if etype == model.ZOMBIE {
		e.Repository = &repository.DummyImpl{}
		e.UserChatListener, e.UserJoinedListener, e.UserLeftListener = common.NewDummyListener(), common.NewDummyListener(), common.NewDummyListener()
	}
	return e, nil
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
