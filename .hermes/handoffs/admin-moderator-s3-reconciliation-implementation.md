# S3 active-target moderator-command reconciliation — implementation handoff

## Result

S3 is complete for `mute`/`dumb`, `unmute`/`undumb`, `color`, and `flair`.

The cancelled/untracked implementation already supplied the typed `common.ModerationOperations` boundary and concrete `newCommand` cases. This reconciliation retained that valid foundation, repaired two parity defects, and completed live registration:

1. Active lookups now use `Engine.GetActiveUserByName` after source-equivalent nick normalization, so the current engine-owned lookup determines presence/canonical name rather than an unlocked copied map.
2. Mute/color/flair protocol calls and mute acknowledgement now use the resolved active user's canonical `Name`, not the caller's arbitrary casing.
3. `RegisterUserUtilitiesWithDirectAgent` now exposes the four concrete S3 canonicals under the existing `common.ModerationOperations` capability gate; all six aliases are MODERATOR and resolve through concrete handlers, never `saturnCommand`.

No automated semantic-monitor protected-user policy was applied to these manual commands. No Saturn file, S2 behavior, S4 feature behavior, stale catalog guard, agent/config/plan/Saturn content, commits, pushes, resets, cleans, or checkouts were changed.

## Exact target artifacts

### Modified

- `internal/command/mute.go` (pre-existing untracked/cancelled-worker artifact)
  - `activeModerationTarget`: normalizes via `util.NormalizeNickTarget`, then calls authoritative `GetActiveUserByName`.
  - `muteCommand.Execute`: requires active target, calls `MuteNick(ctx, NickTarget(target.Name))`, and replies `<canonical-name> <trusted-hash> has been muted` only after success.
  - `unmuteCommand.Execute`: preserves the raw first hash, calls `UnmuteHash(ctx, BanHash(hash))`, and preserves source example/success output.
  - `colorCommand`/`flairCommand` and `executeAppearance`: require active target; call `ForceColor`/`ForceFlair` with canonical resolved target names; preserve source absent/missing/flair-success output shapes.
  - Every command checks cancellation before usage/payload/reply and returns operation errors before any success reply.
- `internal/command/dispatch_adapter.go`
  - Under the pre-existing `common.ModerationOperations` gate, appended `mute`, `unmute`, `color`, and `flair` to concrete legacy-adapter registration.
- `internal/command/mute_color_flair_test.go` (pre-existing untracked/cancelled-worker artifact)
  - Corrected the active-case expectation to canonical active nick.
  - Added authoritative-lookup regression, registration/role/non-generic regression, cancellation/no-side-effect coverage, and all-S3-operation-error/no-success-reply coverage.

### Retained, inspected, and verified (not modified in this reconciliation)

- `internal/common/moderation_operations.go`: context-aware typed S3 methods `MuteNick`, `UnmuteHash`, `ForceFlair`, `ForceColor`.
- `internal/core/moderation_operations.go`: closed JSON payloads with exact `mute`, `unmute`, `forcecolor`, `forceflair` command shapes and `sendRawModeration(ctx, ...)` cancellation behavior.
- `internal/core/engine_impl.go`: `GetActiveUserByName` locks `usersMu`, matches case-insensitively, and returns the current stored canonical user.
- `internal/command/handlers.go`: concrete `newCommand` cases for `mute`, `unmute`, `color`, and `flair` were already present.

## Source traces

Read directly from Saturn (read-only; no changes):

- `src/main/java/org/saturn/app/command/impl/moderator/MuteUserCommandImpl.java:17-59`
  - aliases `mute`, `dumb`; MODERATOR; `normalizedNickArgument(0, "mute merc")`; current-channel same-nick lookup for hash; `modService.mute(target)`; `<target> <hash> has been muted`.
- `.../UnMuteUserCommandImpl.java:16-47`
  - aliases `unmute`, `undumb`; MODERATOR; first raw argument as hash; source example `unmute jJ4M4fsECSazzlj`; `modService.unmute(hash)`; `<hash> has been unmuted`.
- `.../ColorCommandImpl.java:18-65`
  - MODERATOR; at least two arguments; normalized target; active same-nick requirement; `modService.forceColor(target, color)`; no success reply.
- `.../FlairCommandImpl.java:18-66`
  - MODERATOR; at least two arguments; normalized target; active same-nick requirement; `modService.forceFlair(target, flair)`; `\\n Flair set successfully!` success reply.
- `src/main/java/org/saturn/app/service/ModService.java:41-47` and `.../service/impl/ModServiceImpl.java:124-147`
  - raw wire payload names/fields: `mute/nick`, `unmute/hash`, `forceflair/nick/flair`, `forcecolor/nick/color`; nick operations normalize via `IdentityUtil.normalizeNickTarget`.
- Saturn source tests read: `MuteUserCommandImplTest.java`, `UnMuteUserCommandImplTest.java`, `ColorCommandImplTest.java`, `FlairCommandImplTest.java`.
- Target active-user authority trace: `internal/core/engine_impl.go:363-388` replaces active snapshot under lock and `:479-488` resolves current user under read lock.
- Approved architecture read: `.hermes/handoffs/admin-moderator-full-migration-architecture.md:47-63,97-103,109-121`; S3 is explicitly the active-target slice and keeps manual commands separate from automatic protected-user policy.

## TDD evidence

### RED 1 — canonical active target

Test expectation was changed first from caller casing `mErC` to authoritative active canonical `Merc` for mute/color/flair.

```text
go test ./internal/command -run '^(TestMuteRequiresActiveCaseInsensitiveTargetAndIncludesResolvedHash|TestColorAndFlairRequireActiveCaseInsensitiveTarget)$' -count=1 -v
FAIL: mute expected mute:Merc / Merc hash-a, got mute:mErC / mErC hash-a
FAIL: color expected color:Merc:00ff00, got color:mErC:00ff00
FAIL: flair expected flair:Merc:trusted, got flair:mErC:trusted
```

Minimal GREEN: use `target.Name` for S3 nick operations and mute reply.

```text
PASS: the same focused command test; ok zenbot/internal/command 0.421s
```

### RED 2 — current authoritative lookup

A new test gave the active engine a current canonical user while its stale enumerable map was empty.

```text
go test ./internal/command -run '^TestActiveTargetModerationUsesCurrentAuthoritativeLookup$' -count=1 -v
FAIL: status=FAILED; chats=[mod|mErC is not in the room|false]; operations=[]
```

Minimal GREEN: replace direct `GetActiveUsers` iteration in `activeModerationTarget` with `GetActiveUserByName(normalizedNick)`.

```text
PASS: TestActiveTargetModerationUsesCurrentAuthoritativeLookup; ok zenbot/internal/command 0.563s
```

### RED 3 — active registration

A new registration test asserted all S3 aliases are installed as MODERATOR concrete commands.

```text
go test ./internal/command -run '^TestRegisterUserUtilitiesRegistersConcreteS3ModeratorAliases$' -count=1 -v
FAIL: aliases color, flair, mute, dumb, unmute, undumb were not registered
```

Minimal GREEN: append four S3 canonicals to the existing moderation-capability registration list.

```text
PASS: TestRegisterUserUtilitiesRegistersConcreteS3ModeratorAliases; ok zenbot/internal/command 0.567s
```

Cancellation and per-operation-error regression tests were then added to verify retained S3 behavior; they passed without further production changes.

## Verification

| Command | Observed result |
|---|---|
| Focused S3 command tests (all behavior, active canonical target, registration, cancellation, operation failures) | PASS; `ok zenbot/internal/command 0.660s` |
| Same focused S3 set with `-race` | PASS; `ok zenbot/internal/command 2.521s` |
| `go test ./internal/core -run '^(TestRawModerationOperationsEmitExactSaturnPayloads|TestRawModerationOperationsRejectBlankNickWithoutOutput|TestRawModerationOperationsSendNothingWhenCancelled|TestRawModerationOperationReturnsTransportErrorWithoutFallbackOutput)$' -count=1 -v` | PASS; `ok zenbot/internal/core 0.357s`; includes all S3 exact wire payloads and cancellation/no-output cases |
| Existing S2 focused QA command plus `TestAdminModeratorCatalogMatchesSaturnSource` | PASS; `ok zenbot/internal/command 0.755s` and `0.324s` respectively |
| `go vet ./internal/command` | PASS |
| `git diff --check` | PASS |

## External blockers (intentionally untouched)

1. `go test ./internal/command -count=1` and `go test ./... -count=1` fail solely on known S4 stale catalog expectations in `internal/command/admin_moderator_catalog_guard_test.go`:

```text
ShadowBanList (shadowbanlist) generic fallback=false, allowed transitional fallback=true
ShadowBanUserCommandImpl (shadowban) generic fallback=false, allowed transitional fallback=true
UnShadowBanUserCommandImpl (unshadowban) generic fallback=false, allowed transitional fallback=true
```

This is the provided outside-scope S4 blocker; no catalog guard/S4 behavior was changed.

2. `go vet ./internal/core` remains blocked by the pre-existing diagnostic:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

No core production edit was made for it.
