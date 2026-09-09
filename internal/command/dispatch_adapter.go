package command

import (
	"context"
	"fmt"
	"log"

	"zenbot/internal/common"
	"zenbot/internal/model"
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
	a.execute(context.Background())
}

func (a *legacyAdapter) ExecuteContext(ctx context.Context) {
	a.execute(ctx)
}

func (a *legacyAdapter) execute(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := a.def.New(a.engine, a.msg).Execute(ctx)
	if err != nil {
		log.Printf("Saturn command %q failed with status %s: %v", a.def.Canonical, status, err)
	}
}

func (a *legacyAdapter) GetRole() *model.Role { return &a.def.Role }
func (a *legacyAdapter) GetAliases() []string {
	return append([]string(nil), a.def.Aliases...)
}
func (a *legacyAdapter) NewInstance(e common.Engine, m *model.ChatMessage) common.Command {
	return &legacyAdapter{engine: e, def: a.def, msg: m}
}

// RegisterUserUtilities registers only the concrete Saturn utility commands.
// It intentionally does not expose the broader Saturn catalog while those
// commands remain placeholders in Zenbot.
func RegisterUserUtilities(e common.Engine) error {
	return RegisterUserUtilitiesWithDirectAgent(e, nil)
}

// RegisterUserUtilitiesWithDirectAgent registers the concrete utility commands
// and, when supplied, the composition-root direct l submitter.
func RegisterUserUtilitiesWithDirectAgent(e common.Engine, submitter DirectAgentSubmitter) error {
	// These three user commands have dedicated legacy-dispatch implementations;
	// register them explicitly rather than exposing catalog placeholders.
	e.RegisterCommand(&Say{})
	e.RegisterCommand(&Afk{})
	e.RegisterCommand(&List{})
	if _, ok := e.(common.ModerationOperations); ok {
		// Preserve legacy constructors for compatibility. The typed adapters below
		// intentionally overwrite these aliases on fully composed engines.
		e.RegisterCommand(&Ban{})
		e.RegisterCommand(&Unban{})
		e.RegisterCommand(&UnbanAll{})
		e.RegisterCommand(&Lock{})
	}

	canonicals := []string{"help", "crashcourse", "ping", "version", "ape", "coin", "weather", "time", "info", "users", "nicks", "sub", "unsub", "memory", "dbzhelp", "msgchannel"}
	if _, ok := e.(common.ModerationOperations); ok {
		canonicals = append(canonicals, "captcha", "authorize", "deauthorize", "lock", "overflow", "ban", "unban", "unbanall", "mute", "unmute", "color", "flair")
	}
	if _, ok := e.(common.RoomSnapshotSubmitter); ok {
		canonicals = append(canonicals, "nuke")
	}
	if _, mover := e.(common.LiveRoomMover); mover {
		if _, submitter := e.(common.RoomSnapshotSubmitter); submitter {
			canonicals = append(canonicals, "resurrect")
		}
	}
	if _, ok := e.(common.PrefixController); ok {
		canonicals = append(canonicals, "prefix")
	}
	if _, ok := e.(common.AutoMoveController); ok {
		canonicals = append(canonicals, "automove")
	}
	if _, ok := e.(ReplicaController); ok {
		canonicals = append(canonicals, "replica", "replicaoff", "replicastatus")
	}
	if _, ok := e.(common.SupportReplicaRelay); ok {
		canonicals = append(canonicals, "ws", "wsa")
	}
	if lifecycleController(e) != nil {
		canonicals = append(canonicals, "restart", "shutdown")
	}
	if b := bundle(e); b != nil && b.SQLCommand != nil {
		canonicals = append(canonicals, "sql")
	}

	if b := bundle(e); b != nil && b.Users != nil && (b.Users.Queries != nil || b.Users.LastSeen != nil) {
		canonicals = append(canonicals, "lastonline")
	}
	if b := bundle(e); b != nil && b.Activity != nil && b.Activity.Repo != nil {
		canonicals = append(canonicals, "active")
	}
	if b := bundle(e); b != nil && b.ShadowBans != nil && b.ShadowBans.Repo != nil {
		canonicals = append(canonicals, "shadowbanlist", "unshadowban")
		if _, ok := e.(common.ModerationOperations); ok {
			canonicals = append(canonicals, "shadowban")
		}
	}
	if b := bundle(e); b != nil && b.Users != nil && b.Users.GroupB != nil {
		canonicals = append(canonicals, "remove")
	}
	if b := bundle(e); b != nil && b.Users != nil && b.Users.GroupB != nil && b.Security != nil {
		canonicals = append(canonicals, "register", "messages")
		if b.Security.Authorization != nil {
			canonicals = append(canonicals, "access")
		}
	}
	if b := bundle(e); b != nil && b.DBZ != nil {
		canonicals = append(canonicals, "dbzregister", "dbzstats", "dbzstr", "dfight", "dspawn")
	}
	registered := make(map[string]struct{}, len(canonicals))
	for _, canonical := range canonicals {
		if _, duplicate := registered[canonical]; duplicate {
			continue
		}
		registered[canonical] = struct{}{}
		def, ok := commandDefinitionFor(canonical)
		if !ok {
			return fmt.Errorf("missing Saturn utility definition %q", canonical)
		}
		e.RegisterCommand(&legacyAdapter{engine: e, def: def})
	}
	if definition, ok := directLDefinition(submitter); ok {
		e.RegisterCommand(&legacyAdapter{engine: e, def: definition})
	}
	return nil
}
