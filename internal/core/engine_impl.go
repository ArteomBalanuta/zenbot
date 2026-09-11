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
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
	"zenbot/internal/relay"
	"zenbot/internal/repository"
	"zenbot/internal/service"
	"zenbot/internal/transport"
)

type EngineTransport interface {
	Start(context.Context) error
	Messages() <-chan transport.InboundMessage
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
	prefixMu sync.RWMutex
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
	CommandProfiler *profiling.Profiler

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
	afkMu                sync.RWMutex
	subscribersMu        sync.RWMutex
	subscribers          map[string]struct{}
	runtimeMu            sync.Mutex
	runtimeCancel        context.CancelFunc
	runtimeDone          chan struct{}
	joined               atomic.Bool
	Profile              ListenerProfile
	hostRelay            relay.HostRelay
	replicaController    *ManagedReplicaController
	supportRelay         common.SupportReplicaRelay
	autoMove             *AutoMoveState
	autoMoveMu           sync.Mutex
	snapshotCoordinator  *snapshot.RoomSnapshotCoordinator
	LifecycleErrors      chan<- error
	HostLifecycle        common.HostLifecycleController
	runtimeFailure       func(error)
	transportErrReported atomic.Bool
}

// NewEngineImpl installs the construction-time relay dependency. The
// dependency remains private to EngineImpl after construction.
func NewEngineImpl(e *EngineImpl, host relay.HostRelay) *EngineImpl {
	if e == nil {
		return nil
	}
	e.hostRelay = host
	return e
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
		messages := e.Transport.Messages()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-messages:
				if msg.Payload != nil {
					messageCtx := ctx
					if e.CommandProfiler.Enabled() {
						messageCtx = profiling.WithIngress(ctx, profiling.Ingress{
							ReceivedAt: msg.ReceivedAt,
							DequeuedAt: time.Now(),
							QueueDepth: len(messages),
						})
					}
					e.DispatchMessageContext(messageCtx, string(msg.Payload))
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

// PerformanceProfiler exposes process-owned latency diagnostics to listener boundaries.
func (e *EngineImpl) PerformanceProfiler() *profiling.Profiler { return e.CommandProfiler }

// SetAutoMoveState installs the process-shared state during permanent engine composition.
func (e *EngineImpl) SetAutoMoveState(state *AutoMoveState) { e.autoMove = state }

// AutoMoveState exposes the installed composition dependency for construction tests.
func (e *EngineImpl) AutoMoveState() *AutoMoveState { return e.autoMove }

// HostRelay returns the relay dependency installed at AGENT construction.
func (e *EngineImpl) HostRelay() relay.HostRelay { return e.hostRelay }

func (e *EngineImpl) ReplicaChannels() []string {
	if e.replicaController == nil {
		return nil
	}
	return e.replicaController.ReplicaChannels()
}
func (e *EngineImpl) SetReplicaController(c *ManagedReplicaController)        { e.replicaController = c }
func (e *EngineImpl) SetSupportReplicaRelay(relay common.SupportReplicaRelay) { e.supportRelay = relay }
func (e *EngineImpl) SetHostLifecycleController(controller common.HostLifecycleController) {
	e.HostLifecycle = controller
}
func (e *EngineImpl) HostLifecycleController() common.HostLifecycleController { return e.HostLifecycle }
func (e *EngineImpl) RelayToSupport(ctx context.Context, request common.SupportRelayRequest) error {
	if e.supportRelay == nil {
		return fmt.Errorf("support replica relay is not configured")
	}
	return e.supportRelay.RelayToSupport(ctx, request)
}
func (e *EngineImpl) SetRuntimeFailureHandler(fn func(error)) { e.runtimeFailure = fn }
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

func (e *EngineImpl) AutoMoveSnapshot() common.AutoMoveSnapshot {
	if e.autoMove == nil {
		return common.AutoMoveSnapshot{}
	}
	return e.autoMove.Snapshot()
}

func (e *EngineImpl) ConfigureAutoMove(source, destination string) (common.AutoMoveSnapshot, error) {
	if e.autoMove == nil {
		return common.AutoMoveSnapshot{}, fmt.Errorf("automove state is not configured")
	}
	return e.autoMove.Configure(source, destination)
}

func (e *EngineImpl) EnableAutoMove(ctx context.Context) (common.AutoMoveSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return e.AutoMoveSnapshot(), err
	}
	e.autoMoveMu.Lock()
	defer e.autoMoveMu.Unlock()
	if err := ctx.Err(); err != nil {
		return e.AutoMoveSnapshot(), err
	}
	if e.autoMove == nil {
		return common.AutoMoveSnapshot{}, fmt.Errorf("automove state is not configured")
	}
	snapshot := e.autoMove.SetEnabled(true)
	present := make(map[string]struct{}, len(e.ReplicaChannels()))
	for _, channel := range e.ReplicaChannels() {
		present[channel] = struct{}{}
	}
	for _, source := range snapshot.Sources {
		if _, ok := present[source]; ok {
			continue
		}
		if err := e.AddReplica(ctx, source); err != nil {
			return e.AutoMoveSnapshot(), err
		}
	}
	return e.AutoMoveSnapshot(), nil
}

func (e *EngineImpl) DisableAutoMove(ctx context.Context) (common.AutoMoveSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return e.AutoMoveSnapshot(), err
	}
	e.autoMoveMu.Lock()
	defer e.autoMoveMu.Unlock()
	if err := ctx.Err(); err != nil {
		return e.AutoMoveSnapshot(), err
	}
	if e.autoMove == nil {
		return common.AutoMoveSnapshot{}, fmt.Errorf("automove state is not configured")
	}
	snapshot := e.autoMove.SetEnabled(false)
	present := make(map[string]struct{}, len(e.ReplicaChannels()))
	for _, channel := range e.ReplicaChannels() {
		present[channel] = struct{}{}
	}
	var first error
	for _, source := range snapshot.Sources {
		if _, ok := present[source]; !ok {
			continue
		}
		if err := e.RemoveReplica(ctx, source); err != nil && first == nil {
			first = err
		}
	}
	return e.AutoMoveSnapshot(), first
}

func (e *EngineImpl) DispatchMessage(jsonMessage string) {
	e.DispatchMessageContext(context.Background(), jsonMessage)
}

func (e *EngineImpl) DispatchMessageContext(ctx context.Context, jsonMessage string) {
	if ctx == nil {
		ctx = context.Background()
	}
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
		if listener, ok := e.UserChatListener.(interface {
			NotifyContext(context.Context, string)
		}); ok {
			listener.NotifyContext(ctx, jsonMessage)
		} else {
			e.UserChatListener.Notify(jsonMessage)
		}
	case "info":
		e.UserInfoListener.Notify(jsonMessage)
	case "session":
	default:
		log.Printf("Non functional payload: %s", jsonMessage)
	}
}

func (e *EngineImpl) sendOutbound(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return e.sendOutboundContext(ctx, message)
}

func (e *EngineImpl) sendOutboundContext(ctx context.Context, message string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if e.Transport != nil {
		return e.Transport.SendText(ctx, message)
	}
	select {
	case e.OutMessageQueue <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *EngineImpl) SendRawMessage(message string) {
	_ = e.sendOutbound(message)
}

func (e *EngineImpl) SendChatMessage(author, message string, IsWhisper bool) (string, error) {
	if author != "" {
		message = normalizeChatText(message)
	}
	if author != "" && IsWhisper {
		message = "/whisper @" + author + " .\n" + message
	} else if author != "" {
		message = "@" + author + " " + message
	}

	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

func normalizeChatText(message string) string {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.ReplaceAll(message, "\r", "\n")
	return strings.ReplaceAll(message, `\n`, "\n")
}

func (e *EngineImpl) SendWhisperMessage(author, payload string) (string, error) {
	message := "/whisper @" + author + " " + strings.ReplaceAll(payload, `\n`, "\n")
	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

func (e *EngineImpl) SendAddressedMessage(author, payload string, whisper bool) (string, error) {
	payload = normalizeChatText(payload)
	message := "@" + author + " " + payload
	if whisper {
		message = "/whisper @" + author + " " + payload
	}
	chatPayload := fmt.Sprintf(`{ "cmd": "chat", "text": "%s"}`, escapeJSON(message))
	return message, e.sendOutbound(chatPayload)
}

// LogCommand persists a command outcome when the engine repository supports
// Saturn's command-audit contract.
func (e *EngineImpl) LogCommand(ctx context.Context, record model.CommandAuditRecord) (int64, error) {
	if e.Repository == nil {
		return 0, errors.New("command audit repository is not configured")
	}
	return e.Repository.LogCommand(ctx, record)
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
			next[copyRoomUser(u)] = struct{}{}
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
	owned := copyRoomUser(joined)
	e.usersMu.Lock()
	defer e.usersMu.Unlock()
	if e.ActiveUsers == nil {
		e.ActiveUsers = make(map[*model.User]struct{})
	}
	for u := range e.ActiveUsers {
		if model.IdentityKey(u.Trip, u.Hash, u.Name) == model.IdentityKey(owned.Trip, owned.Hash, owned.Name) {
			delete(e.ActiveUsers, u)
		}
	}
	e.ActiveUsers[owned] = struct{}{}
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
	if _, exists := e.subscribers[trip]; exists {
		delete(e.subscribers, trip)
		return true
	}
	return false
}

func (e *EngineImpl) IsSubscribedTrip(trip string) bool {
	e.subscribersMu.RLock()
	defer e.subscribersMu.RUnlock()
	_, exists := e.subscribers[strings.TrimSpace(trip)]
	return exists
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
	if left == nil {
		return
	}
	e.usersMu.Lock()
	defer e.usersMu.Unlock()
	for u := range e.ActiveUsers {
		if strings.EqualFold(u.Name, left.Name) {
			delete(e.ActiveUsers, u)
			break
		}
	}
}

func (e *EngineImpl) GetAfkUsers() *map[*model.User]string {
	e.afkMu.RLock()
	defer e.afkMu.RUnlock()
	snapshot := make(map[*model.User]string, len(e.AfkUsers))
	for user, reason := range e.AfkUsers {
		snapshot[copyRoomUser(user)] = reason
	}
	return &snapshot
}

func (e *EngineImpl) AddAfkUser(u *model.User, reason string) {
	if u == nil {
		return
	}
	owned := copyRoomUser(u)
	e.afkMu.Lock()
	if e.AfkUsers == nil {
		e.AfkUsers = make(map[*model.User]string)
	}
	identity := model.IdentityKey(owned.Trip, owned.Hash, owned.Name)
	for current := range e.AfkUsers {
		if model.IdentityKey(current.Trip, current.Hash, current.Name) == identity {
			delete(e.AfkUsers, current)
		}
	}
	e.AfkUsers[owned] = reason
	e.afkMu.Unlock()
	log.Printf("Added Afk User: %s, Trip: %s, Reason: %s", owned.Name, owned.Trip, reason)
}

func (e *EngineImpl) RenameAfkUser(before, after string) {
	before = strings.TrimSpace(before)
	after = strings.TrimSpace(after)
	e.afkMu.Lock()
	defer e.afkMu.Unlock()
	for user := range e.AfkUsers {
		if strings.EqualFold(user.Name, before) {
			user.Name = after
		}
	}
}

func (e *EngineImpl) RemoveIfAfk(u *model.User) {
	if u == nil {
		return
	}
	name, trip := u.Name, u.Trip
	removed := false
	e.afkMu.Lock()
	for user := range e.AfkUsers {
		if user.Name == name || (trip != "" && user.Trip == trip) {
			delete(e.AfkUsers, user)
			removed = true
			break
		}
	}
	e.afkMu.Unlock()
	if removed {
		log.Printf("Removed Afk user %s", name)
		_, _ = e.SendChatMessage(name, " is not afk anymore - welcome back.", false)
	}
}

// TODO: improve to mention users by checking against trip of the mentioned user
func (e *EngineImpl) NotifyAfkIfMentioned(m *model.ChatMessage) {
	if m == nil {
		return
	}
	text, author := m.Text, m.Name
	snapshot := e.GetAfkUsers()
	for user, reason := range *snapshot {
		mentionedTrip := user.Trip != "" && strings.Contains(text, user.Trip)
		mentionedName := user.Name != "" && strings.Contains(text, user.Name)
		if mentionedTrip || mentionedName {
			_, _ = e.SendChatMessage(author, fmt.Sprintf(" user: %s is afk, reason: %s", user.Name, reason), false)
		}
	}
}

func (e *EngineImpl) GetActiveUserByName(name string) *model.User {
	e.usersMu.RLock()
	defer e.usersMu.RUnlock()
	for u := range e.ActiveUsers {
		if strings.EqualFold(u.Name, strings.TrimSpace(name)) {
			return copyRoomUser(u)
		}
	}
	return nil
}

func (e *EngineImpl) LogMessage(trip, name, hash, message, channel string) (int64, error) {
	return e.Repository.LogMessage(trip, name, hash, message, channel)
}

func (e *EngineImpl) LogPresence(trip, name, hash, eventType, channel string) (int64, error) {
	return e.Repository.LogPresence(trip, name, hash, eventType, channel)
}

func (e *EngineImpl) LogMessageRecord(ctx context.Context, record model.MessageRecord) (int64, error) {
	auditor, ok := e.Repository.(repository.AuditRepository)
	if !ok {
		return 0, errors.New("typed message audit repository is not configured")
	}
	return auditor.MessageAudit(ctx, record)
}

func (e *EngineImpl) IsManagedBotName(name string) bool {
	if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(e.Name)) {
		return true
	}
	if e.replicaController == nil || e.replicaController.manager == nil {
		return false
	}
	for _, replica := range e.replicaController.manager.ManagedEngines() {
		if replica != nil && strings.EqualFold(replica.GetName(), name) {
			return true
		}
	}
	return false
}

func (e *EngineImpl) GetActiveUsers() *map[*model.User]struct{} {
	e.usersMu.RLock()
	defer e.usersMu.RUnlock()
	snapshot := make(map[*model.User]struct{}, len(e.ActiveUsers))
	for user := range e.ActiveUsers {
		snapshot[copyRoomUser(user)] = struct{}{}
	}
	return &snapshot
}

// ActiveUserNames returns a read-safe immutable active-user snapshot.
func (e *EngineImpl) ActiveUserNames() []string {
	e.usersMu.RLock()
	defer e.usersMu.RUnlock()
	names := make([]string, 0, len(e.ActiveUsers))
	for user := range e.ActiveUsers {
		names = append(names, user.Name)
	}
	return names
}

func (e *EngineImpl) ServiceBundle() *service.Bundle { return e.Services }

func (e *EngineImpl) GetChannel() string {
	return e.Channel
}

func (e *EngineImpl) GetName() string {
	return e.Name
}

func (e *EngineImpl) ShadowBan(ctx context.Context, principal string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	repo, ok := e.Repository.(repository.ShadowBanRepository)
	if !ok {
		return fmt.Errorf("authoritative shadow-ban repository is unavailable")
	}
	user := e.GetActiveUserByName(principal)
	if user == nil {
		return fmt.Errorf("shadow-ban principal %q is not active", principal)
	}
	copy := *user
	return repo.PersistShadowBan(ctx, copy, "repeated same-hash raid")
}

// WarnFlood emits the source-defined non-whisper warning through the engine's
// outbound transport; callers cannot supply message content or whisper mode.
func (e *EngineImpl) WarnFlood(ctx context.Context, principal string) error {
	name, err := e.activePrincipal(principal)
	if err != nil {
		return err
	}
	return e.sendModerationCommand(ctx, "chat", map[string]string{"text": "@" + name + " Please stop flooding."})
}

// MutePrincipal sends the only autonomous mute protocol shape.
func (e *EngineImpl) MutePrincipal(ctx context.Context, principal string) error {
	name, err := e.activePrincipal(principal)
	if err != nil {
		return err
	}
	return e.sendModerationCommand(ctx, "mute", map[string]string{"nick": name})
}

// KickPrincipal sends the only autonomous current-room kick protocol shape.
func (e *EngineImpl) KickPrincipal(ctx context.Context, principal string) error {
	name, err := e.activePrincipal(principal)
	if err != nil {
		return err
	}
	return e.sendModerationCommand(ctx, "kick", map[string]string{"nick": name})
}

func (e *EngineImpl) activePrincipal(principal string) (string, error) {
	user := e.GetActiveUserByName(principal)
	if user == nil || strings.TrimSpace(user.Name) == "" {
		return "", fmt.Errorf("message moderation principal %q is not active", principal)
	}
	return user.Name, nil
}

// sendModerationCommand remains for the QA-approved reversal operation. New
// manual moderation operations use the closed typed payload boundary instead.
func (e *EngineImpl) sendModerationCommand(ctx context.Context, command string, values map[string]string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := make(map[string]string, len(values)+1)
	payload["cmd"] = command
	for key, value := range values {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return e.sendOutboundContext(ctx, string(encoded))
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
	e.registerCommandFor(c, e)
}

func (e *EngineImpl) registerCommandFor(c common.Command, commandEngine common.Engine) {
	aliases := c.GetAliases()
	var constructorFn = func(msg *model.ChatMessage) common.Command {
		return c.NewInstance(commandEngine, msg)
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

func (e *EngineImpl) SetPrefix(prefix string) {
	e.Prefix = prefix
	if e.replicaController != nil {
		e.replicaController.SetPrefix(prefix)
	}
}

func (e *EngineImpl) GetPrefix() string {
	e.prefixMu.RLock()
	defer e.prefixMu.RUnlock()
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
