package core

import (
	"log"
	"net/url"
)
import "zenbot/bot/contracts"
import "zenbot/bot/model"
import "zenbot/bot/config"
import "zenbot/bot/repository"
import "zenbot/bot/service"

type EngineFactory interface {
	NewEngine() contracts.EngineInterface
}

func NewEngine(etype model.EngineType, c *config.Config, repository *repository.Repository) *Engine {
	u, err := url.Parse(c.WebsocketUrl)
	if err != nil {
		log.Fatalln("Can't parse websocket URL:", c.WebsocketUrl)
		panic("Error parsing Websocket URL")
	}

	e := &Engine{
		eType:    etype,
		prefix:   c.CmdPrefix,
		Channel:  c.Channel,
		Name:     c.Name,
		Password: c.Password,

		EnabledCommands: make(map[string]CommandMetadata),

		OutMessageQueue: make(chan string, 256),
		ActiveUsers:     make(map[*model.User]struct{}),
	}

	e.Repository = repository
	e.SecurityService = service.NewSecurityService(c)

	e.CoreListener = NewCoreListener(e)
	e.UserChatListener = NewUserChatListener(e)
	e.OnlineSetListener = NewOnlineSetListener(e)
	e.UserJoinedListener = NewUserJoinedListener(e)
	e.UserLeftListener = NewUserLeftListener(e)

	e.HcConnection = NewConnection(u.String(), e.CoreListener)

	return e
}
