package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/transport"
)

type EngineTransport interface {
	Start(context.Context) error
	Messages() <-chan []byte
	Errors() <-chan error
	Connected() bool
	SendText(context.Context, string) error
	SendRaw(context.Context, []byte) error
	Close(context.Context) error
}

type ManagedEngine interface {
	common.Engine
	StartContext(context.Context) error
	StopContext(context.Context) error
	Healthy() bool
	EngineType() model.EngineType
	ReplicaChannels() []string
}

type ListenerProfile int

const (
	Permanent ListenerProfile = iota
	TemporaryOnlineSet
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
	Transport       EngineTransport
	Repository      repository.Repository

	//TODO: use a proper collection.
	CoreListener       common.Listener
	OnlineSetListener  common.Listener
	UserJoinedListener common.Listener
	UserChatListener   common.Listener
	UserLeftListener   common.Listener
	UserInfoListener   common.Listener

	SecurityService *service.SecurityService
	Services        *service.Bundle

	EnabledCommands      map[string]common.CommandMetadata
	usersMu              sync.RWMutex
	subscribersMu        sync.RWMutex
	subscribers          map[string]struct{}
	runtimeMu            sync.Mutex
	runtimeCancel        context.CancelFunc
	runtimeDone          chan struct{}
	joined               atomic.Bool
	Profile              ListenerProfile
	replicaController    *ManagedReplicaController
	LifecycleErrors      chan<- error
	runtimeFailure       func(error)
	transportErrReported atomic.Bool
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
	if e.Transport != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = e.StopContext(ctx)
		return
	}
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

func (e *EngineImpl) reportTransportError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, transport.ErrClosed) {
		return err
	}
	wrapped := fmt.Errorf("engine %s transport: %w", e.Channel, err)
	if e.transportErrReported.CompareAndSwap(false, true) {
		if e.runtimeFailure != nil {
			e.runtimeFailure(wrapped)
		} else if e.LifecycleErrors != nil {
			select {
			case e.LifecycleErrors <- wrapped:
			default:
			}
		}
	}
	return err
}

func (e *EngineImpl) StartContext(parent context.Context) error {
	if e.Transport == nil {
		return fmt.Errorf("managed transport is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	e.runtimeMu.Lock()
	if e.runtimeCancel != nil {
		e.runtimeMu.Unlock()
		return fmt.Errorf("engine already started")
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	e.runtimeCancel, e.runtimeDone = cancel, done
	e.runtimeMu.Unlock()
	if err := e.Transport.Start(ctx); err != nil {
		e.reportTransportError(err)
		cancel()
		e.runtimeMu.Lock()
		e.runtimeCancel = nil
		close(done)
		e.runtimeMu.Unlock()
		return err
	}
	if !e.joined.Swap(true) {
		p := fmt.Sprintf(`{ "cmd": "join", "channel": "%s", "nick": "%s#%s" }`, e.Channel, e.Name, e.Password)
		if err := e.Transport.SendText(ctx, p); err != nil {
			_ = e.StopContext(context.Background())
			return err
		}
	}
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-e.Transport.Messages():
				if msg != nil {
					e.DispatchMessage(string(msg))
				}
			case err := <-e.Transport.Errors():
				if err != nil {
					e.reportTransportError(err)
					cancel()
				}
				return
			}
		}
	}()
	return nil
}
func (e *EngineImpl) StopContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	e.runtimeMu.Lock()
	cancel, done := e.runtimeCancel, e.runtimeDone
	e.runtimeCancel = nil
	e.runtimeMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	err := e.Transport.Close(ctx)
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			if err == nil {
				err = ctx.Err()
			}
		}
	}
	return err
}
func (e *EngineImpl) Healthy() bool                { return e.Transport != nil && e.Transport.Connected() }
func (e *EngineImpl) EngineType() model.EngineType { return e.Type }
func (e *EngineImpl) ReplicaChannels() []string {
	if e.replicaController == nil {
		return nil
	}
	return e.replicaController.ReplicaChannels()
}
func (e *EngineImpl) SetReplicaController(c *ManagedReplicaController) { e.replicaController = c }
func (e *EngineImpl) SetRuntimeFailureHandler(fn func(error))          { e.runtimeFailure = fn }
func (e *EngineImpl) AddReplica(ctx context.Context, channel string) error {
	if e.replicaController == nil {
		return fmt.Errorf("replica controller is not configured")
	}
	return e.replicaController.AddReplica(ctx, channel)
}
func (e *EngineImpl) RemoveReplica(ctx context.Context, channel string) error {
	if e.replicaController == nil {
		return fmt.Errorf("replica controller is not configured")
	}
	return e.replicaController.RemoveReplica(ctx, channel)
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

func (e *EngineImpl) sendOutbound(message string) error {
	if e.Transport != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return e.Transport.SendText(ctx, message)
	}
	e.OutMessageQueue <- message
	return nil
}

func (e *EngineImpl) SendRawMessage(message string) {
	_ = e.sendOutbound(message)
}

func (e *EngineImpl) SendChatMessage(author, message string, IsWhisper bool) (string, error) {
	if author != "" && IsWhisper {
		message = "/whisper @" + author + " .\n" + message
	} else if author != "" {
		message = "@" + author + " " + message
	}

	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

func (e *EngineImpl) SendWhisperMessage(author, payload string) (string, error) {
	message := "/whisper @" + author + " " + strings.ReplaceAll(payload, `\n`, "\n")
	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

func (e *EngineImpl) SendAddressedMessage(author, payload string, whisper bool) (string, error) {
	payload = strings.ReplaceAll(payload, "\r\n", "\n")
	payload = strings.ReplaceAll(payload, "\r", "\n")
	payload = strings.ReplaceAll(payload, `\n`, "\n")
	message := "@" + author + " " + payload
	if whisper {
		message = "/whisper @" + author + " " + payload
	}
	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

func (e *EngineImpl) startSharingMessages() {
	defer e.EngineWg.Done()
	for msg := range e.OutMessageQueue {
		log.Println("sending: ", msg)
		e.HcConnection.Write(msg)
	}
}

func (e *EngineImpl) ReplaceActiveUsers(users []*model.User) {
	next := make(map[*model.User]struct{}, len(users))
	for _, u := range users {
		if u != nil {
			v := *u
			next[&v] = struct{}{}
		}
	}
	e.usersMu.Lock()
	e.ActiveUsers = next
	e.usersMu.Unlock()
}

func (e *EngineImpl) AddActiveUser(joined *model.User) {
	if joined == nil {
		return
	}
	e.usersMu.Lock()
	defer e.usersMu.Unlock()
	for u := range e.ActiveUsers {
		if model.IdentityKey(u.Trip, u.Hash, u.Name) == model.IdentityKey(joined.Trip, joined.Hash, joined.Name) {
			delete(e.ActiveUsers, u)
		}
	}
	e.ActiveUsers[joined] = struct{}{}
}

func (e *EngineImpl) SubscribeTrip(trip string) bool {
	trip = strings.TrimSpace(trip)
	if trip == "" {
		return false
	}
	e.subscribersMu.Lock()
	defer e.subscribersMu.Unlock()
	if e.subscribers == nil {
		e.subscribers = make(map[string]struct{})
	}
	if _, exists := e.subscribers[trip]; exists {
		return false
	}
	e.subscribers[trip] = struct{}{}
	return true
}

func (e *EngineImpl) UnsubscribeTrip(trip string) bool {
	trip = strings.TrimSpace(trip)
	e.subscribersMu.Lock()
	defer e.subscribersMu.Unlock()
	for current := range e.subscribers {
		if strings.EqualFold(current, trip) {
			delete(e.subscribers, current)
			return true
		}
	}
	return false
}

func (e *EngineImpl) IsSubscribedTrip(trip string) bool {
	e.subscribersMu.RLock()
	defer e.subscribersMu.RUnlock()
	for current := range e.subscribers {
		if strings.EqualFold(current, strings.TrimSpace(trip)) {
			return true
		}
	}
	return false
}

func (e *EngineImpl) GetSubscribedTrips() []string {
	e.subscribersMu.RLock()
	defer e.subscribersMu.RUnlock()
	out := make([]string, 0, len(e.subscribers))
	for trip := range e.subscribers {
		out = append(out, trip)
	}
	return out
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
			e.SendChatMessage(u.Name, " is not afk anymore - welcome back.", false)
			break
		}
	}
}

// TODO: improve to mention users by checking against trip of the mentioned user
func (e *EngineImpl) NotifyAfkIfMentioned(m *model.ChatMessage) {
	for a, reason := range e.AfkUsers {
		if strings.Contains(m.Text, a.Trip) || strings.Contains(m.Text, a.Name) {
			e.SendChatMessage(m.Name, fmt.Sprintf(" user: %s is afk, reason: %s", a.Name, reason), false)
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

func (e *EngineImpl) ServiceBundle() *service.Bundle { return e.Services }

func (e *EngineImpl) GetChannel() string {
	return e.Channel
}

func (e *EngineImpl) GetName() string {
	return e.Name
}

func (e *EngineImpl) Kick(name string, channel string) {
	p := fmt.Sprintf(`{ "cmd": "kick", "nick": "%s", "to": "%s" }`, name, channel)
	e.SendRawMessage(p)
}

func (e *EngineImpl) Ban(name string) {
	p := fmt.Sprintf(`{ "cmd": "ban", "nick": "%s" }`, name)
	e.SendRawMessage(p)
}

func (e *EngineImpl) Unban(hash string) {
	p := fmt.Sprintf(`{ "cmd": "unban", "hash": "%s" }`, hash)
	e.SendRawMessage(p)
}

func (e *EngineImpl) UnbanAll() {
	e.SendRawMessage(`{ "cmd": "unbanall" }`)
}

func (e *EngineImpl) Lock() {
	e.SendRawMessage(`{ "cmd": "lockroom" }`)
}

func (e *EngineImpl) Unlock() {
	e.SendRawMessage(`{ "cmd": "unlockroom" }`)
}

func (e *EngineImpl) RegisterCommand(c common.Command) {
	aliases := c.GetAliases()
	var constructorFn = func(msg *model.ChatMessage) common.Command {
		return c.NewInstance(e, msg)
	}

	for _, alias := range aliases {
		e.EnabledCommands[strings.ToLower(strings.TrimSpace(alias))] = common.CommandMetadata{
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
	if e.SecurityService == nil {
		return false
	}
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
