package factory

import (
	"log"
	"net/url"
	"sync"
	"zenbot/internal/common"
	"zenbot/internal/config"
	"zenbot/internal/core"
	"zenbot/internal/listener"
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

	e := &core.EngineImpl{
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

	e.CoreListener = listener.NewCoreListener(e)
	e.HcConnection = core.NewConnection(u.String(), e.CoreListener)

	e.Repository = repo
	e.SecurityService = service.NewSecurityService(c)
	e.OnlineSetListener = listener.NewOnlineSetListener(e, nil)

	e.UserChatListener = listener.NewUserChatListener(e)
	e.UserInfoListener = listener.NewInfoChatListener(e)
	e.UserJoinedListener = listener.NewUserJoinedListener(e)
	e.UserLeftListener = listener.NewUserLeftListener(e)

	if etype == model.ZOMBIE {
		e.Repository = &repository.DummyImpl{}
		e.UserChatListener = common.NewDummyListener()
		e.UserJoinedListener = common.NewDummyListener()
		e.UserLeftListener = common.NewDummyListener()
	}

	return e
}
