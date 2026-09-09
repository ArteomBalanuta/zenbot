# S3 active-target manual moderation — independent QA

## Result

**PASS (focused S3 and retained S2 gates).** No verified S3 production or test defect required a correction. The current dirty/untracked S3 implementation is source-aligned for `mute`/`dumb`, `unmute`/`undumb`, `color`, and `flair`; this QA added only this handoff.

No commit, push, reset, clean, checkout, Saturn edit, S2 edit, S4 edit, catalog-guard edit, or unrelated dirty-work modification was performed.

## Source and static audit

Read Saturn read-only:

- `src/main/java/org/saturn/app/command/impl/moderator/MuteUserCommandImpl.java:17-59`: aliases `mute`, `dumb`; `MODERATOR`; normalizes first nick; finds current active same-nick user/hash; emits `mute` using nick; success is `<target> <hash> has been muted`.
- `.../UnMuteUserCommandImpl.java:16-47`: aliases `unmute`, `undumb`; `MODERATOR`; uses raw first argument as hash; example `unmute jJ4M4fsECSazzlj`; emits `unmute/hash`; success is `<hash> has been unmuted`.
- `.../ColorCommandImpl.java:18-65`: `MODERATOR`; requires target/value; normalizes target; requires active user; emits `forcecolor` with `nick` and `color`; no success reply.
- `.../FlairCommandImpl.java:18-66`: `MODERATOR`; same active/normalization behavior; emits `forceflair` with `nick` and `flair`; successful reply `\\n Flair set successfully!`.
- `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java:124-147`: exact wire payloads are `mute/nick`, `unmute/hash`, `forceflair/nick/flair`, `forcecolor/nick/color`; nick operations normalize via `IdentityUtil.normalizeNickTarget`.

Target audit:

- `internal/command/mute.go`: `activeModerationTarget` first calls source-equivalent `util.NormalizeNickTarget`, then authoritative `Engine.GetActiveUserByName`. It never enumerates the unlocked `GetActiveUsers` snapshot. `EngineImpl.GetActiveUserByName` takes `usersMu.RLock`, matches case-insensitively, and returns the current stored user/canonical name.
- Mute, color, and flair send the resolved current active `target.Name`; mute acknowledges with resolved `target.Name` plus current trusted `target.Hash`. An absent active target returns failure and sends no typed/raw moderation operation. No manual S3 command calls semantic-monitor protected-target policy.
- Unmute takes `arguments[0]` verbatim and passes it as `common.BanHash`; no nick normalization is applied to the raw hash.
- All command paths test `ctx.Err()` before usage/replies/protocol work. Typed-operation errors return `FAILED` without success reply. Core `sendRawModeration` also checks cancellation before JSON encoding/send.
- `internal/core/moderation_operations.go` uses closed `moderationPayload` structs and `encoding/json`; exact fields are `{"cmd":"mute","nick":...}`, `{"cmd":"unmute","hash":...}`, `{"cmd":"forcecolor","nick":...,"color":...}`, and `{"cmd":"forceflair","nick":...,"flair":...}`.
- `internal/command/handlers.go` has concrete `newCommand` cases for `mute`, `unmute`, `color`, and `flair`, never `saturnCommand`. Source catalog audit passed, including each source alias and `MODERATOR` role.
- `internal/command/dispatch_adapter.go` appends only S3 canonicals under `if _, ok := e.(common.ModerationOperations); ok`; source definitions expand them to all six aliases (`mute`,`dumb`,`unmute`,`undumb`,`color`,`flair`). The positive registration test verifies each alias is MODERATOR and concrete. Static inspection confirms the capability gate prevents all six registrations when unavailable.

## Exact verification commands and observed results

| Command | Result |
|---|---|
| `go test ./internal/command -run 'Test(Mute|Unmute|Color|Flair|ActiveTargetModeration|RegisterUserUtilities.*S3)' -count=1 -v` | PASS; `ok zenbot/internal/command 0.660s`. Covers aliases, canonical active nick/hash, absent target/no operation, raw unmute hash, color/flair behavior, authoritative current lookup, cancellation, typed-operation errors, registration/role/concrete handler. |
| `go test ./internal/command -run 'Test(Mute|Unmute|Color|Flair|ActiveTargetModeration|RegisterUserUtilities.*S3)' -count=1 -race -coverprofile=/tmp/zenbot-s3-command.cover` | PASS; `ok zenbot/internal/command 2.473s`; package coverage `9.1%`. S3 function coverage: `muteCommand.Execute 85.7%`, `unmuteCommand.Execute 92.9%`, `colorCommand.Execute 100.0%`, `flairCommand.Execute 100.0%`, `activeModerationTarget 83.3%`, `executeAppearance 77.3%`. |
| `go test ./internal/core -run '^(TestRawModerationOperationsEmitExactSaturnPayloads|TestRawModerationOperationsRejectBlankNickWithoutOutput|TestRawModerationOperationsSendNothingWhenCancelled|TestRawModerationOperationReturnsTransportErrorWithoutFallbackOutput)$' -count=1 -race -coverprofile=/tmp/zenbot-s3-core.cover` | PASS; `ok zenbot/internal/core 1.466s`; package coverage `7.9%`. S3 core functions `MuteNick`, `UnmuteHash`, `ForceFlair`, `ForceColor`, and `normalizeModerationNick` each `100.0%`; `sendRawModeration 75.0%`. Exact payload, escaping, blank-nick/no-output, cancellation/no-output, and transport error behavior passed. |
| `go test ./internal/command -run '^(TestManualSimpleModerationCommandsUseTypedOperations|TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects|TestManualSimpleModerationOperationFailureHasNoSuccessReply|TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases|TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability|TestBanUsesNormalizedNickAndExactConfirmation|TestBanWithoutTargetIsSilent|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries)$' -count=1 -v` | PASS; `ok zenbot/internal/command 0.623s`. Retained S2 gate is green. |
| `go test ./internal/command -run '^TestAdminModeratorCatalogMatchesSaturnSource$' -count=1 -v` | PASS; `ok zenbot/internal/command 0.313s`. |
| `go vet ./internal/command && git diff --check` | PASS (both exit status 0). |
| `go test ./internal/command -count=1` | Expected FAIL only at S4 stale catalog-guard rows listed below. |
| `go test ./... -count=1` | Expected FAIL only at the same S4 stale catalog-guard rows; all other reported packages passed, including `internal/core`, `internal/repository/h2`, and `internal/service`. |

## Changes

- Created this QA handoff only: `.hermes/handoffs/admin-moderator-s3-qa.md`.
- No S3 source or test correction was justified after audit and focused/race verification.

## Blockers intentionally not changed

1. Broad `internal/command` and `./...` runs fail solely on the known S4 stale `allowedScopedGenericFallbacks` expectations in `internal/command/admin_moderator_catalog_guard_test.go`:

```text
ShadowBanList (shadowbanlist) generic fallback=false, allowed transitional fallback=true
ShadowBanUserCommandImpl (shadowban) generic fallback=false, allowed transitional fallback=true
UnShadowBanUserCommandImpl (unshadowban) generic fallback=false, allowed transitional fallback=true
```

This is outside S3; no S4/catalog-guard behavior was changed.

2. `go vet ./internal/core` remains separately blocked by the pre-existing diagnostic:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

No core production edit was made for that unrelated issue.
