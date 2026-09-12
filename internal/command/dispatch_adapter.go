package command

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/profiling"
)

// legacyAdapter bridges Saturn's context-aware command contract to Zenbot's
// existing inbound Command registration contract. The legacy contract does not
// carry a context or return an error; Execute retains background compatibility
// and errors remain in the existing log-based behavior.
type legacyAdapter struct {
	engine common.Engine
	def    common.CommandDefinition
	msg    *model.ChatMessage
}

func (a *legacyAdapter) Execute() {
	_, _ = a.ExecuteResult(context.Background())
}

func (a *legacyAdapter) ExecuteContext(ctx context.Context) {
	_, _ = a.ExecuteResult(ctx)
}

// ExecuteResult preserves the handler outcome even if its subsequent audit
// fails. Compatibility entry points intentionally discard this result.
func (a *legacyAdapter) ExecuteResult(ctx context.Context) (model.Status, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return model.FAILED, err
	}
	profiling.SetCommandName(ctx, a.def.Canonical)
	handlerDone := profiling.Measure(ctx, "command.handler")
	status, err := a.def.New(a.engine, a.msg).Execute(ctx)
	handlerDone()
	auditDone := profiling.Measure(ctx, "command.audit")
	a.audit(ctx, status)
	auditDone()
	if err != nil {
		log.Printf("Saturn command %q failed with status %s: %v", a.def.Canonical, status, err)
	}
	return status, err
}

func (a *legacyAdapter) CanonicalName() string { return a.def.Canonical }

var userTripWhitelistedCommands = map[string]struct{}{
	"access": {}, "memory": {}, "prefix": {}, "register": {}, "remove": {},
	"replica": {}, "replicaoff": {}, "replicastatus": {}, "restart": {}, "shutdown": {},
	"coin": {}, "list": {}, "msgchannel": {}, "nicks": {}, "note": {}, "notes": {},
	"say": {}, "sub": {}, "unsub": {}, "weather": {},
	"dbzhelp": {}, "dbzregister": {}, "dbzstats": {}, "dbzstr": {}, "dfight": {}, "dspawn": {},
}

// AuthorizeWithEngine restores Saturn's constructor-level userTrips whitelist
// without turning it into a global role bypass.
func (a *legacyAdapter) AuthorizeWithEngine(engine common.Engine, user *model.User) bool {
	if user == nil {
		return false
	}
	if _, allowed := userTripWhitelistedCommands[a.def.Canonical]; allowed {
		if services := bundle(engine); services != nil && services.Security != nil && services.Security.IsConfiguredUserTrip(user.Trip) {
			return true
		}
	}
	return engine.IsUserAuthorized(user, &a.def.Role)
}

type commandAuditLogger interface {
	LogCommand(context.Context, model.CommandAuditRecord) (int64, error)
}

func (a *legacyAdapter) audit(ctx context.Context, status model.Status) {
	logger, ok := a.engine.(commandAuditLogger)
	if !ok || a.msg == nil {
		return
	}
	record := model.CommandAuditRecord{
		Trip:            a.msg.Trip,
		CommandName:     formatAuditList(a.def.Aliases),
		Arguments:       formatAuditList(args(a.msg)),
		Status:          string(status),
		CreatedOnMillis: time.Now().UnixMilli(),
		Channel:         a.engine.GetChannel(),
	}
	if _, err := logger.LogCommand(ctx, record); err != nil {
		log.Printf("audit Saturn command %q: %v", a.def.Canonical, err)
	}
}

func formatAuditList(values []string) string {
	return "[" + strings.Join(values, ", ") + "]"
}

func (a *legacyAdapter) GetRole() *model.Role { return &a.def.Role }
func (a *legacyAdapter) GetAliases() []string {
	return append([]string(nil), a.def.Aliases...)
}
func (a *legacyAdapter) NewInstance(e common.Engine, m *model.ChatMessage) common.Command {
	return &legacyAdapter{engine: e, def: a.def, msg: m}
}

// RegisterUserUtilities registers every Saturn-compatible command supported by
// the capabilities composed into the supplied engine.
func RegisterUserUtilities(e common.Engine) error {
	return RegisterUserUtilitiesWithDirectAgent(e, nil)
}

// RegisterUserUtilitiesWithDirectAgent registers the concrete utility commands
// and, when supplied, the composition-root direct l submitter.
func RegisterUserUtilitiesWithDirectAgent(e common.Engine, submitter DirectAgentSubmitter) error {
	if catalogError != nil {
		return fmt.Errorf("Saturn command catalog: %w", catalogError)
	}
	canonicals := []string{"help", "crashcourse", "say", "afk", "list", "ping", "version", "ape", "coin", "weather", "time", "info", "users", "nicks", "sub", "unsub", "memory", "dbzhelp", "msgchannel"}
	canonicals = append(canonicals, "captcha", "authorize", "deauthorize", "lock", "overflow", "ban", "kick", "unban", "unbanall", "mute", "unmute", "color", "flair", "nuke", "resurrect", "prefix")
	if _, ok := e.(common.AutoMoveController); ok {
		canonicals = append(canonicals, "automove")
	}
	canonicals = append(canonicals, "replica", "replicaoff", "replicastatus", "ws", "wsa", "restart", "shutdown", "sql", "lastonline", "mail", "note", "notes", "active", "shadowbanlist", "unshadowban", "shadowban", "remove", "register", "messages", "access")
	if b := bundle(e); b != nil && b.DBZ != nil {
		canonicals = append(canonicals, "dbzregister", "dbzstats", "dbzstr", "dfight", "dspawn")
	}
	registered := make(map[string]struct{}, len(canonicals))
	canonicals = append(canonicals, "vibe")
	for _, canonical := range canonicals {
		if !configuredCommandAvailable(e, canonical, manualInvocation) {
			continue
		}
		if _, duplicate := registered[canonical]; duplicate {
			continue
		}
		registered[canonical] = struct{}{}
		def, ok := commandDefinitionFor(canonical)
		if !ok {
			return fmt.Errorf("missing Saturn utility definition %q", canonical)
		}
		if canonical == "vibe" {
			vibeSubmitter, _ := submitter.(VibeSubmitter)
			def.New = func(engine common.Engine, message *model.ChatMessage) common.SaturnCommand {
				return &vibeCommand{commandBase: commandBase{engine: engine, message: message, role: def.Role, aliases: def.Aliases, canonical: def.Canonical}, submitter: vibeSubmitter}
			}
		}
		if err := e.RegisterCommand(&legacyAdapter{engine: e, def: def}); err != nil {
			return fmt.Errorf("register Saturn command %q: %w", canonical, err)
		}
	}
	if definition, ok := directLDefinition(submitter); ok {
		if err := e.RegisterCommand(&legacyAdapter{engine: e, def: definition}); err != nil {
			return fmt.Errorf("register direct l command: %w", err)
		}
	}
	return nil
}
