package command

import (
	"reflect"
	"testing"
	"zenbot/internal/common"
	"zenbot/internal/model"
)

type saturnCatalogRow struct {
	class     string
	canonical string
	role      model.Role
	aliases   []string
}

// saturnAdminModeratorCatalog is transcribed from the source annotations in
// /Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl.
// Keep this table source-shaped: class identity, annotation order, role, and
// aliases are all part of the guard rather than inferred from target code.
var saturnAdminModeratorCatalog = []saturnCatalogRow{
	{"AccessUserCommandImpl", "access", model.ADMIN, []string{"grant", "access"}},
	{"MemoryCommandImpl", "memory", model.ADMIN, []string{"mem", "memory", "memstats"}},
	{"MineTripCommandImpl", "mine", model.ADMIN, []string{"mine"}},
	{"PrefixCommandImpl", "prefix", model.ADMIN, []string{"prefix"}},
	{"ReplicaCommandImpl", "replica", model.ADMIN, []string{"replica", "bot", "agent"}},
	{"ReplicaOffCommandImpl", "replicaoff", model.ADMIN, []string{"replicaoff", "offline", "botoff", "agentoff"}},
	{"ReplicaStatusCommandImpl", "replicastatus", model.ADMIN, []string{"replicastatus", "status"}},
	{"RestartCommandImpl", "restart", model.ADMIN, []string{"restart", "reload", "re"}},
	{"ShutdownCommandImpl", "shutdown", model.ADMIN, []string{"exit", "quit", "shutdown"}},
	{"SqlUserCommandImpl", "sql", model.ADMIN, []string{"sql"}},
	{"WhiskeyReplicaCommandImpl", "whiskey", model.ADMIN, []string{"whiskey"}},

	{"ActivityCommandImpl", "active", model.MODERATOR, []string{"active", "activity"}},
	{"AuthorizeTripCommandImpl", "authorize", model.MODERATOR, []string{"authorize", "auth"}},
	{"AutoMoveUserCommandImpl", "automove", model.MODERATOR, []string{"automove"}},
	{"BanUserCommandImpl", "ban", model.MODERATOR, []string{"ban"}},
	{"CaptchaCommandImpl", "captcha", model.MODERATOR, []string{"captcha"}},
	{"ColorCommandImpl", "color", model.MODERATOR, []string{"color"}},
	{"DeAuthorizeTripCommandImpl", "deauthorize", model.MODERATOR, []string{"deauthorize", "deauth"}},
	{"FlairCommandImpl", "flair", model.MODERATOR, []string{"flair"}},
	{"KickUserCommandImpl", "kick", model.MODERATOR, []string{"kick", "k", "out"}},
	{"LastMessagesCommandImpl", "messages", model.MODERATOR, []string{"messages", "lastmessages"}},
	{"LockRoomUserCommandImpl", "lock", model.MODERATOR, []string{"lock", "lockroom"}},
	{"MuteUserCommandImpl", "mute", model.MODERATOR, []string{"mute", "dumb"}},
	{"NukeCommandImpl", "nuke", model.MODERATOR, []string{"nuke"}},
	{"OverflowCommandImpl", "overflow", model.MODERATOR, []string{"overflow", "shoot", "love", "hug", "kiss"}},
	{"RegisterUserCommandImpl", "register", model.MODERATOR, []string{"reg", "register"}},
	{"RemoveUserCommandImpl", "remove", model.MODERATOR, []string{"del", "delete", "remove"}},
	{"ResurrectUserCommandImpl", "resurrect", model.MODERATOR, []string{"move", "recover", "heal", "resurrect"}},
	{"ShadowBanList", "shadowbanlist", model.MODERATOR, []string{"shadowbanlist", "banlist", "bannedusers"}},
	{"ShadowBanUserCommandImpl", "shadowban", model.MODERATOR, []string{"shadowban", "sban"}},
	{"UnBanAllUserCommandImpl", "unbanall", model.MODERATOR, []string{"unbanall", "pardonall"}},
	{"UnBanUserCommandImpl", "unban", model.MODERATOR, []string{"unban"}},
	{"UnMuteUserCommandImpl", "unmute", model.MODERATOR, []string{"unmute", "undumb"}},
	{"UnShadowBanUserCommandImpl", "unshadowban", model.MODERATOR, []string{"unshadowban", "shadowmercy", "unblock"}},
}

func TestAdminModeratorCatalogMatchesSaturnSource(t *testing.T) {
	registry := common.NewSaturnCommandRegistry()
	if err := RegisterAll(registry); err != nil {
		t.Fatal(err)
	}

	definitions := make(map[string]common.CommandDefinition)
	var admin, moderator int
	for _, definition := range registry.Definitions() {
		switch definition.Role {
		case model.ADMIN:
			admin++
		case model.MODERATOR:
			moderator++
		default:
			continue
		}
		if _, exists := definitions[definition.Canonical]; exists {
			t.Fatalf("duplicate scoped canonical %q", definition.Canonical)
		}
		definitions[definition.Canonical] = definition
	}
	if admin != 11 || moderator != 23 || len(definitions) != 34 {
		t.Fatalf("scoped catalog counts admin=%d moderator=%d total=%d, want 11/23/34", admin, moderator, len(definitions))
	}
	if len(saturnAdminModeratorCatalog) != 34 {
		t.Fatalf("source inventory has %d rows, want 34", len(saturnAdminModeratorCatalog))
	}

	sourceCanonicals := make(map[string]struct{}, len(saturnAdminModeratorCatalog))
	for _, source := range saturnAdminModeratorCatalog {
		if _, duplicate := sourceCanonicals[source.canonical]; duplicate {
			t.Errorf("source inventory repeats canonical %q", source.canonical)
		}
		sourceCanonicals[source.canonical] = struct{}{}
		target, ok := definitions[source.canonical]
		if !ok {
			t.Errorf("%s (%s) is absent from RegisterAll", source.class, source.canonical)
			continue
		}
		if target.Role != source.role {
			t.Errorf("%s (%s) role=%v, want source role %v", source.class, source.canonical, target.Role, source.role)
		}
		if !reflect.DeepEqual(target.Aliases, source.aliases) {
			t.Errorf("%s (%s) aliases=%q, want source aliases %q", source.class, source.canonical, target.Aliases, source.aliases)
		}
	}
	for canonical := range definitions {
		if _, found := sourceCanonicals[canonical]; !found {
			t.Errorf("RegisterAll has unexpected scoped canonical %q", canonical)
		}
	}
}

// These are the explicitly known, not-yet-concrete S1-S9 migration rows. The
// set is intentionally exact: adding any new scoped generic route fails this
// test, and removing a row requires replacing it with a concrete handler plus
// its own behavior test. Do not treat this list as parity approval.
var allowedScopedGenericFallbacks = map[string]struct{}{
	"mine": {}, "whiskey": {},
}

func TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded(t *testing.T) {
	scoped := make(map[string]struct{}, len(saturnAdminModeratorCatalog))
	for _, source := range saturnAdminModeratorCatalog {
		scoped[source.canonical] = struct{}{}
		command := newCommand(source.canonical, source.aliases, source.role, &commandEngineStub{}, &model.ChatMessage{})
		_, generic := command.(*saturnCommand)
		_, allowed := allowedScopedGenericFallbacks[source.canonical]
		if generic != allowed {
			t.Errorf("%s (%s) generic fallback=%t, allowed transitional fallback=%t", source.class, source.canonical, generic, allowed)
		}
	}
	for canonical := range allowedScopedGenericFallbacks {
		if _, found := scoped[canonical]; !found {
			t.Errorf("generic fallback allowlist contains unscoped canonical %q", canonical)
		}
	}
}
