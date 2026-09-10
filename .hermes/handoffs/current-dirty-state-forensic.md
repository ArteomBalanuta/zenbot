# Current dirty Saturn → Zenbot migration: forensic diagnostic

**Scope.** Read-only analysis of the dirty worktree at `/Users/ab/workspace/go-projects/zenbot`. This report distinguishes current checkout evidence from inherited handoff claims. No application source was edited.

## 1. Current evidence snapshot

- [OBSERVED] `git status --short` before this report contained **22 tracked modifications** and the supplied migration files/handoffs as untracked entries; the current status has additional pre-existing handoff artifacts, for **83** entries before this report was written. The tracked changes are concentrated in composition, command dispatch, core moderation, repository queries, and services.
- [OBSERVED] `git diff --check` exits 0.
- [TEST-BACKED] The current checkout passes `go test ./... -count=1`, including `internal/command`, `internal/core`, `internal/repository/h2`, `internal/service`, `internal/agent/{participation,live}`, and `cmd/zenbot`. The real-H2 package took about 36 seconds.
- [LIMITATION] `go vet ./...` currently fails at `internal/core/engine_impl.go:96`: `NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl`. The current diff adds `prefixMu sync.RWMutex` to `EngineImpl`, but this analysis did not establish whether a mutex-copy warning already existed before the dirty baseline. In either case, the current constructor/copy design needs review before a clean gate.

## 2. Vertical-slice ledger

| Slice | State | Checkout evidence | Boundary / remaining qualification |
|---|---|---|---|
| S1 typed raw moderation protocol | **Implemented, focused-test backed** | `internal/common/moderation_operations.go` defines the closed `common.ModerationOperations`; `internal/core/moderation_operations.go` uses `moderationPayload` + `json.Marshal` for ban, unban, unbanall, lock/unlock, captcha, auth/deauth, mute/unmute, flair/color, kick, and overflow. `internal/core/moderation_operations_test.go:TestRawModerationOperations…` asserts payloads, escaping, cancellation/no send, and transport errors. | The interface is a runtime capability; commands must not be exposed on engines that do not implement it. |
| S2 simple manual moderation | **Implemented, focused-test backed** | Concrete adapters are in `internal/command/moderation_simple.go`; `handlers.go:newCommand` selects them for `captcha`, `authorize`, `deauthorize`, `lock`, `overflow`, `ban`, `unban`, and `unbanall`. Legacy `Ban`, `Unban`, `UnbanAll`, and `Lock` now delegate to these adapters. `internal/command/moderation_s2_test.go` covers roles/aliases, source-shaped usage/replies, payloads, cancellation and capability registration. | See §3: the live legacy dispatcher deliberately uses `context.Background()` and only logs command errors. Thus handler-level cancellation guarantees are not propagated by inbound legacy dispatch. |
| S3 active-target moderation | **Implemented, focused-test backed** | `internal/command/mute.go` implements `mute`/`dumb`, `unmute`/`undumb`, `color`, and `flair`; it resolves active targets through `GetActiveUserByName`, preserves the raw unmute hash, and emits canonical active name/hash. `internal/command/mute_color_flair_test.go` covers case resolution, absence, failure, cancellation, replies, aliases and registration. | Active target policy is command-specific and does not reuse semantic-monitor protection gates, matching the stated migration boundary. |
| S4 public shadow-ban family | **Implemented, focused-test backed** | `internal/command/shadow_ban.go` implements list, active/offline/contains shadowban, and unshadowban; `internal/service/shadow_ban.go`, `internal/repository/repository.go`, and `internal/repository/h2/shadow_ban.go` supply typed local persistence. `internal/repository/h2/shadow_ban_test.go` real-H2 tests Base64 round-trip, name/trip/hash removal, injection-safe bindings and all-delete; `internal/command/shadow_ban_test.go` covers active/offline/contains, cancellation and registration. | The source-shaped `-all` unshadowban path intentionally returns `FAILED` even after deleting and replying (`shadow_ban.go:147-163`); this is unusual but explicitly asserted in `TestUnshadowBanSingleAndAllDeleteUseRepositoryAndNoAgentPath`. |
| S5 moderator activity | **Implemented, focused-test backed** | `internal/command/activity.go` → `service.ActivityService.Stats` → `repository.ActivityRepository` → `h2.Database.ActivityStats`; factory composition is conditional in `internal/factory/engine_factory.go`. Activity query/result, no-row, error-as-result, cancellation, capability registration and real-H2 grouping/filtering are covered by `activity*_test.go`. | `ActivityService.Stats` intentionally converts a repository error to textual successful result (`internal/service/activity.go:17-23`), unlike the usual error-return rule. This is source-specific behavior, not a shared service policy. |
| Regular-user lastonline | **Implemented, focused-test backed** | `LastOnlineRecord`/`UserQueryRepository.LastOnline`, H2 two-query implementation, `UserService.LastOnline`, and `lastonlineCommand` are wired by `services.go`, `handlers.go`, and capability-gated registration. Tests cover aliases, normalization, output escaping, real-H2 history/presence behavior and listener fake compatibility. | Registration requires `Bundle.Users.Queries`; missing capability means aliases are absent, not generic fallback. |
| Admin prefix | **Implemented, focused-test backed** | `common.PrefixController`, `core.EngineImpl.UpdatePrefix`, locked `GetPrefix`, `prefixCommand`, and capability-gated registration are present. `prefix_state_test.go` covers host/current-replica propagation and concurrent read/update; `prefix_test.go` covers parsing, cancellation and registration. | Explicitly volatile: no persistence and no propagation to later-created replicas (`internal/core/prefix_state.go`). The new lock also produces the current `go vet` copylock finding, which must be resolved or consciously accepted before a clean gate. |
| Semantic-moderation reversal primitives | **Partially implemented / intentionally unactivated** | `internal/core/moderation_target.go` snapshots listener-resolved identity; `moderation_reversal.go` exposes typed `UnmuteTarget` and `UnshadowBanTarget`; `moderation_reversal_test.go` verifies blank/cancel failure, exact unmute payload, bounded queue deadline, authoritative repository use, and no protocol call for unshadowban. | No production call site invokes either reversal primitive. They are not a complete autonomous moderation action loop. |
| Semantic severe-abuse ingress | **Scaffolding only; fail closed** | Candidate pattern and Unicode boundary implementation are in `internal/agent/participation/semantic_moderation.go`; `Pipeline` and live adapter support an injected candidate/typed target (`invocation.go`, `live/participation.go`). Main sets readiness from `SemanticModerationIngressReady()`. `cmd/zenbot/live_agent_test.go` asserts candidate remains nil and readiness false. | `SemanticModerationIngressReady()` is hard-coded `false` and `newLiveAgent` does not compose `RoomParticipation.SemanticCandidate`. There is no agent moderation gateway/tool mapping, so no live semantic candidate can submit. |

## 3. Registration, capability, and composition findings

### [OBSERVED] Concrete public registrations are capability gated

`internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` is the runtime registration source, called from `cmd/zenbot/main.go:274`.

- `common.ModerationOperations` gates all S1/S2/S3 aliases. It also gates `shadowban`, which requires typed `KickNick` after persistence.
- `Bundle.ShadowBans.Repo` independently gates `shadowbanlist` and `unshadowban`.
- `Bundle.Activity.Repo` gates `active`/`activity`; `Bundle.Users.Queries` gates lastonline aliases; `common.PrefixController` gates prefix.
- `factory.NewEngineWithOptions` populates Activity and ShadowBans services only when the supplied repository implements the corresponding narrow interfaces (`internal/factory/engine_factory.go`).

[RECOMMENDED] Preserve these independent gates in future composition changes. Do not register aliases merely because `RegisterAll` has their metadata: the catalog is broader than the executable runtime registry.

### [RISK] The live dispatch bridge drops command context and return status

`legacyAdapter.Execute` in `internal/command/dispatch_adapter.go:22-27` invokes every `SaturnCommand` with `context.Background()` and only writes errors to the process log. Consequently:

1. direct handler tests correctly demonstrate cancellation/no-side-effect behavior, but ordinary inbound command dispatch cannot inherit a caller/transport cancellation;
2. a command error has no structured dispatch/audit status at this bridge;
3. the retained legacy `Ban`, `Unban`, `UnbanAll`, and `Lock` wrappers also discard the delegated status/error.

This does not make S1–S4 unimplemented, but it is a cross-slice lifecycle/audit gap relative to the context-aware new command API. A migration claim should say **handler context-aware; legacy live dispatch contextless**, not end-to-end cancellation-safe.

### [RISK] Semantic readiness documentation and actual primitives have diverged

`semantic_moderation.go:68-72` says unmute/unshadowban typed operations do not exist, but the dirty tree now has two separate target-bound reversal methods (`EngineImpl.UnmuteTarget` and `EngineImpl.UnshadowBanTarget`) and conventional manual methods (`UnmuteHash` plus public unshadowban service path).

[OBSERVED] The false readiness result remains safe because main leaves `SemanticCandidate` nil.

[RECOMMENDED] Before any activation, replace the stale prerequisite rationale with an explicit reviewed gateway contract: all six source aliases (`captcha`, `mute`, `unmute`, `kick`, `shadowban`, `unshadowban`) must map from one canonical listener-resolved target into typed operations, with cancellation/action isolation and no public `run_command` or `DispatchUserCommand` reuse. Do **not** flip the boolean merely because similarly named manual/reversal methods exist.

### [RISK] Prefix synchronization needs a clean design gate

`EngineImpl` now contains `prefixMu`, while `NewEngineImpl` returns/copies an `EngineImpl` value (`internal/core/engine_impl.go:96` according to vet). Copying a value after containing a mutex is a correctness hazard even if current focused prefix tests pass. `UpdatePrefix` also holds the host lock while calling `setPrefix` on replicas, so future bi-directional prefix updates or changed replica ownership need a lock-order review.

[RECOMMENDED] Make the construction/copy semantics explicit (normally construct and retain `*EngineImpl`, or move synchronization into an uncopyable pointer-owned state) and rerun `go vet ./...`, prefix race tests, and full suite.

## 4. Cross-slice consistency assessment

1. [OBSERVED] The command catalog is no longer the implementation ledger. `internal/command/admin_moderator_catalog_guard_test.go` confirms exact 11-admin/23-moderator metadata and has only eleven permitted generic fallbacks: `mine`, `replica`, `replicaoff`, `replicastatus`, `restart`, `shutdown`, `sql`, `whiskey`, `automove`, `nuke`, and `resurrect`. All other scoped rows resolve to a concrete `newCommand` case.
2. [OBSERVED] This means the dirty state has closed the simple, active-target, shadow-ban, activity, and prefix command families, but **not** complete admin/moderator parity. The remaining generic admin rows are mine, replica family, restart, shutdown, SQL, and Whiskey; remaining generic moderator rows are automove, nuke, and resurrect.
3. [OBSERVED] The H2 shadow-ban boundary is consistently local persistence: public shadowban persists then typed-kicks an active user, whereas unshadowban deletes local records without a server protocol call. The autonomous reversal uses the same local removal semantics but only after a typed target snapshot.
4. [OBSERVED] Source-specific error semantics differ between slices: activity turns database errors into rendered success text, while moderation protocol/repository errors return `FAILED` and suppress success replies. Treat this as explicit per-command parity, not a generic service convention.
5. [LIMITATION] Existing tests prove handlers, registration predicates, payloads, and focused real-H2 storage. They do not by themselves prove every concrete moderator alias through a real authenticated inbound listener with command audit status, or prove live semantic action behavior (which is deliberately disabled).

## 5. Evidence commands to retain/run

Run from repository root. These are ordered from fast/high-signal to broad.

```sh
# Current tree integrity and known static issue
git diff --check
git status --short
go vet ./...                         # currently fails only at EngineImpl copylock

# Registry / capability / public-command contracts
go test ./internal/command -run 'Test(AdminModeratorCatalog|RegisterUserUtilities|ManualSimpleModeration|Mute|Unmute|Color|Flair|ShadowBan|UnshadowBan|Activity|LastOnline|LastSeen|Prefix)' -count=1 -race

# Exact raw protocol + reversal target boundaries
go test ./internal/core -run 'Test(RawModerationOperations|UnmuteTarget|UnshadowBanTarget|NewModerationTarget|UpdatePrefix|GetPrefix)' -count=1 -race

# H2 persistence/query evidence
go test ./internal/repository/h2 -run 'Test(ActivityStats|LastOnline|PersistShadowBan|RemoveShadowBan|ShadowBan)' -count=1

# Service formatting/error semantics
go test ./internal/service -run 'Test(ActivityService|UserServiceLastOnline|ShadowBan)' -count=1

# Disabled semantic ingress and composition proof
go test ./internal/agent/participation ./internal/agent/live ./cmd/zenbot \
  -run 'Test.*(Semantic|Moderation|Participation|LiveAgent)' -count=1 -race

# Final regression gate
go test ./... -count=1
```

## 6. Completion recommendation

[RECOMMENDED] Treat S1–S5, lastonline, and prefix as independently implemented command verticals with a currently green full test suite, rather than declaring the entire Saturn admin/moderator migration complete. Before calling the worktree release-clean, resolve the `EngineImpl` mutex-copy diagnostic, decide whether the legacy dispatch bridge needs cancellable/error-reporting/audit semantics, and preserve semantic ingress as disabled until a dedicated reviewed agent action gateway exists.
