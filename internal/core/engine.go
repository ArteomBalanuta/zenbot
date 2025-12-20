package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"zenbot/internal/config"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type Engine struct {
	eType    model.EngineType
	prefix   string
	Channel  string
	Name     string
	Password string

	lastKickedUser    string
	lastKickedChannel string

	engineWg *sync.WaitGroup

	OutMessageQueue chan string
	ActiveUsers     map[*model.User]struct{}
	AfkUsers        map[*model.User]string
	HcConnection    *Connection
	Repository      repository.Repository

	//TODO: use a proper collection.
	CoreListener       MessageListener
	OnlineSetListener  MessageListener
	UserJoinedListener MessageListener
	UserChatListener   MessageListener
	UserLeftListener   MessageListener
	UserInfoListener   MessageListener

	SecurityService *service.SecurityService

	EnabledCommands map[string]CommandMetadata
}

func NewEngine(etype model.EngineType, c *config.Config, repo repository.Repository) *Engine {
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

		engineWg:        new(sync.WaitGroup),
		EnabledCommands: make(map[string]CommandMetadata),

		OutMessageQueue: make(chan string, 256),
		ActiveUsers:     make(map[*model.User]struct{}),
		AfkUsers:        make(map[*model.User]string),
	}

	e.CoreListener = NewCoreListener(e)
	e.HcConnection = NewConnection(u.String(), e.CoreListener)

	e.Repository = repo
	e.SecurityService = service.NewSecurityService(c)
	e.OnlineSetListener = NewOnlineSetListener(e, nil)

	e.UserChatListener = NewUserChatListener(e)
	e.UserInfoListener = NewInfoChatListener(e)
	e.UserJoinedListener = NewUserJoinedListener(e)
	e.UserLeftListener = NewUserLeftListener(e)

	if etype == model.ZOMBIE {
		e.Repository = &repository.DummyImpl{}
		e.UserChatListener = NewDummyListener()
		e.UserJoinedListener = NewDummyListener()
		e.UserLeftListener = NewDummyListener()
	}

	return e
}

func (e *Engine) Start() {
	c := e.HcConnection
	c.Wg.Add(1)
	go c.Connect()

	for {
		if c.joinedRoom == false && c.IsWsConnected() {
			joinPayload := fmt.Sprintf(`{ "cmd": "join", "channel": "%s", "nick": "%s#%s" }`, e.Channel, e.Name, e.Password)

			c.Write(joinPayload)

			log.Println("Joining the room: ", e.Channel)
			c.joinedRoom = true

			break
		}
	}

	e.RegisterCommand(&List{})
	e.RegisterCommand(&Say{})
	e.RegisterCommand(&Afk{})

	e.engineWg.Add(1)
	go e.StartSharingMessages()
	e.engineWg.Wait()

	fmt.Println("Engine WGroup stopped")
}

func (e *Engine) Stop() {
	e.HcConnection.pingCancel()

	err := e.HcConnection.Close()
	if err != nil {
		fmt.Println("Error closing connection:", err)
		return
	}
	close(e.OutMessageQueue)

	e.HcConnection.Wg.Wait()
	fmt.Println("Connection WGroup finished.")
}

func (e *Engine) DispatchMessage(jsonMessage string) {
	// Parse into a map
	var data map[string]interface{}
	err := json.Unmarshal([]byte(jsonMessage), &data)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	// Extract "cmd"
	cmd, ok := data["cmd"].(string)
	if !ok {
		fmt.Println("Key 'cmd' not found or not a string")
		return
	}

	switch cmd {
	case "join":
	case "onlineSet":
		e.OnlineSetListener.Notify(jsonMessage)
	case "onlineAdd":
		e.UserJoinedListener.Notify(jsonMessage)
	case "onlineRemove":
		e.UserLeftListener.Notify(jsonMessage)
	case "chat":
		e.UserChatListener.Notify(jsonMessage)
	case "info":
		e.UserInfoListener.Notify(jsonMessage)
	case "session":
	default:
		log.Printf("Non functional payload: %s", jsonMessage)
	}
}

func (e *Engine) SendRawMessage(message string) {
	e.OutMessageQueue <- message
}

func (e *Engine) SendMessage(author, message string, IsWhisper bool) (string, error) {
	if strings.TrimSpace(author) == "" {
		return "", fmt.Errorf("author can't be null")
	}

	if IsWhisper {
		message = "/whisper @" + author + " .\n" + message
	} else {
		message = "@" + author + " " + message
	}

	e.OutMessageQueue <- message
	return message, nil
}

func (e *Engine) StartSharingMessages() {
	defer e.engineWg.Done()
	for msg := range e.OutMessageQueue {
		chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(msg))

		log.Println("sending: ", chatPayload)
		e.HcConnection.Write(chatPayload)
	}
}

func escapeJSON(input string) string {
	escaped, _ := json.Marshal(input)
	// Remove the surrounding quotes
	s := string(escaped[1 : len(escaped)-1])

	// Restore specific whitespace characters
	s = strings.ReplaceAll(s, `\n`, "\\n")
	s = strings.ReplaceAll(s, `\t`, "\\t")
	s = strings.ReplaceAll(s, `\r`, "\\r")

	return s
}

func (e *Engine) AddActiveUser(joined *model.User) {
	e.ActiveUsers[joined] = struct{}{}
}

func (e *Engine) RemoveActiveUser(left *model.User) {
	for u := range e.ActiveUsers {
		if u.Name == left.Name {
			delete(e.ActiveUsers, u)
			break
		}
	}
}

func (e *Engine) AddAfkUser(u *model.User, reason string) {
	e.AfkUsers[u] = reason
	log.Printf("Added Afk User: %s, Trip: %s, Reason: %s", u.Name, u.Trip, reason)
}

func (e *Engine) removeIfAfk(u *model.User) {
	for user := range e.AfkUsers {
		if (user.Name == u.Name) || (u.Trip != "" && user.Trip == u.Trip) {
			delete(e.AfkUsers, user)
			log.Printf("Removed Afk user %s", u.Name)
			e.SendMessage(u.Name, " is not afk anymore - welcome back.", false)
			break
		}
	}
}

// TODO: improve to mention users by checking against trip of the mentioned user
func (e *Engine) notifyAfkIfMentioned(m *model.ChatMessage) {
	for a, reason := range e.AfkUsers {
		if strings.Contains(m.Text, a.Trip) || strings.Contains(m.Text, a.Name) {
			e.SendMessage(m.Name, fmt.Sprintf(" user: %s is afk, reason: %s", a.Name, reason), false)
		}
	}
}

func (e *Engine) GetUserByName(name string) *model.User {
	for u := range e.ActiveUsers {
		if u.Name == name {
			return u
		}
	}
	return nil
}

func (e *Engine) GetActiveUsers() map[*model.User]struct{} {
	return e.ActiveUsers
}

func (e *Engine) GetChannel() string {
	return e.Channel
}

func ParseCommandText(text, prefix string) string {
	afterPrefix := text[len(prefix):]
	fields := strings.Fields(afterPrefix)
	return fields[0]
}

func (e *Engine) RegisterCommand(c Command) {
	aliases := c.GetAliases()
	var constructorFn = func(msg *model.ChatMessage) Command {
		return c.NewInstance(e, msg)
	}

	for _, alias := range aliases {
		e.EnabledCommands[alias] = CommandMetadata{
			Alias:   alias,
			Command: constructorFn,
		}
	}

	fmt.Printf("Registered command with aliases: %v\n", aliases)
}
