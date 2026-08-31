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
// carry a context or return an error, so inbound dispatch uses a background
// context and preserves errors through the existing log-based behavior.
type legacyAdapter struct {
	engine common.Engine
	def    common.CommandDefinition
	msg    *model.ChatMessage
}

func (a *legacyAdapter) Execute() {
	status, err := a.def.New(a.engine, a.msg).Execute(context.Background())
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
	// These three user commands have dedicated legacy-dispatch implementations;
	// register them explicitly rather than exposing catalog placeholders.
	e.RegisterCommand(&Say{})
	e.RegisterCommand(&Afk{})
	e.RegisterCommand(&List{})
	e.RegisterCommand(&Ban{})
	e.RegisterCommand(&Unban{})
	e.RegisterCommand(&UnbanAll{})
	e.RegisterCommand(&Lock{})

	canonicals := []string{"help", "crashcourse", "ping", "version", "ape", "coin", "weather", "time", "info", "users", "nicks", "sub", "unsub"}
	if _, ok := e.(ReplicaController); ok {
		canonicals = append(canonicals, "replica", "replicaoff", "replicastatus")
	}
	if b := bundle(e); b != nil && b.Users != nil && b.Users.GroupB != nil {
		canonicals = append(canonicals, "remove")
	}
	if b := bundle(e); b != nil && b.Users != nil && b.Users.GroupB != nil && b.Security != nil {
		canonicals = append(canonicals, "register", "authorize", "access", "messages")
	}
	if b := bundle(e); b != nil && b.DBZ != nil {
		canonicals = append(canonicals, "dbzregister", "dbzstats", "dbzstr", "dfight", "dbzhelp", "dspawn")
	}
	for _, canonical := range canonicals {
		def, ok := commandDefinitionFor(canonical)
		if !ok {
			return fmt.Errorf("missing Saturn utility definition %q", canonical)
		}
		e.RegisterCommand(&legacyAdapter{engine: e, def: def})
	}
	return nil
}
