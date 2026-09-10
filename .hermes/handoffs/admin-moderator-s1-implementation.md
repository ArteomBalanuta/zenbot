# Admin/Moderator S1 — Typed Raw Moderation Operations Implementation

## Scope completed

Implemented architecture slice S1 only. No public command registration, command behavior, configuration, Saturn source, plans/audits, staging, commit, or push was changed.

## Source evidence read

- Saturn: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/ModServiceImpl.java`
- Target: `internal/core/engine_impl.go`
- Architecture: `.hermes/handoffs/admin-moderator-full-migration-architecture.md`

## Delivered boundary

- Added `internal/common/moderation_operations.go` with a narrow `common.ModerationOperations` capability and distinct payload-field input types: `NickTarget`, `BanHash`, `Trip`, `Flair`, `Color`, and `Channel`.
- Added `internal/core/moderation_operations.go`, a closed, context-aware JSON payload encoder. It implements every observed `ModServiceImpl` raw operation:
  - `ban`, `unban`, `unbanall`
  - `lockroom`, `unlockroom`
  - `enablecaptcha`, `disablecaptcha`
  - `authtrip`, `deauthtrip`
  - `mute`, `unmute`
  - `forceflair`, `forcecolor`
  - `kick` and `kick` with `to`
  - `overflow`
- Nick-bearing operations use `util.NormalizeNickTarget`; hashes/trips/flair/color/channel values remain source-shaped values in their dedicated payload fields.
- JSON uses a closed struct plus `encoding/json`, yielding compact exact JSON and safe escaping; no generic map/raw-string payload construction was introduced in command or agent packages.
- Each operation checks cancellation before serialization/send and returns transport/queue errors without producing a fallback reply or payload. The existing QA-approved semantic reversal path remains intact.
- `EngineImpl.EnableCaptcha` now routes through the S1 exact JSON operation; its existing focused assertion was updated from whitespace-formatted JSON to the canonical exact JSON output.

## Tests added/updated

- Added `internal/core/moderation_operations_test.go`:
  - all 16 observed payload variants, including `kick` with/without `to`
  - nick normalization and blank-nick rejection
  - quote/newline JSON escaping for trip, flair, and hash fields
  - cancellation produces no outbound message for every operation
  - transport failure returns an error without queue fallback
  - compile-time proof that `EngineImpl` implements `common.ModerationOperations`
- Updated existing captcha expectation in `internal/core/moderation_test.go` to exact canonical JSON.

## Verification

Passed:

```text
go test ./internal/core -run 'TestRawModeration' -count=1
go test ./internal/core -count=1
go build ./cmd/zenbot
git diff --check
```

`go test ./internal/common ./internal/core ./internal/command ./internal/agent/... -count=1` and `go test ./... -count=1` were run. Both fail in pre-existing/out-of-scope worktree tests unrelated to S1:

- `internal/command`: `TestActivityCommandPreservesAliasesRoleFirstArgumentAndWhisper`; `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded`
- `internal/repository/h2`: `TestActivityStatsUsesSourceCaseInsensitiveTripQueryAndOrdersWeekHour`; `TestActivityStatsReturnsNoRowsForAbsentTrip` (EOF)

The command failures concern the uncommitted activity/catalog slice; the H2 failures concern its activity fixture/query slice. S1 core tests pass.

## Files owned by S1

- `internal/common/moderation_operations.go` (new)
- `internal/core/moderation_operations.go` (new)
- `internal/core/moderation_operations_test.go` (new)
- `internal/core/engine_impl.go` (only `EnableCaptcha` moved to the S1 boundary)
- `internal/core/moderation_test.go` (canonical JSON expectation)

## Follow-on

S2–S4 should type-assert/use `common.ModerationOperations` from concrete command implementations and preserve per-command parsing, authorization, output, and manual-target policy decisions. This slice intentionally did not activate those command paths or change automatic moderation policy.
