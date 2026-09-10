# S4 public shadow-ban QA audit

## Scope and result

Independent tester audit of the dirty Zenbot worktree, limited to S4 public shadow-ban command parity. **PASS: no verified S4 defect required a production or test change.** No S2/S3 behavior, agent/config/plans/Saturn source, unrelated files, staging, reset/clean/checkout, commit, or push was changed. This handoff is the only file created by this QA pass.

## Authoritative Saturn source trace (read-only)

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ShadowBanList.java:16-31`
  - `MODERATOR`; aliases `shadowbanlist`, `banlist`, `bannedusers`; list operation and `SUCCESSFUL` status.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ShadowBanUserCommandImpl.java:20-104`
  - `MODERATOR`; aliases `shadowban`, `sban`; missing-target example; literal `arguments.size() > 1 && arguments.contains("-c")` with `arguments.get(1)` contains pattern; active trip/name/hash persistence followed by kick; offline name-only record; source replies.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/UnShadowBanUserCommandImpl.java:16-56`
  - `MODERATOR`; aliases `unshadowban`, `shadowmercy`, `unblock`; missing-target example; any `-all` invokes list/delete-all and returns `FAILED`; first target removes by source target and replies, returning `SUCCESSFUL`.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/ModServiceImpl.java:43-80,155-205,208-264,266-282`
  - nullable `banned_users` tuple persistence; UTF-8 Base64 hash store/decode; parameter-shaped deletion by `name OR trip OR base64(target)`; list/no-ban and all-delete rendering; raw `kick` protocol operation.

## Zenbot source trace and behavioral verdict

- `internal/command/registry.go:133` registers the exact S4 aliases at `MODERATOR`; `internal/command/admin_moderator_catalog_guard_test.go:51-56` source-shaped catalog guard asserts them.
- `internal/command/handlers.go:345-350` resolves S4 canonicals to concrete `shadowBanListCommand`, `shadowBanCommand`, and `unshadowBanCommand`, not `saturnCommand` generic dispatch.
- `internal/command/admin_moderator_catalog_guard_test.go:117-121` has no S4 canonical in `allowedScopedGenericFallbacks`; `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded` passes.
- `internal/command/dispatch_adapter.go:65-70` gates list/unshadow registration on a usable typed shadow-ban repository and additionally gates `shadowban` on `common.ModerationOperations`; `common.ModerationOperations` includes typed `KickNick` (`internal/common/moderation_operations.go:14-32`). This prevents public active-target aliases from registering without typed kick capability.
- `internal/command/shadow_ban.go:29-50` lists local typed records, produces source no-ban/output shape, and receives decoded hashes from service/repository.
- `internal/command/shadow_ban.go:54-129` implements missing-target usage, literal `-c`, active current-user re-resolution before each persist/kick, name-only offline records, typed persistence before typed kick, and cancellation checks before side effects/replies. Repository, cancellation, or kick-operation errors return `FAILED` with no success reply.
- `internal/command/shadow_ban.go:134-174` implements target removal and source `-all` list/delete-all semantics, including the source `FAILED` status for all mode.
- `internal/service/shadow_ban.go:10-50` is a narrow typed local persistence boundary with no agent path.
- `internal/repository/h2/shadow_ban.go:19-108` binds all SQL parameters, writes raw hash as Base64, decodes it during list, removes matching name/trip/Base64 hash, and removes all records. No generic raw dispatch, `Ban` fallback, provider/model, or agent semantic route is used by S4.

## Focused execution evidence

All commands executed in `/Users/ab/workspace/go-projects/zenbot`.

| Command | Result |
|---|---|
| `go test ./internal/command -run 'Test(ShadowBan|UnshadowBan|RegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' -count=1 -v` | PASS; 12 named S4/catalog/capability tests, `ok zenbot/internal/command 0.509s` |
| `go test -race ./internal/command -run 'Test(ShadowBan|UnshadowBan|RegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' -count=1` | PASS; `ok zenbot/internal/command 2.492s`; no race report |
| `go test ./internal/repository/h2 -run 'Test(PersistShadowBan|RemoveShadowBan|ShadowBan)' -count=1 -v` | PASS; four real-H2 tests, `ok zenbot/internal/repository/h2 3.084s` |
| `go test ./internal/command -count=1` | PASS; `ok zenbot/internal/command 7.699s` |
| `go test ./... -count=1` | PASS; all packages, including `internal/command` (`8.708s`) and real-H2 repository package (`36.313s`) |
| `go vet ./internal/command` | PASS (exit 0) |
| `git diff --check` | PASS (exit 0) |

### Real-H2 proof

`internal/repository/h2/shadow_ban_test.go` ran against `h2fixture.Open` real H2:

- `TestPersistShadowBanStoresTrustedIdentityInRealH2`: raw hash persisted Base64 with trusted trip/name/reason.
- `TestRemoveShadowBanBySourceTargetMatchesNameTripAndBase64Name`: a single parameterized deletion matches name, trip, and Base64-hash variants, leaves unrelated row, and permits idempotent zero-row retry.
- `TestRemoveShadowBanBySourceTargetTreatsSQLLookingNameAsValue`: SQL-looking target does not remove unrelated data.
- `TestShadowBanCommandRecordsRoundTripAndDeleteAllInRealH2`: typed active record Base64 round-trips decoded, nullable offline name-only record round-trips, and `DELETE FROM banned_users` leaves an empty typed list.

### Coverage note

Focused command invocation (`go test ./internal/command -cover -run 'Test(ShadowBan|UnshadowBan|RegisterUserUtilitiesDoesNotExposeShadowBanCommandsWithoutTypedKick|AdminModeratorCatalog(GenericFallbackIsExplicitlyBounded|MatchesSaturnSource))' -count=1`) reported **12.0% of statements for the whole `internal/command` package**. This is package-wide coverage, not a misleading file-level S4 coverage claim.

## Fixes and static-analysis outcome

- **Fixes applied by this QA pass:** none. Source and focused behavior met the stated S4 contract; no failing regression capable of justifying a TDD change was found.
- `go vet ./internal/core` remains non-zero only at the known out-of-scope warning: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`. It was not changed.
- Worktree was already dirty before QA, including S2/S3/S5 and agent-path work. It was preserved.

## Final status

**S4 public shadow-ban command parity: PASS.** Required focused/race/real-H2/command/full-suite/static/diff validations are green, except the documented pre-existing out-of-scope core copylock vet warning.
