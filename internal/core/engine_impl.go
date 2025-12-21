package core

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
)

type EngineImpl struct {
	Type     model.EngineType
	Prefix   string
	Channel  string
	Name     string
	Password string

	LastKickedUser    string
	LastKickedChannel string

	EngineWg *sync.WaitGroup

	OutMessageQueue chan string
	ActiveUsers     map[*model.User]struct{}
	AfkUsers        map[*model.User]string
	HcConnection    *Connection
	Repository      repository.Repository

	//TODO: use a proper collection.
	CoreListener       common.Listener
	OnlineSetListener  common.Listener
	UserJoinedListener common.Listener
	UserChatListener   common.Listener
	UserLeftListener   common.Listener
	UserInfoListener   common.Listener

	SecurityService *service.SecurityService

	EnabledCommands map[string]common.CommandMetadata
}

func (e *EngineImpl) Start() {
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

	e.EngineWg.Add(1)
	go e.startSharingMessages()
	e.EngineWg.Wait()

	fmt.Println("Engine WGroup stopped")
}

func (e *EngineImpl) Stop() {
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

func (e *EngineImpl) DispatchMessage(jsonMessage string) {
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

func (e *EngineImpl) SendRawMessage(message string) {
	e.OutMessageQueue <- message
}

func (e *EngineImpl) SendMessage(author, message string, IsWhisper bool) (string, error) {
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

func (e *EngineImpl) startSharingMessages() {
	defer e.EngineWg.Done()
	for msg := range e.OutMessageQueue {
		chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(msg))

		log.Println("sending: ", chatPayload)
		e.HcConnection.Write(chatPayload)
	}
}

func (e *EngineImpl) AddActiveUser(joined *model.User) {
	e.ActiveUsers[joined] = struct{}{}
}

func (e *EngineImpl) RemoveActiveUser(left *model.User) {
	for u := range e.ActiveUsers {
		if u.Name == left.Name {
			delete(e.ActiveUsers, u)
			break
		}
	}
}

func (e *EngineImpl) GetAfkUsers() *map[*model.User]string {
	return &e.AfkUsers
}

func (e *EngineImpl) AddAfkUser(u *model.User, reason string) {
	e.AfkUsers[u] = reason
	log.Printf("Added Afk User: %s, Trip: %s, Reason: %s", u.Name, u.Trip, reason)
}

func (e *EngineImpl) RemoveIfAfk(u *model.User) {
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
func (e *EngineImpl) NotifyAfkIfMentioned(m *model.ChatMessage) {
	for a, reason := range e.AfkUsers {
		if strings.Contains(m.Text, a.Trip) || strings.Contains(m.Text, a.Name) {
			e.SendMessage(m.Name, fmt.Sprintf(" user: %s is afk, reason: %s", a.Name, reason), false)
		}
	}
}

func (e *EngineImpl) GetActiveUserByName(name string) *model.User {
	for u := range e.ActiveUsers {
		if u.Name == name {
			return u
		}
	}
	return nil
}

func (e *EngineImpl) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	return e.Repository.LogMessage(trip, name, hash, message, channel)
}

func (e *EngineImpl) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	return e.Repository.LogMessage(trip, name, hash, eventType, channel)
}

func (e *EngineImpl) GetActiveUsers() *map[*model.User]struct{} {
	return &e.ActiveUsers
}

func (e *EngineImpl) GetChannel() string {
	return e.Channel
}

func (e *EngineImpl) GetName() string {
	return e.Name
}

func (e *EngineImpl) RegisterCommand(c common.Command) {
	aliases := c.GetAliases()
	var constructorFn = func(msg *model.ChatMessage) common.Command {
		return c.NewInstance(e, msg)
	}

	for _, alias := range aliases {
		e.EnabledCommands[alias] = common.CommandMetadata{
			Alias:   alias,
			Command: constructorFn,
		}
	}

	fmt.Printf("Registered command with aliases: %v\n", aliases)
}

func (e *EngineImpl) GetEnabledCommands() *map[string]common.CommandMetadata {
	return &e.EnabledCommands
}

func (e *EngineImpl) SetOnlineSetListener(l common.Listener) {
	e.OnlineSetListener = l
}

func (e *EngineImpl) SetLastKickedUser(u string) {
	e.LastKickedUser = u
}

func (e *EngineImpl) SetLastKickedChannel(c string) {
	e.LastKickedChannel = c
}

func (e *EngineImpl) WaitConnectionWgDone() {
	e.HcConnection.Wg.Wait()
}

func (e *EngineImpl) SetName(name string) {
	e.Name = name
}

func (e *EngineImpl) GetPrefix() string {
	return e.Prefix
}

func (e *EngineImpl) IsUserAuthorized(u *model.User, r *model.Role) bool {
	return e.SecurityService.IsAuthorized(u, r)
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
