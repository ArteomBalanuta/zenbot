package command

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/repository"
	"zenbot/internal/util"
)

type commandBase struct {
	engine    common.Engine
	message   *model.ChatMessage
	role      model.Role
	aliases   []string
	canonical string
}

// DirectAgentSubmitter is the narrow command-to-live-runtime boundary.
type DirectAgentSubmitter interface {
	Submit(context.Context, *model.ChatMessage, string) error
}

// DirectAgentInvoker preserves the original synchronous composition contract.
// New command registration uses DirectAgentSubmitter so direct and ambient
// requests share one process-owned runtime.
type DirectAgentInvoker interface {
	Invoke(context.Context, *model.ChatMessage, string) (string, error)
}

type directLCommand struct {
	commandBase
	submitter DirectAgentSubmitter
}

func (c *directLCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	prompt := commandBody(c.message)
	if prompt == "" {
		return model.FAILED, fmt.Errorf("l requires a prompt")
	}
	if err := c.submitter.Submit(ctx, c.message, prompt); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *directLCommand) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	base := c.commandBase
	base.engine, base.message, base.aliases = e, m, c.Aliases()
	return &directLCommand{commandBase: base, submitter: c.submitter}
}

func directLDefinition(submitter DirectAgentSubmitter) (common.CommandDefinition, bool) {
	if submitter == nil {
		return common.CommandDefinition{}, false
	}
	definition, ok := commandDefinitionFor("l")
	if !ok {
		return common.CommandDefinition{}, false
	}
	definition.New = func(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
		return &directLCommand{commandBase: commandBase{engine: e, message: m, role: definition.Role, aliases: definition.Aliases, canonical: definition.Canonical}, submitter: submitter}
	}
	return definition, true
}

func (c *commandBase) Role() model.Role  { return c.role }
func (c *commandBase) Aliases() []string { return append([]string(nil), c.aliases...) }
func (c *commandBase) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.canonical, c.Aliases(), c.role, e, m)
}

// splitCommandToken consumes one structural token and its separator while
// preserving the remaining text byte-for-byte, including interior whitespace.
func splitCommandToken(text string) (token, tail string) {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	end := strings.IndexFunc(text, unicode.IsSpace)
	if end < 0 {
		return text, ""
	}
	_, separatorWidth := utf8.DecodeRuneInString(text[end:])
	return text[:end], text[end+separatorWidth:]
}

func commandBody(message *model.ChatMessage) string {
	if message == nil {
		return ""
	}
	_, body := splitCommandToken(message.Text)
	return strings.TrimSpace(body)
}
func args(m *model.ChatMessage) []string {
	a := m.GetArguments()
	if len(a) > 0 {
		return a[1:]
	}
	return nil
}

// rawRoomSelector removes only the optional leading room-link marker. Room
// names are otherwise source-owned and remain byte-for-byte unchanged.
func rawRoomSelector(raw string) (string, bool) {
	room := strings.TrimSpace(raw)
	room = strings.TrimPrefix(room, "?")
	return room, room != ""
}

func reply(c *commandBase, text string) error {
	_, err := c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper")
	return err
}

func replyContext(ctx context.Context, c *commandBase, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return reply(c, text)
}
func raw(c *commandBase, v any) { b, _ := json.Marshal(v); c.engine.SendRawMessage(string(b)) }

type sayCommand struct{ commandBase }

func (c *sayCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	message := commandBody(c.message)
	if _, err := c.engine.SendChatMessage("", message, false); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func rolePtr(r model.Role) *model.Role { return &r }

type afkCommand struct{ commandBase }

func (c *afkCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if strings.TrimSpace(c.message.Trip) == "" {
		if err := replyContext(ctx, &c.commandBase, "Set your trip in order to use this command"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	caller := c.engine.GetActiveUserByName(c.message.Name)
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if caller == nil || caller.Trip != c.message.Trip || !strings.EqualFold(caller.Name, strings.TrimSpace(c.message.Name)) {
		if err := replyContext(ctx, &c.commandBase, "Unable to verify the active caller."); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	reason := commandBody(c.message)
	c.engine.AddAfkUser(caller, reason)
	if err := replyContext(ctx, &c.commandBase, " is afk"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type listCommand struct{ commandBase }

type infoUserCommand struct{ commandBase }

func (c *infoUserCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		if err := replyContext(ctx, &c.commandBase, "\n Example: "+c.engine.GetPrefix()+"info merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	target, err := util.NormalizeNickTarget(&a[0])
	if err != nil {
		if err := replyContext(ctx, &c.commandBase, "\n Example: "+c.engine.GetPrefix()+"info merc"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	var user *model.User
	for u := range *c.engine.GetActiveUsers() {
		if u != nil && strings.EqualFold(target, u.Name) {
			user = u
			break
		}
	}
	if user == nil {
		if err := replyContext(ctx, &c.commandBase, "\n target with nick:  "+target+" not found!"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	text := "\n User trip: " + user.Trip + "\n User hash: " + user.Hash
	if err := observeAndReply(ctx, &c.commandBase, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func (c *listCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		out := formatSaturnUsers(*c.engine.GetActiveUsers())
		if err := replyContext(ctx, &c.commandBase, out); err != nil {
			return model.FAILED, err
		}
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"list programming"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if len(a) != 1 {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"list programming"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	channel, ok := rawRoomSelector(a[0])
	if !ok {
		if err := replyContext(ctx, &c.commandBase, "Example: "+c.engine.GetPrefix()+"list programming"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if channel != "" && channel != c.engine.GetChannel() {
		submitter, ok := c.engine.(common.CredentialedRoomSnapshotSubmitter)
		if !ok {
			return model.FAILED, fmt.Errorf("credentialed room snapshot submitter is not configured")
		}
		workflowID, err := listWorkflowID()
		if err != nil {
			return model.FAILED, err
		}
		if err := submitter.SubmitCredentialedRoomSnapshot(snapshot.RoomSnapshotRequest{
			Context:       ctx,
			WorkflowID:    workflowID,
			Author:        c.message.Name,
			Whisper:       c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper",
			SourceChannel: c.engine.GetChannel(),
			TargetChannel: channel,
			ReplyMessage:  "Unable to list users in the requested room.",
			Operation:     snapshot.NewListRoomOperation(),
		}); err != nil {
			return model.FAILED, err
		}
		return model.SUCCESSFUL, nil
	}
	if err := replyContext(ctx, &c.commandBase, formatSaturnUsers(*c.engine.GetActiveUsers())); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

func formatSaturnUsers(users map[*model.User]struct{}) string {
	list := make([]*model.User, 0, len(users))
	for u := range users {
		list = append(list, u)
	}
	return snapshot.FormatUsers(list)
}

var listWorkflowID = func() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate list workflow ID: %w", err)
	}
	return fmt.Sprintf("list-%x", bytes), nil
}

type banCommand struct{ commandBase }

func (c *banCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		return model.FAILED, nil
	}
	c.engine.Ban(a[0])
	if err := replyContext(ctx, &c.commandBase, a[0]+" has been banned"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type kickCommand struct{ commandBase }

func (c *kickCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	mode, rawTargets, ok := parseKickSelection(args(c.message))
	if !ok {
		return model.FAILED, nil
	}
	targets, err := resolveKickSelection(c.engine, mode, rawTargets)
	if err != nil {
		return model.FAILED, err
	}
	operations, err := moderationOperations(c.engine)
	if err != nil {
		return model.FAILED, err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
		if err := operations.KickNick(ctx, common.NickTarget(target.Name)); err != nil {
			return model.FAILED, err
		}
		if err := ctx.Err(); err != nil {
			return model.FAILED, err
		}
	}
	return model.SUCCESSFUL, nil
}

func parseKickSelection(arguments []string) (string, []string, bool) {
	if len(arguments) == 0 {
		return "", nil, false
	}
	switch arguments[0] {
	case "-m":
		if len(arguments) < 2 || containsKickMode(arguments[1:]) {
			return "", nil, false
		}
		return "multiple", arguments[1:], true
	case "-c":
		if len(arguments) != 2 || containsKickMode(arguments[1:]) {
			return "", nil, false
		}
		return "contains", arguments[1:], true
	default:
		if len(arguments) != 1 {
			return "", nil, false
		}
		return "exact", arguments, true
	}
}

func containsKickMode(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "-m" || argument == "-c" {
			return true
		}
	}
	return false
}

func resolveKickSelection(engine common.Engine, mode string, rawTargets []string) ([]*model.User, error) {
	if mode == "contains" {
		fragment := rawTargets[0]
		users := engine.GetActiveUsers()
		if users == nil {
			return nil, repository.ErrNotFound
		}
		candidates := make([]string, 0, len(*users))
		for user := range *users {
			if user != nil && strings.Contains(user.Name, fragment) {
				candidates = append(candidates, user.Name)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			left, right := strings.ToLower(candidates[i]), strings.ToLower(candidates[j])
			if left == right {
				return candidates[i] < candidates[j]
			}
			return left < right
		})
		rawTargets = candidates
	}

	targets := make([]*model.User, 0, len(rawTargets))
	seen := make(map[string]struct{}, len(rawTargets))
	for _, rawTarget := range rawTargets {
		var target *model.User
		if mode == "contains" {
			target = activeModerationTargetByCanonicalName(engine, rawTarget)
		} else {
			var err error
			target, err = activeModerationTarget(engine, rawTarget)
			if err != nil {
				return nil, err
			}
		}
		if target == nil {
			if mode == "exact" {
				return nil, repository.ErrNotFound
			}
			continue
		}
		canonical := strings.ToLower(target.Name)
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, repository.ErrNotFound
	}
	return targets, nil
}

// activeModerationTargetByCanonicalName resolves a source-owned nickname
// literally. Unlike raw command operands, canonical names must not have a
// leading mention marker stripped a second time.
func activeModerationTargetByCanonicalName(engine common.Engine, name string) *model.User {
	if user := engine.GetActiveUserByName(name); user != nil {
		return user
	}
	users := engine.GetActiveUsers()
	if users == nil {
		return nil
	}
	for user := range *users {
		if user != nil && strings.EqualFold(user.Name, name) {
			return user
		}
	}
	return nil
}

type unbanCommand struct{ commandBase }

func (c *unbanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		if err := replyContext(ctx, &c.commandBase, " user not found"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	c.engine.Unban(a[0])
	if err := replyContext(ctx, &c.commandBase, a[0]+" has been unbanned"); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type unbanAllCommand struct{ commandBase }

func (c *unbanAllCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	c.engine.UnbanAll()
	if err := replyContext(ctx, &c.commandBase, "mercy."); err != nil {
		return model.FAILED, err
	}
	return model.SUCCESSFUL, nil
}

type lockCommand struct{ commandBase }

func (c *lockCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || (a[0] != "on" && a[0] != "off") {
		if err := replyContext(ctx, &c.commandBase, c.engine.GetPrefix()+"lock [on|off]"); err != nil {
			return model.FAILED, err
		}
		return model.FAILED, nil
	}
	if a[0] == "on" {
		c.engine.Lock()
		if err := replyContext(ctx, &c.commandBase, " Room locked!"); err != nil {
			return model.FAILED, err
		}
	} else {
		c.engine.Unlock()
		if err := replyContext(ctx, &c.commandBase, " Room unlocked!"); err != nil {
			return model.FAILED, err
		}
	}
	return model.SUCCESSFUL, nil
}

func newCommand(canonical string, aliases []string, role model.Role, e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	b := commandBase{engine: e, message: m, role: role, aliases: aliases, canonical: canonical}
	switch canonical {
	case "sub", "unsub":
		return newSubscriptionCommand(canonical, aliases, role, e, m)
	case "mail":
		return &mailCommand{b}
	case "note":
		return &noteCommand{b}
	case "notes":
		return &notesCommand{b}
	case "ping":
		return &pingUtilityCommand{b}
	case "version":
		return &versionCommand{b}
	case "ape":
		return &apeCommand{b}
	case "coin":
		return &coinCommand{b}
	case "weather":
		return &weatherCommand{b}
	case "time":
		return &timeCommand{b}
	case "say":
		return &sayCommand{b}
	case "afk":
		return &afkCommand{b}
	case "list":
		return &listCommand{b}
	case "info":
		return &infoUserCommand{b}
	case "lastonline":
		return &lastonlineCommand{b}
	case "active":
		return &activityCommand{b}
	case "shadowbanlist":
		return &shadowBanListCommand{b}
	case "shadowban":
		return &shadowBanCommand{b}
	case "unshadowban":
		return &unshadowBanCommand{b}
	case "mute":
		return &muteCommand{b}
	case "unmute":
		return &unmuteCommand{b}
	case "color":
		return &colorCommand{b}
	case "flair":
		return &flairCommand{b}
	case "users":
		return &usersCommand{b}
	case "nicks":
		return &nicksCommand{b}
	case "nuke":
		return &nukeCommand{b}
	case "resurrect":
		return &resurrectCommand{b}
	case "help":
		return &helpCommand{b}
	case "ban":
		return &simpleBanCommand{b}
	case "kick":
		return &kickCommand{b}
	case "unban":
		return &simpleUnbanCommand{b}
	case "unbanall":
		return &simpleUnbanAllCommand{b}
	case "register":
		return &registerCommand{b}
	case "authorize":
		return &moderationIdentityCommand{commandBase: b}
	case "deauthorize":
		return &deauthorizeCommand{commandBase: b}
	case "access":
		return &accessCommand{b}
	case "messages":
		return &messagesCommand{b}
	case "remove":
		return &removeCommand{b}
	case "dbzregister", "dbzstats", "dbzstr", "dfight", "dbzhelp", "dspawn":
		return &dbzCommand{b}
	case "lock":
		return &simpleLockCommand{b}
	case "captcha":
		return &captchaCommand{b}
	case "overflow":
		return &overflowCommand{b}
	case "memory":
		return &memoryCommand{b}
	case "prefix":
		return &prefixCommand{b}
	case "automove":
		return &automoveCommand{b}
	case "replica":
		return &replicaCommand{b}
	case "replicaoff":
		return &replicaOffCommand{b}
	case "replicastatus":
		return &replicaStatusCommand{b}
	case "ws":
		return &supportRelayCommand{commandBase: b}
	case "wsa":
		return &supportRelayCommand{commandBase: b, anonymous: true}
	case "msgchannel":
		return &msgChannelCommand{commandBase: b}
	case "restart":
		return &restartCommand{commandBase: b}
	case "shutdown":
		return &shutdownCommand{commandBase: b}
	case "sql":
		return &sqlCommand{commandBase: b}
	default:
		return &saturnCommand{engine: e, message: m, role: role, aliases: aliases, canonical: canonical}
	}
}
func commandDefinitionFor(alias string) (common.CommandDefinition, bool) {
	if catalogError != nil {
		return common.CommandDefinition{}, false
	}
	if d, ok := validatedCatalog.Lookup(alias); ok {
		return d, true
	}
	return common.CommandDefinition{}, false
}
