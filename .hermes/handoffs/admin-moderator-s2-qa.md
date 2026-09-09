# S2 manual simple moderation — independent QA

## Result

**PASS (focused S2 gate).** The reconciled S2 vertical is concrete, capability-gated, context-aware at the `SaturnCommand`/`common.ModerationOperations` boundary, and matches the inspected Saturn source contracts. No S2 production defect was found. I added escaping regression coverage for the two S2 raw value shapes that were previously only exercised with non-escaping values.

No commit, push, reset, clean, checkout, Saturn edit, config change, plan edit, or agent-path edit was performed. Existing dirty/untracked work was preserved.

## Exact verification commands and observed status

| Command | Status / observed output |
|---|---|
| `go test ./internal/command -run '^(TestManualSimpleModerationCommandsUseTypedOperations|TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects|TestManualSimpleModerationOperationFailureHasNoSuccessReply|TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases|TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability|TestBanUsesNormalizedNickAndExactConfirmation|TestBanWithoutTargetIsSilent|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries)$' -count=1 -v` | PASS: `ok zenbot/internal/command 0.681s` |
| `go test ./internal/core -run 'Test.*Moderation' -count=1 -v` | PASS: `ok zenbot/internal/core 0.249s` |
| `gofmt -w internal/core/moderation_operations_test.go && go test ./internal/command -run '^(TestManualSimpleModerationCommandsUseTypedOperations|TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects|TestManualSimpleModerationOperationFailureHasNoSuccessReply|TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases|TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability|TestBanUsesNormalizedNickAndExactConfirmation|TestBanWithoutTargetIsSilent|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries|TestRegisterUserUtilitiesDispatchesModerationAliasesAndLeavesLegacyCommandsUnknown)$' -count=1 && go test ./internal/core -run '^(TestRawModerationOperationsEmitExactSaturnPayloads|TestRawModerationOperationsRejectBlankNickWithoutOutput|TestRawModerationOperationsSendNothingWhenCancelled|TestRawModerationOperationReturnsTransportErrorWithoutFallbackOutput)$' -count=1` | PASS: `ok zenbot/internal/command 0.919s`; `ok zenbot/internal/core 0.264s` |
| `go test ./internal/command -run '^(TestManualSimpleModerationCommandsUseTypedOperations|TestManualSimpleModerationUsageErrorsAndCancellationHaveNoSideEffects|TestManualSimpleModerationOperationFailureHasNoSuccessReply|TestRegisterUserUtilitiesRegistersAllManualSimpleModerationAliases|TestRegisterUserUtilitiesDoesNotExposeS2AuthorizeWithoutModerationCapability|TestBanUsesNormalizedNickAndExactConfirmation|TestBanWithoutTargetIsSilent|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries|TestRegisterUserUtilitiesDispatchesModerationAliasesAndLeavesLegacyCommandsUnknown)$' -count=1 -race -coverprofile=/tmp/zenbot-s2-command.cover` | PASS: `ok zenbot/internal/command 4.555s`, `coverage: 13.3% of statements` package-wide |
| `go test ./internal/core -run '^(TestRawModerationOperationsEmitExactSaturnPayloads|TestRawModerationOperationsRejectBlankNickWithoutOutput|TestRawModerationOperationsSendNothingWhenCancelled|TestRawModerationOperationReturnsTransportErrorWithoutFallbackOutput)$' -count=1 -coverprofile=/tmp/zenbot-s2-core.cover` | PASS: `ok zenbot/internal/core 0.370s`, `coverage: 7.9% of statements` package-wide |
| `go tool cover -func=/tmp/zenbot-s2-command.cover` | S2 handler coverage: `firstModerationArgument 100.0%`; captcha `88.9%`; auth/deauth `86.4%`; overflow `73.3%`; ban `76.5%`; unban `76.9%`; unbanall `66.7%`; lock `84.2%`. |
| `go tool cover -func=/tmp/zenbot-s2-core.cover` | `moderation_operations.go`: S2 operations `BanNick`, `UnbanHash`, `UnbanAllContext`, `LockRoom`, `UnlockRoom`, `EnableCaptcha`, `DisableCaptcha`, `AuthorizeTrip`, `DeauthorizeTrip`, `OverflowNick`, and `normalizeModerationNick` each `100.0%`; `sendRawModeration 75.0%`; `sendOutboundContext 71.4%`. |
| `go test ./internal/command -run '^TestAdminModeratorCatalogMatchesSaturnSource$' -count=1 -v` | PASS: `ok zenbot/internal/command 0.319s`. Source-role/alias inventory remains 11 admin + 23 moderator / 34 total. |
| `go vet ./internal/command && git diff --check` | PASS: `PASS: go vet ./internal/command; git diff --check` |
| `go vet ./internal/core` | BLOCKED outside S2: existing vet diagnostic `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`. No modification made. |
| `go test ./internal/command -count=1` | Expected non-zero; only `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded` failed for S4 rows detailed below. |
| `go test ./... -count=1` | Expected non-zero; same sole S4 catalog-guard failure. All other listed packages passed, including `internal/core`, `internal/repository/h2`, and `internal/service`. |

## Static review and source parity

Read Saturn's eight required moderator classes and `ModServiceImpl` directly. All eight are `MODERATOR` and their aliases match target catalog/registration:

- `captcha`: no argument/`on` -> `enablecaptcha`; `off` -> `disablecaptcha`; otherwise `!captcha [on|off]`.
- `authorize`/`auth` -> `authtrip`; `deauthorize`/`deauth` -> `deauthtrip`.
- `lock`/`lockroom`: literal `on` -> `lockroom`, literal `off` -> `unlockroom`; source-shaped usage/replies.
- `overflow`/`shoot`/`love`/`hug`/`kiss`: normalized nick -> `overflow`; successful invocation has no acknowledgement.
- `ban`: normalized nick -> `ban` and confirmation; missing target has the established usage path.
- `unban`: raw first hash -> `unban`, preserving it rather than nick-normalizing; missing/blank gets example usage.
- `unbanall`/`pardonall`: `unbanall` and `mercy.`.

`moderation_simple.go` checks cancellation before any usage, payload, or reply and returns operation errors before success acknowledgements. `common.ModerationOperations` is typed and context-aware. `core/moderation_operations.go` uses a closed struct plus `encoding/json.Marshal`, not string formatting/maps; each raw operation passes the dispatch context through `sendRawModeration` and `sendOutboundContext`. Exact payload tests cover all S2 protocol commands; new S2 cases assert escaping of a normalized ban nick containing quote/backslash and a raw unban hash containing quote/newline.

The active registration path in `dispatch_adapter.go` adds every S2 canonical only when the engine implements `common.ModerationOperations`; the no-capability regression verifies none of the 16 S2 aliases is registered. S2 canonicals resolve in `newCommand` to concrete handlers, not `saturnCommand`. Whisper/source acknowledgement routing uses the common `reply` helper and inbound whisper fields.

Caveat assessed: `legacyAdapter` necessarily invokes `context.Background()` because the legacy `common.Command.Execute()` API carries no context; the concrete S2 `SaturnCommand.Execute(ctx)` handlers and the typed operations themselves are context-aware and are where cancellation/no-side-effect guarantees are tested.

## QA change

- `internal/core/moderation_operations_test.go` (already untracked S1/S2 boundary test file): added two exact payload regression cases:
  - `ban JSON-escapes normalized nick`
  - `unban JSON-escapes raw hash`

No production behavior or S3/S4 test/behavior was changed.

## External blocker — do not fix in S2

Both broad commands fail only because cancelled partial S4 made these commands concrete while `allowedScopedGenericFallbacks` in `internal/command/admin_moderator_catalog_guard_test.go` still expects them to be generic:

```text
ShadowBanList (shadowbanlist) generic fallback=false, allowed transitional fallback=true
ShadowBanUserCommandImpl (shadowban) generic fallback=false, allowed transitional fallback=true
UnShadowBanUserCommandImpl (unshadowban) generic fallback=false, allowed transitional fallback=true
```

That stale S4 expectation is correctly isolated and was not modified.
