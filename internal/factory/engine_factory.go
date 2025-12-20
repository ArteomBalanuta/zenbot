package factory

import (
	"log"
	"net/url"
	"sync"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

func NewEngine(etype model.EngineType, c *config.Config, repo repository.Repository) common.Engine {
	u, err := url.Parse(c.WebsocketUrl)
	if err != nil {
		log.Fatalln("Can't parse websocket URL:", c.WebsocketUrl)
		panic("Error parsing Websocket URL")
	}

	e := core.EngineImpl{
		Type:     etype,
		Prefix:   c.CmdPrefix,
		Channel:  c.Channel,
		Name:     c.Name,
		Password: c.Password,

		EngineWg:        new(sync.WaitGroup),
		EnabledCommands: make(map[string]common.CommandMetadata),

		OutMessageQueue: make(chan string, 256),
		ActiveUsers:     make(map[*model.User]struct{}),
		AfkUsers:        make(map[*model.User]string),
	}

	e.CoreListener = core.NewCoreListener(e)
	e.HcConnection = core.NewConnection(u.String(), e.CoreListener)

	e.Repository = repo
	e.SecurityService = service.NewSecurityService(c)
	e.OnlineSetListener = core.NewOnlineSetListener(e, nil)

	e.UserChatListener = core.NewUserChatListener(e)
	e.UserInfoListener = core.NewInfoChatListener(e)
	e.UserJoinedListener = core.NewUserJoinedListener(e)
	e.UserLeftListener = core.NewUserLeftListener(e)

	if etype == model.ZOMBIE {
		e.Repository = &repository.DummyImpl{}
		e.UserChatListener = core.NewDummyListener()
		e.UserJoinedListener = core.NewDummyListener()
		e.UserLeftListener = core.NewDummyListener()
	}

	return &e
}
