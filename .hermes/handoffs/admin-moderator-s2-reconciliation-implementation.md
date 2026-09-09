# S2 manual simple moderation — reconciliation handoff

## Scope and result

S2 is reconciled within its manual simple moderation vertical. The existing S2 implementation dispatches concrete MODERATOR handlers through `common.ModerationOperations` and the typed core protocol operations; it does not use the generic acknowledged/no-op `saturnCommand` route.

This reconciliation repaired one registration-capability defect: the pre-existing identity-service registration condition could expose S2 `authorize`/`auth` even when an engine lacked `common.ModerationOperations`. `authorize` is now registered only by the typed moderation-capability condition, together with the rest of the S2 public command set.

No S3/S4 production or test file was modified, deleted, reverted, or mechanically reconciled. No provider, configuration, plan, agent dispatch, or Saturn source was edited. Nothing was committed, staged, reset, cleaned, or pushed.

## Source trace (read-only Saturn)

Definitions inspected under Saturn `develop`:

- `src/main/java/org/saturn/app/command/impl/moderator/CaptchaCommandImpl.java`
- `.../AuthorizeTripCommandImpl.java`
- `.../DeAuthorizeTripCommandImpl.java`
- `.../LockRoomUserCommandImpl.java`
- `.../OverflowCommandImpl.java`
- `.../BanUserCommandImpl.java`
- `.../UnBanUserCommandImpl.java`
- `.../UnBanAllUserCommandImpl.java`

Each class declares `Role.MODERATOR` and its source aliases: `captcha`; `authorize,auth`; `deauthorize,deauth`; `lock,lockroom`; `overflow,shoot,love,hug,kiss`; `ban`; `unban`; `unbanall,pardonall`.

Usage and protocol implementations inspected in:

- `src/main/java/org/saturn/app/service/ModService.java`
- `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java:84-121,150-151,280-282`

Observed raw payloads are `ban` with normalized `nick`, `unban` with raw `hash`, `unbanall`, `lockroom`, `unlockroom`, `enablecaptcha`, `disablecaptcha`, `authtrip` with `trip`, `deauthtrip` with `trip`, and `overflow` with normalized `nick`. Observed command outputs/whispers are preserved by the S2 handlers: captcha and lock success messages, auth/deauth acknowledgements and examples, overflow missing-target example with no success reply, ban/unban confirmations/examples, and `mercy.` for unbanall. The target additionally follows the approved architecture cancellation/error rule: canceled contexts and operation errors yield `FAILED` with no outbound payload/success reply.

## Current S2 behavior

- `captcha`: no argument or literal `on` calls `EnableCaptcha`; literal `off` calls `DisableCaptcha`; other input replies exactly `!captcha [on|off]` for the configured prefix.
- `authorize`/`auth` call `AuthorizeTrip` (`authtrip`); `deauthorize`/`deauth` call `DeauthorizeTrip` (`deauthtrip`).
- `lock`/`lockroom` accept literal `on|off` and call `LockRoom`/`UnlockRoom` (`lockroom`/`unlockroom`).
- `overflow`/`shoot`/`love`/`hug`/`kiss` normalize the target and call `OverflowNick`; successful overflow sends no chat reply.
- `ban` normalizes the target and calls `BanNick`; `unban` preserves the supplied hash and calls `UnbanHash`; `unbanall`/`pardonall` call `UnbanAllContext`.
- `RegisterUserUtilitiesWithDirectAgent` registers all S2 aliases only when the engine implements `common.ModerationOperations`. It no longer separately registers `authorize` based only on the identity persistence bundle.

## Paths

### Changed by this reconciliation

- `internal/command/dispatch_adapter.go`
  - Removed `authorize` from the identity-persistence capability branch. It remains in the `common.ModerationOperations` branch.
- `internal/command/moderation_s2_test.go`
  - Added a focused no-moderation-capability registration regression test.
- `.hermes/handoffs/admin-moderator-s2-reconciliation-implementation.md`
  - This handoff.

### Existing S2 vertical paths formatted/reviewed

- `internal/command/moderation_simple.go`
- `internal/command/moderation_s2_test.go`
- `internal/command/ban.go`
- `internal/command/unban.go`
- `internal/command/unbanAll.go`
- `internal/command/lock.go`
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/handlers_test.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `internal/command/identity_commands_test.go`

The typed S1 dependency paths reviewed as the concrete boundary were `internal/common/moderation_operations.go` and `internal/core/moderation_operations.go`; they were not changed by this reconciliation.

## Strict TDD evidence for the repaired behavior

### RED

Test added first:

```text
TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability
```

Command:

```text
go test ./internal/command -run '^TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability$' -count=1
```

Observed failure before production change:

```text
S2 alias "authorize" registered without common.ModerationOperations
S2 alias "auth" registered without common.ModerationOperations
FAIL zenbot/internal/command
```

### GREEN

Minimal production change: removed only `"authorize"` from the identity `GroupB + Security` registration append in `internal/command/dispatch_adapter.go`.

Command after the change:

```text
gofmt -w internal/command/moderation_s2_test.go internal/command/dispatch_adapter.go && go test ./internal/command -run '^TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability$' -count=1
```

Observed result:

```text
ok zenbot/internal/command 0.519s
```

## Verification executed

Formatting and whitespace:

```text
gofmt -w internal/command/moderation_simple.go internal/command/moderation_s2_test.go internal/command/ban.go internal/command/unban.go internal/command/unbanAll.go internal/command/lock.go internal/command/handlers.go internal/command/dispatch_adapter.go internal/command/handlers_test.go internal/command/admin_moderator_catalog_guard_test.go internal/command/identity_commands_test.go
git diff --check
```

Result: success; `git diff --check` emitted no output.

Focused S2 gate:

```text
go test ./internal/command -run '^(TestManualSimpleModerationCommandsUseTypedOperations|TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects|TestManualSimpleModerationOperationFailureHasNoSuccessReply|TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases|TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability|TestBanUsesNormalizedNickAndExactConfirmation|TestBanWithoutTargetIsSilent|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries)$' -count=1
```

Result: `ok zenbot/internal/command 0.603s`.

Typed core gate:

```text
go test ./internal/core -count=1
```

Result: `ok zenbot/internal/core 0.421s`.

## Remaining non-S2 blocker

The full command package and full repository suite compile, but both fail only at the pre-existing concurrent S4 catalog guard mismatch:

```text
TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded
ShadowBanList (shadowbanlist) generic fallback=false, allowed transitional fallback=true
ShadowBanUserCommandImpl (shadowban) generic fallback=false, allowed transitional fallback=true
UnShadowBanUserCommandImpl (unshadowban) generic fallback=false, allowed transitional fallback=true
```

Commands executed after reconciliation:

```text
go test ./internal/command -count=1
go test ./... -count=1
```

Both returned non-zero because of those three S4 assertions in `internal/command/admin_moderator_catalog_guard_test.go`. The full-suite run otherwise reported passing packages, including `internal/core`, `internal/repository/h2`, and `internal/service`. This S2 reconciliation intentionally did not alter the S4 guard or behavioral tests.
