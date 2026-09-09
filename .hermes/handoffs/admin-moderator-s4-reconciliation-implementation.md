# S4 public shadow-ban reconciliation — implementation

## Result

S4 is concrete and source-traced. `shadowbanlist`/`banlist`/`bannedusers`, `shadowban`/`sban`, and `unshadowban`/`shadowmercy`/`unblock` now pass focused command and real-H2 persistence coverage, use typed local persistence and the typed `KickNick` moderation operation, and have been removed from the catalog generic-fallback allowlist. No agent semantic-monitor route, raw generic command dispatch, `Ban` fallback, model/provider path, or Saturn source was changed.

## Saturn source mapping

- `src/main/java/org/saturn/app/command/impl/moderator/ShadowBanList.java:16-31`: MODERATOR aliases, persisted ban list invocation, successful list command.
- `.../ShadowBanUserCommandImpl.java:20-104`: MODERATOR aliases; missing-target example; literal `-c` active-user scan; source-normalized single target; active record + kick; offline name record; source replies.
- `.../UnShadowBanUserCommandImpl.java:16-56`: MODERATOR aliases; missing target; any `-all` argument invokes list/delete-all and retains source `FAILED` status; first target removes and replies.
- `src/main/java/org/saturn/app/service/impl/ModServiceImpl.java:43-80,155-205,208-264,266-282`: nullable tuple persistence with Base64 UTF-8 hash; Base64 decode on list; parameter-shaped name/trip/base64-hash deletion; source rendering/no-bans/all-delete output; exact `kick` raw operation.

## Target mapping and changes

- `internal/command/shadow_ban.go`
  - Retained concrete list/rendering, source-shaped single/offline/`-c`, and unshadow single/all handlers.
  - Repaired single-target identity selection to use `Engine.GetActiveUserByName` after source nick normalization rather than an unlocked snapshot scan.
  - Re-validates each `-c` snapshot selection through `GetActiveUserByName` before persisting/kicking, so persistence and kick use the authoritative current trip/name/hash.
  - Checks cancellation after persistence and before the typed `KickNick` call; operation/repository failures return `FAILED` without success replies.
- `internal/command/shadow_ban_test.go`
  - Added authoritative single identity, authoritative contains identity, post-persist cancellation/no-kick, and capability-gate regressions. Existing coverage exercises source formatting/no-bans, active/offline/contains behavior, unshadow target/all behavior, repository failure, and initial cancellation.
- `internal/command/dispatch_adapter.go`
  - `shadowbanlist` and `unshadowban` remain storage-capability gated; `shadowban` additionally requires `common.ModerationOperations`, guaranteeing the concrete typed kick operation is present before its aliases are registered.
- `internal/command/admin_moderator_catalog_guard_test.go`
  - Removed only the three validated S4 canonicals from `allowedScopedGenericFallbacks`.
- Validated without changing `internal/repository/repository.go`, `internal/repository/h2/shadow_ban.go`, or `internal/service/shadow_ban.go`: their typed command repository/service implementation performs parameterized H2 persistence/list/delete-all and Base64 round trips; source-compatible removal binds name, trip, and Base64 hash.

## Strict TDD evidence

| RED test/command | Expected observed failure | Minimal GREEN change | GREEN evidence |
|---|---|---|---|
| `TestShadowBanSingleUsesAuthoritativeActiveIdentity` | Persisted offline `{Name:Merc}` instead of current `{Trip:current-trip, Name:merc, Hash:current-hash}` | `shadowBanSingle` now uses `GetActiveUserByName` | Same test PASS |
| `TestShadowBanContainsUsesCurrentIdentityForSelectedActiveUser` | Persisted stale trip/hash from scan snapshot | Re-fetch selected user by name before persist/kick | Same test PASS |
| `TestRegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick` | `shadowban` and `sban` registered with storage only | Gate `shadowban` registration on `common.ModerationOperations`; keep list/unshadow storage-only | Same test PASS |
| `TestShadowBanCancellationAfterPersistDoesNotKickOrReply` | Canceled context still emitted typed kick (`kicked=[merc]`) | Check `ctx.Err()` after persistence and before kick | Same test PASS |

The pre-change focused guard also failed exactly on the three stale S4 generic-fallback entries. It was changed only after command and real-H2 behavior was green.

## Verification

- `go test ./internal/command -run 'Test(ShadowBan|UnshadowBan)|TestRegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick|TestAdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource)' -count=1 -v` — PASS.
- `go test ./internal/repository/h2 -run 'Test(PersistShadowBan|RemoveShadowBan|ShadowBan)' -count=1 -v` — PASS; uses real H2 and covers raw-hash Base64 storage/decode, nullable offline record, parameterized source target deletion, injection-shaped input, and delete-all.
- `gofmt -w internal/command/shadow_ban.go internal/command/shadow_ban_test.go internal/command/dispatch_adapter.go internal/command/admin_moderator_catalog_guard_test.go` — completed.
- `go test ./internal/command -count=1` — PASS.
- `go test ./... -count=1` — PASS (including `internal/repository/h2`).
- `go vet ./internal/command && git diff --check` — PASS.

## Remaining blockers

None for S4. `go vet ./internal/core` remains independently non-zero at the pre-existing `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`; it was not changed because it is outside S4 and `go test ./... -count=1` is green. The worktree remains intentionally dirty with unrelated S2/S3/S5 and agent-path changes; none were reset, cleaned, committed, or modified for S4.
