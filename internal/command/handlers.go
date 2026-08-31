package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"zenbot/internal/agent/runtime"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type commandBase struct {
	engine    common.Engine
	message   *model.ChatMessage
	role      model.Role
	aliases   []string
	canonical string
}

// DirectAgentInvoker is the narrow direct-command boundary supplied by application composition.
type DirectAgentInvoker interface {
	Invoke(context.Context, *model.ChatMessage, string) (string, error)
}

type directLCommand struct {
	commandBase
	invoker DirectAgentInvoker
}

func (c *directLCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	prompt := strings.TrimSpace(strings.Join(args(c.message), " "))
	if prompt == "" {
		return model.FAILED, fmt.Errorf("l requires a prompt")
	}
	var text string
	var completion runtime.DirectCompletion
	var hasCompletion bool
	var err error
	if artifact, ok := c.invoker.(interface {
		InvokeCompletion(context.Context, *model.ChatMessage, string) (runtime.DirectCompletion, error)
	}); ok {
		completion, err = artifact.InvokeCompletion(ctx, c.message, prompt)
		text, hasCompletion = completion.Text(), true
	} else {
		text, err = c.invoker.Invoke(ctx, c.message, prompt)
	}
	if err != nil {
		return model.FAILED, err
	}
	if strings.TrimSpace(text) == "" {
		return model.SUCCESSFUL, nil
	}
	if _, err := c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper"); err != nil {
		return model.FAILED, err
	}
	if hasCompletion {
		if persistent, ok := c.invoker.(interface {
			PersistDelivery(context.Context, *model.ChatMessage, string, runtime.DirectCompletion) error
		}); ok {
			if err := persistent.PersistDelivery(ctx, c.message, prompt, completion); err != nil {
				log.Printf("agent tool evidence persistence failed")
			}
		}
	} else if persistent, ok := c.invoker.(interface {
		Persist(context.Context, *model.ChatMessage, string, string) error
	}); ok {
		if err := persistent.Persist(ctx, c.message, prompt, text); err != nil {
			log.Printf("agent memory persistence failed")
		}
	}
	return model.SUCCESSFUL, nil
}

func directLDefinition(invoker DirectAgentInvoker) (common.CommandDefinition, bool) {
	if invoker == nil {
		return common.CommandDefinition{}, false
	}
	definition, ok := commandDefinitionFor("l")
	if !ok {
		return common.CommandDefinition{}, false
	}
	definition.New = func(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
		return &directLCommand{commandBase: commandBase{engine: e, message: m, role: definition.Role, aliases: definition.Aliases, canonical: definition.Canonical}, invoker: invoker}
	}
	return definition, true
}

func (c *commandBase) Role() model.Role  { return c.role }
func (c *commandBase) Aliases() []string { return append([]string(nil), c.aliases...) }
func (c *commandBase) NewInstance(e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	return newCommand(c.aliases[0], c.aliases, c.role, e, m)
}
func args(m *model.ChatMessage) []string {
	a := m.GetArguments()
	if len(a) > 0 {
		return a[1:]
	}
	return nil
}
func reply(c *commandBase, text string) {
	_, _ = c.engine.SendChatMessage(c.message.Name, text, c.message.IsWhisper || c.message.Whisper || c.message.Type == "whisper")
}
func raw(c *commandBase, v any) { b, _ := json.Marshal(v); c.engine.SendRawMessage(string(b)) }

type sayCommand struct{ commandBase }

func (c *sayCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	message := strings.Join(args(c.message), " ") + " "
	if user := c.engine.GetActiveUserByName(c.message.Name); user == nil || !c.engine.IsUserAuthorized(user, rolePtr(model.ADMIN)) {
		message = strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
				return r
			}
			return -1
		}, message)
	}
	c.engine.SendChatMessage("", message, false)
	return model.SUCCESSFUL, nil
}

func rolePtr(r model.Role) *model.Role { return &r }

type afkCommand struct{ commandBase }

func (c *afkCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	if c.message.Trip == "" {
		reply(&c.commandBase, "Set your trip in order to use this command")
		return model.FAILED, nil
	}
	reason := strings.Join(args(c.message), " ")
	for u := range *c.engine.GetActiveUsers() {
		if u.Trip == c.message.Trip {
			c.engine.AddAfkUser(u, reason)
		}
	}
	reply(&c.commandBase, " is afk")
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
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"info merc")
		return model.FAILED, nil
	}
	target := strings.TrimSpace(a[0])
	if strings.HasPrefix(target, "@") {
		target = strings.TrimSpace(strings.TrimPrefix(target, "@"))
	}
	if target == "" {
		reply(&c.commandBase, "\\n Example: "+c.engine.GetPrefix()+"info merc")
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
		reply(&c.commandBase, "\\n target with nick:  "+target+" not found!")
		return model.FAILED, nil
	}
	reply(&c.commandBase, "\n User trip: "+user.Trip+"\n User hash: "+user.Hash)
	return model.SUCCESSFUL, nil
}

func (c *listCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		out := formatSaturnUsers(*c.engine.GetActiveUsers())
		reply(&c.commandBase, out)
		reply(&c.commandBase, "Example: "+c.engine.GetPrefix()+"list programming")
		return model.FAILED, nil
	}
	if strings.TrimSpace(a[0]) != "" && strings.TrimSpace(a[0]) != c.engine.GetChannel() {
		// Remote snapshot/listing is not available through the target Engine contract.
		return model.FAILED, nil
	}
	reply(&c.commandBase, formatSaturnUsers(*c.engine.GetActiveUsers()))
	return model.SUCCESSFUL, nil
}

func formatSaturnUsers(users map[*model.User]struct{}) string {
	out := ""
	for u := range users {
		trip := u.Trip
		if trip == "" {
			trip = "------"
		}
		out += "\n" + u.Hash + " | " + trip + " | " + u.Name + "\n"
	}
	return out
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
	reply(&c.commandBase, a[0]+" has been banned")
	return model.SUCCESSFUL, nil
}

type kickCommand struct{ commandBase }

func (c *kickCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 {
		return model.FAILED, nil
	}
	if c.engine.GetActiveUserByName(a[0]) == nil {
		reply(&c.commandBase, " user not found")
		return model.FAILED, nil
	}
	c.engine.Kick(a[0], "abcdef")
	reply(&c.commandBase, " user has been kicked")
	return model.SUCCESSFUL, nil
}

type unbanCommand struct{ commandBase }

func (c *unbanCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || strings.TrimSpace(a[0]) == "" {
		reply(&c.commandBase, " user not found")
		return model.FAILED, nil
	}
	c.engine.Unban(a[0])
	reply(&c.commandBase, a[0]+" has been unbanned")
	return model.SUCCESSFUL, nil
}

type unbanAllCommand struct{ commandBase }

func (c *unbanAllCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	c.engine.UnbanAll()
	reply(&c.commandBase, "mercy.")
	return model.SUCCESSFUL, nil
}

type lockCommand struct{ commandBase }

func (c *lockCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	a := args(c.message)
	if len(a) == 0 || (a[0] != "on" && a[0] != "off") {
		reply(&c.commandBase, c.engine.GetPrefix()+"lock [on|off]")
		return model.FAILED, nil
	}
	if a[0] == "on" {
		c.engine.Lock()
		reply(&c.commandBase, " Room locked!")
	} else {
		c.engine.Unlock()
		reply(&c.commandBase, " Room unlocked!")
	}
	return model.SUCCESSFUL, nil
}

type unlockCommand struct{ commandBase }

func (c *unlockCommand) Execute(ctx context.Context) (model.Status, error) {
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	c.engine.Unlock()
	reply(&c.commandBase, " room unlocked")
	return model.SUCCESSFUL, nil
}

func newCommand(canonical string, aliases []string, role model.Role, e common.Engine, m *model.ChatMessage) common.SaturnCommand {
	b := commandBase{engine: e, message: m, role: role, aliases: aliases, canonical: canonical}
	switch canonical {
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
	case "users":
		return &usersCommand{b}
	case "nicks":
		return &nicksCommand{b}
	case "help":
		return &helpCommand{b}
	case "ban":
		return &banCommand{b}
	case "kick":
		return &kickCommand{b}
	case "unban":
		return &unbanCommand{b}
	case "unbanall":
		return &unbanAllCommand{b}
	case "register":
		return &registerCommand{b}
	case "authorize":
		return &authorizeCommand{b}
	case "access":
		return &accessCommand{b}
	case "messages":
		return &messagesCommand{b}
	case "remove":
		return &removeCommand{b}
	case "dbzregister", "dbzstats", "dbzstr", "dfight", "dbzhelp", "dspawn":
		return &dbzCommand{b}
	case "lock":
		return &lockCommand{b}
	case "unlock":
		return &unlockCommand{b}
	case "memory":
		return &memoryCommand{b}
	default:
		return &saturnCommand{engine: e, message: m, role: role, aliases: aliases, canonical: canonical}
	}
}
func commandDefinitionFor(alias string) (common.CommandDefinition, bool) {
	for _, d := range catalog() {
		for _, a := range append([]string{d.Canonical}, d.Aliases...) {
			if strings.EqualFold(a, alias) {
				return d, true
			}
		}
	}
	if strings.EqualFold(alias, "unlock") || strings.EqualFold(alias, "unlockroom") {
		return def("unlock", []string{"unlock", "unlockroom"}, model.MODERATOR), true
	}
	return common.CommandDefinition{}, false
}
