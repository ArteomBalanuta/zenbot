# Admin/Moderator S2 — Manual Simple Moderation Implementation

## Scope completed

Implemented S2 manual simple moderation only: captcha, auth/deauth, lock, overflow, ban/unban/unbanall. No S3/S4 command, semantic activation, configuration, plan, or Saturn-source changes were made.

## Delivered behavior

- Added concrete context-aware handlers in `internal/command/moderation_simple.go`.
  - `captcha`: no argument and literal `on` enable captcha; literal `off` disables it; otherwise `!captcha [on|off]` usage.
  - `authorize`/`auth` and `deauthorize`/`deauth`: issue typed `authtrip`/`deauthtrip` operations and retain Saturn source usage and acknowledgements.
  - `lock`/`lockroom`: literal `on|off` maps to typed `lockroom|unlockroom` operations and source replies.
  - `overflow` plus `shoot|love|hug|kiss`: normalizes the nick target, emits typed `overflow`, and has no success reply.
  - `ban`, `unban`, `unbanall`/`pardonall`: issue typed operations with source usage/output behavior.
- All S2 handlers use `common.ModerationOperations` rather than legacy `Engine` raw methods. They check dispatch cancellation before any reply/payload and propagate capability/operation errors without success output.
- Reconciled legacy `Ban`, `Unban`, `UnbanAll`, and `Lock` wrappers to delegate to the typed implementations.
- `RegisterUserUtilities` conditionally exposes the S2 aliases only when the engine implements `common.ModerationOperations`, using the context-aware Saturn adapter rather than generic fallback.
- `newCommand` now routes every S2 canonical to a concrete handler; removed S2 entries from the generic-fallback guard allowlist.

## Tests

- Added `internal/command/moderation_s2_test.go` covering exact payloads, replies, whisper forwarding, usage paths, cancellation, operation failures, aliases/roles, and conditional live registration.
- Extended test engine typed-operation support in `internal/command/handlers_test.go`.
- Retained the accepted identity persistence unit by targeting its direct legacy identity handler; public `auth` dispatch now intentionally follows the S2 raw Saturn command contract.

## Verification

Passed before subsequent concurrent S3/S4 worktree edits:

```text
gofmt -w [S2 files]
go test ./internal/command -run 'TestManualSimpleModeration|TestRegisterUserUtilitiesDispatchesModerationAliasesAndLeavesLegacyCommandsUnknown|TestBanUsesNormalizedNickAndExactConfirmation|TestUnbanPreservesHashAndUsageBoundary|TestUnbanAllAndLockBoundaries' -count=1
go test ./internal/core -count=1
```

`git diff --check` passed. A later full `go test ./... -count=1` compiled all packages but failed in `internal/command` because the concurrent S4 implementation made `shadowbanlist`, `shadowban`, and `unshadowban` concrete while its existing generic-fallback allowlist still claims they must remain generic. After that run, a concurrent S3 test-support edit introduced duplicate package declarations (`commandBase`, `args`, `reply`, `commandEngineStub`) in `internal/command/s3_command_support_test.go`, so the command package no longer compiles. No S3/S4 file was changed to resolve either blocker.

Full-suite status must be re-run after the concurrent S3/S4 work is reconciled.

## S2-owned files

- `internal/command/moderation_simple.go` (new)
- `internal/command/moderation_s2_test.go` (new)
- `internal/command/ban.go`
- `internal/command/unban.go`
- `internal/command/unbanAll.go`
- `internal/command/lock.go`
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/handlers_test.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `internal/command/identity_commands_test.go`
