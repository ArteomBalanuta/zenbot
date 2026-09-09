# `automove` Technical Specification — current architecture handoff

**Classification: High Complexity / High Risk.** This is a bounded implementation handoff only. No application or test source was changed to produce it. The target working tree was already dirty; it must not be reset, cleaned, staged, committed, or otherwise normalized.

## 1. Scope, authority, and decision status

**Saturn authority**

- `../saturn/src/main/java/org/saturn/app/command/impl/moderator/AutoMoveUserCommandImpl.java`
- `../saturn/src/main/java/org/saturn/app/listener/impl/UserJoinedListenerImpl.java`

**Target seams inspected in full**

- `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/command/registry.go`
- `internal/listener/user_joined_listener.go`, `internal/listener/message/handlers.go`
- `internal/core/replica_controller.go`, `internal/core/replica_manager.go`, `internal/core/engine_impl.go`, `internal/core/moderation_operations.go`
- `internal/common/engine.go`, `internal/common/moderation_operations.go`
- `internal/factory/engine_factory.go`, `internal/factory/replica_factory.go`, `cmd/zenbot/main.go`
- concrete `nuke` / `resurrect` command and registration patterns.

### Decision requiring user input

**None is required to implement bounded Saturn parity.** Implement volatile, process-local state only: enabled flag, additive source rooms, destination room. Do not add configuration, persistence, schema, cross-process synchronization, retries, or source removal.

User input is required **only** if the product deliberately wants to depart from source behavior: persistence across restart, a configurable/dynamic move notice, automatic launch after `automove <source> <destination>` while already enabled, source removal, case-insensitive eligibility, or guaranteed transactional all-or-nothing replica stopping. Those are excluded from the parity slice.

## 2. Observed evidence

### 2.1 Saturn behavior `[OBSERVED]`

| Concern | Evidence |
|---|---|
| Command identity | `AutoMoveUserCommandImpl` is `@CommandAliases({"automove"})` and returns `Role.MODERATOR` (lines 19–20, 42–44). |
| Initial/static state | Process-static `AUTO_MOVE_STATUS=false`, `DESTINATION_CHANNEL="lounge"`, and static `SOURCE_CHANNELS`; each command construction adds `purgatory` (lines 21–23, 33–39). |
| Usage/parse | `execute` handles the first argument as a toggle before checking arity; exact two non-toggle arguments configure; all other forms show usage and return `FAILED` (lines 47–67). |
| Enable | `on` sets enabled first, then for each source not in host `replicasMappedByChannel` invokes the replica registration path (lines 81–118). |
| Disable | `off` clears enabled first, removes each source replica from the host map, and stops the removed replica; absent entries are skipped (lines 121–141). |
| Configure | Two arguments add the source and replace destination; they do not launch a replica even if enabled (lines 144–149). |
| Join gates | Saturn join handling requires enabled, `EngineType.REPLICA`, and `SOURCE_CHANNELS.contains(engine.channel)` (listener lines 65–69). |
| Eligibility | It executes `SELECT trip FROM trips WHERE type = 'USER'` and Java `List.contains(user.trip)`: exact/case-sensitive string membership (listener lines 70–100; `SqlUtil.java` lines 81–84). |
| Join action | Eligible joins enqueue a public address notice, then `kickTo(nick, destination)` (listener lines 70–77). `ModServiceImpl.kickTo` normalizes nick and emits `{"cmd":"kick","nick":...,"to":...}` (lines 273–277). |

### 2.2 Zenbot state `[OBSERVED]`

- The catalog is already source-shaped: canonical `automove`, sole alias `automove`, role `model.MODERATOR` in `internal/command/registry.go:RegisterAll` (line 133). The help text already advertises `automove <on|off>` in `internal/command/help.go` (line 81).
- `internal/command/handlers.go:newCommand` lacks an `automove` case, therefore returns generic `saturnCommand`; that fallback replies ` automove accepted` (`internal/command/registry.go:Execute`, lines 95–96). `internal/command/admin_moderator_catalog_guard_test.go` expressly allowlists it as generic (lines 113–120).
- Production registration is stricter: `RegisterUserUtilitiesWithDirectAgent` does **not** include `automove` in its canonical list (`internal/command/dispatch_adapter.go`, lines 46–101). Thus the catalog/factory can construct a generic fallback, but normal production composition does not currently expose it as a registered command.
- Runtime command authorization happens before `Execute`: `message.DispatchUserCommand.Handle` resolves a registered command, calls `Engine.IsUserAuthorized(author, cmd.GetRole())`, replies on denial, and only then calls `cmd.Execute()` (`internal/listener/message/handlers.go`, lines 169–189).
- `legacyAdapter.Execute` invokes concrete Saturn commands under `context.Background()` and logs returned errors (`internal/command/dispatch_adapter.go`, lines 12–35). This limits cancellation available to chat-dispatched commands; direct command tests can still verify a pre-cancelled context.
- `core.ReplicaManager` owns the replica map under an RW mutex, rejects blank/host/duplicate additions, returns a copy in `Replicas`, removes before stopping, and becomes terminal after `StopAll` (`internal/core/replica_manager.go`).
- `core.ManagedReplicaController.AddReplica` constructs, starts, registers, and stops a just-started engine if registration fails; it also removes a failed replica from the manager (`internal/core/replica_controller.go`, lines 32–60). `ReplicaFactory.NewReplica` copies configuration and sets `model.REPLICA` (`internal/factory/replica_factory.go`).
- `EngineImpl` exposes `AddReplica`, `RemoveReplica`, and `ReplicaChannels` only through its installed managed controller (`internal/core/engine_impl.go`, lines 252–271). Main creates one manager/controller and later calls `manager.StopAll(stopCtx)` after host lifecycle stop (`cmd/zenbot/main.go`, lines 287–301 and 320–329).
- The concrete typed `KickNickTo(ctx, target, channel)` boundary already exists in `common.ModerationOperations` and `core.EngineImpl`; it validates/normalizes nick and sends the exact raw kick payload (`internal/common/moderation_operations.go:14–33`; `internal/core/moderation_operations.go:99–131`).
- A permanent engine gets `UserJoinedListener`; a temporary online-set engine gets dummy join listeners. Current permanent join order is parse → `AddActiveUser` → existing semantic `JoinAutomation` → subscription `shareUserInfo` → `LogPresence` (`internal/listener/user_joined_listener.go`; `internal/factory/engine_factory.go`, lines 88–102). There is no automove hook.
- Existing target repository interfaces do not expose the source’s USER-trip list. `UserQueryRepository` is used for users/nicks/history, not this eligibility query (`internal/repository/user_queries.go`). H2 currently has no `SELECT trip FROM trips WHERE type='USER'` method (`internal/repository/h2/user_queries.go`).

## 3. Source-target semantic mismatches and required fidelity choices

| Topic | Source | Target / required recommendation |
|---|---|---|
| State visibility | Java static fields are shared across engines in one JVM. | Construct **one shared Go state pointer** in main and pass it to host and every future permanent replica. Per-engine copied state is incorrect. |
| Default-source initialization | Java adds `purgatory` on every command object construction; set semantics make it effectively present. | Initialize it once in the state constructor. This preserves observable state while removing a needless construction side effect. |
| Destination notice | Saturn always says `?lounge` in both clauses, even if the configured destination differs. | Preserve this literal source notice; only the raw kick `to` uses dynamic destination. Do not “fix” the wording without a product decision. |
| Existing join effects | Saturn performs shadow-ban/recently-online work before automove. Zenbot currently does not have equivalent steps in this listener. | Add automove **last** after all current target effects; do not invent/reuse remote composition or alter semantic moderation ordering. |
| State validation | Saturn accepts nonblank/nonempty-ish tokens after trim; it does not normalize case. | Trim source/destination, reject blank values to preserve managed-replica invariants; exact/case-sensitive source and trip matching. This is a bounded safety adaptation; document it in behavior tests. |
| Lifecycle errors | Saturn does not surface structured start/stop errors. | Preserve state transition order, return Go lifecycle error to command adapter, omit success reply. On disable, continue attempting all source stops and return the first error. |
| Context | Saturn listener is void/no cancellation. | Listener keeps current `context.Background()` contract; new typed methods honor context when directly invoked. Do not broaden `legacyAdapter` or the inbound listener stack in this slice. |

## 4. Recommended ownership and typed contracts `[RECOMMENDED]`

Create `internal/common/automove.go`:

```go
type AutoMoveSnapshot struct {
    Enabled     bool
    Sources     []string // sorted immutable copy
    Destination string
}

type AutoMoveController interface {
    AutoMoveSnapshot() AutoMoveSnapshot
    ConfigureAutoMove(source, destination string) (AutoMoveSnapshot, error)
    EnableAutoMove(context.Context) (AutoMoveSnapshot, error)
    DisableAutoMove(context.Context) (AutoMoveSnapshot, error)
}

type AutoMoveJoinPolicy interface {
    EligibleReplica(channel string) (destination string, ok bool)
}
```

Create `internal/repository/automove.go`:

```go
type AutoMoveTripRepository interface {
    UserTrips(context.Context) ([]string, error)
}
```

Create `internal/core/auto_move.go`:

```go
type AutoMoveState struct {
    mu          sync.RWMutex
    enabled     bool
    sources     map[string]struct{}
    destination string
}

func NewAutoMoveState() *AutoMoveState // false, {"purgatory"}, "lounge"
func (s *AutoMoveState) Snapshot() common.AutoMoveSnapshot
func (s *AutoMoveState) Configure(source, destination string) (common.AutoMoveSnapshot, error)
func (s *AutoMoveState) SetEnabled(bool) common.AutoMoveSnapshot
func (s *AutoMoveState) EligibleReplica(channel string) (destination string, ok bool)
```

`EngineImpl` owns an `autoMove *AutoMoveState` and a **separate controller/reconciliation mutex**. It implements `common.AutoMoveController`:

1. `EnableAutoMove(ctx)` checks context, serializes reconciliation, marks enabled, snapshots sources, then obtains `ReplicaChannels()` and invokes existing `AddReplica(ctx, source)` only for absent exact sources.
2. `DisableAutoMove(ctx)` checks context, serializes reconciliation, marks disabled, snapshots sources, and calls `RemoveReplica(ctx, source)` only for channels present in the manager snapshot. Continue after individual errors and return the first.
3. `ConfigureAutoMove` only adds source/replaces destination. It must **not** create a replica.

State locking protects only state. Never hold state, manager, or active-user locks during H2, outbound sends, construct/start, or stop. The reconciliation mutex prevents `on`/`off` interleaving; it must not replace `ReplicaManager` ownership.

Create `internal/listener/automove_join.go` with a narrow policy/action seam:

```go
type AutoMoveJoinAutomation struct {
    State      common.AutoMoveJoinPolicy
    Trips      repository.AutoMoveTripRepository
    Move       common.ModerationOperations
    SendNotice func(string, string, bool) (string, error)
    Channel    func() string
    IsReplica  func() bool
}
func (a *AutoMoveJoinAutomation) OnJoin(context.Context, *model.User)
```

It is listener policy, not command policy and not repository policy. It snapshots/gates first; only an enabled source replica queries `UserTrips`. Exact match on `user.Trip` authorizes. It sends source’s literal notice first:

```text
your trip is authorized to join ?lounge, you will be moved to ?lounge
```

Then it calls `Move.KickNickTo(ctx, common.NickTarget(user.Name), common.Channel(destination))`. If the notice errors, log it and still attempt the kick (Saturn has no notice error result); a trip-query error logs and performs neither action. A pre-cancelled context performs neither outbound action.

## 5. Command behavior, registration, and execution gates

### Exact syntax / aliases / role `[OBSERVED]`

- Canonical: `automove`
- Aliases: exactly `["automove"]`
- Authorized role: `MODERATOR`
- Prefix is the engine’s current prefix.

### State transition table `[RECOMMENDED; source-shaped]`

| Arguments after `automove` | State/lifecycle | Result |
|---|---|---|
| none | unchanged | three usage/status/configuration replies; `FAILED` |
| `on` plus any trailing tokens | enabled first; ensure a replica for every currently configured missing source | enabled reply and `SUCCESSFUL`, or `FAILED,error` with no success reply |
| `off` plus any trailing tokens | disabled first; stop/remove every currently present source replica | disabled reply and `SUCCESSFUL`, or `FAILED,error` with no success reply |
| exactly two non-toggle arguments | add first as source; replace destination; no start/stop | configuration reply and `SUCCESSFUL` |
| one non-toggle, or 3+ non-toggle | unchanged | three usage/status/configuration replies; `FAILED` |
| pre-cancelled context | unchanged | `FAILED, ctx.Err()` and no reply/replica operation |

Toggle matching is case-insensitive because Saturn uses `equalsIgnoreCase`; trailing tokens are ignored because toggle resolution precedes the two-argument branch. Source/destination configuration does not remove any source.

Required source-shaped replies (format values deterministically with sorted sources; Java `HashSet` order is not stable):

```text
<prefix>automove [on|off]
Current status: <bool> , Source rooms: <sources> , Destination room: <destination>
To set source: hell, destination: heaven - use: <prefix>automove hell heaven
 <prefix>automove is enabled
 <prefix>automove is disabled
Set source channel: <sources> , destination channel: <destination>. Make sure bot's REPLICA is serving source channels.
```

### Registration and authorization `[RECOMMENDED]`

- Add `case "automove": return &automoveCommand{commandBase: b}` to `internal/command/handlers.go:newCommand`.
- Remove `automove` from `saturnCommand`’s generic acceptance case and from `allowedScopedGenericFallbacks` in `admin_moderator_catalog_guard_test.go`.
- In `RegisterUserUtilitiesWithDirectAgent`, append `automove` **only** when `e` implements `common.AutoMoveController`. This follows capability gating already used for `nuke`, `resurrect`, prefix, and replica commands; an incomplete engine must not expose a no-op moderator command.
- `automoveCommand.Execute` type-asserts `common.AutoMoveController`; an invocation bypassing registration fails closed if absent.
- Do not duplicate authorization inside the command. Dispatch’s existing `DispatchUserCommand` role gate is authoritative; tests must prove its registered adapter presents `MODERATOR`.

## 6. Listener order and end-to-end sequence

### Permanent replica join path `[OBSERVED + RECOMMENDED]`

```text
onlineAdd JSON
  -> EngineImpl.DispatchMessage("onlineAdd")
  -> UserJoinedListener.Notify
  -> model.GetUser
       parse error -> return (no registration, policy, send, or kick)
  -> EngineImpl.AddActiveUser(user)
  -> existing semantic JoinAutomation.OnJoin(background, user)
  -> existing shareUserInfo(user)
  -> existing LogPresence(..., "joined", channel)
  -> NEW AutoMoveJoinAutomation.OnJoin(background, user) [last]
       -> shared state: enabled && exact configured source && IsReplica
       -> AutoMoveTripRepository.UserTrips(background)
       -> exact USER-trip membership
       -> public literal ?lounge notice
       -> ModerationOperations.KickNickTo(user.Name, snapshot.destination)
```

Host and temporary online-set engines do not move a user: host fails `IsReplica`; temporary sessions retain dummy join listeners. The hook must retain add-active-user-before-policy behavior demonstrated by `TestUserJoinedListenerInvokesTrustedAutomationAfterRegistrationAndIgnoresMalformed`.

### Command/lifecycle path `[RECOMMENDED]`

```text
moderator chat !automove on
  -> DispatchUserCommand role authorization (MODERATOR)
  -> legacyAdapter.Execute(background)
  -> automoveCommand.Execute
  -> EngineImpl.EnableAutoMove
       -> state enabled=true; source snapshot
       -> ManagedReplicaController.AddReplica for each missing source
           -> ReplicaFactory.NewReplica(shared AutoMoveState)
           -> StartContext
           -> ReplicaManager.Add
  -> issuer enabled reply

eligible user joins source replica
  -> shared AutoMoveState eligibility
  -> USER-trip query -> literal notice -> typed kick-to dynamic destination

moderator chat !automove off / process shutdown
  -> Disable: state enabled=false, manager Remove -> replica StopContext
  -> shutdown remains lifecycle.Stop(host), then ReplicaManager.StopAll
```

## 7. Cancellation, errors, races, and shutdown

- `EnableAutoMove` or `DisableAutoMove` checks `ctx.Err()` before mutating state. Existing chat dispatch supplies background context, so timeout cancellation is only testable/directly usable until a separate context propagation slice.
- Enable state remains true if a replica start fails: Saturn sets it before reconciliation. Return the error, report via existing controller error sink, and do not send success text.
- Disable state remains false if a stop fails. `ReplicaManager.Remove` deletes before `Stop`; return first failure after attempting remaining sources. A later `on` can recreate any missing source replica.
- `ReplicaManager.StopAll` is terminal. After shutdown begins, an enable attempt fails via the manager; no automove goroutine/retry cleanup is introduced.
- Existing replica runtime-failure handler removes the failed replica. Do not remove the configured source from automove state; future `on` reconciles it, matching Saturn’s retained source set.
- A join may race `off` after it has captured an enabled snapshot. Strong linearizability between a network join and disable is neither present in Saturn nor appropriate without holding locks across I/O. Bound the behavior: no data race; a pre-gate disabled snapshot sends nothing; an already-qualified in-flight join may still send one notice/kick.
- The join hook must not move `nil`, blank-name, malformed, host, non-source, disabled, non-USER, or trip-query-error users. `KickNickTo` provides final nick normalization/validation.

## 8. Exact file map

| File | Change |
|---|---|
| `internal/common/automove.go` | **Create:** shared controller, snapshot, and join-policy interfaces. |
| `internal/core/auto_move.go` | **Create:** synchronized volatile state and `EngineImpl` lifecycle reconciliation. |
| `internal/core/auto_move_test.go` | **Create:** defaults, snapshots, enable/disable, lifecycle error, race tests. |
| `internal/repository/automove.go` | **Create:** narrow `AutoMoveTripRepository`; keeps USER-trip eligibility separate from users/nicks/history queries. |
| `internal/repository/h2/user_queries.go` | **Modify:** `UserTrips(ctx)` using exact `SELECT trip FROM trips WHERE type = 'USER'`. |
| `internal/repository/h2/user_queries_test.go` | **Modify:** real H2 USER-only/case-preserving query tests. |
| `internal/command/automove.go` | **Create:** concrete parser, replies, controller calls. |
| `internal/command/automove_test.go` | **Create:** syntax/state/replies/cancel/capability tests. |
| `internal/command/handlers.go` | **Modify:** concrete `automove` case. |
| `internal/command/dispatch_adapter.go` | **Modify:** capability-gated registration. |
| `internal/command/admin_moderator_catalog_guard_test.go` | **Modify:** remove fallback allowance. |
| `internal/listener/automove_join.go` | **Create:** join policy/action. |
| `internal/listener/automove_join_test.go` | **Create:** eligibility/order/errors. |
| `internal/listener/user_joined_listener.go` | **Modify:** compose/invoke new hook last. |
| `internal/listener/user_joined_listener_test.go` | **Modify:** explicit existing/new hook ordering and malformed behavior. |
| `internal/factory/engine_factory.go` | **Modify:** carry shared state and compose permanent listener hook fail-closed. |
| `internal/factory/replica_factory.go` | **Modify:** pass same state through replica options. |
| `internal/factory/engine_factory_test.go` | **Modify:** pointer identity and permanent/temporary composition. |
| `cmd/zenbot/main.go` | **Modify:** create one state, pass it to host and `ReplicaFactory.Options`. |

**Do not modify:** config, schema/migrations, remote/snapshot composition, agent semantic moderation, Saturn source, or unrelated dirty files.

## 9. First minimal RED test and full test map

### First minimal RED test

Create `internal/command/automove_test.go` first:

```go
func TestAutoMoveDefaultUsageIsConcreteModeratorCommand(t *testing.T) {
    // A capability stub starts with false/{purgatory}/lounge.
    // commandDefinitionFor("automove").New(...).Execute(context.Background())
    // must return FAILED, send the three exact source-shaped usage replies,
    // and make no enable/disable/configuration call.
}
```

This is the smallest test that proves the generic fallback has been replaced without starting a replica or querying H2.

### Full ordered RED → GREEN map

1. `internal/command/automove_test.go`: default usage/concrete type, role/alias, `on|off` with trailing arguments, two-token additive configure, invalid arity, capability absence, and pre-cancelled context.  
   `go test ./internal/command -run '^TestAutoMove' -count=1`
2. `internal/core/auto_move_test.go`: defaults, sorted immutable snapshots, exact case membership, configure-no-start, absent-only enable, present-only disable, start/stop failure state semantics, and concurrent on/off reconciliation.  
   `go test ./internal/core -run '^TestAutoMove' -count=1` then `go test -race ./internal/core -run '^TestAutoMove' -count=1`
3. H2 repository test: USER rows only, preserved original trip case, row/scan/context error propagation.  
   `go test ./internal/repository/h2 -run 'TestUserTrips' -count=1`
4. `internal/listener/automove_join_test.go`: disabled, host, temporary, non-source, non-USER, case mismatch, trip-query error, cancellation, notice-error-still-kicks, dynamic raw destination, and literal `?lounge` notice.  
   `go test ./internal/listener -run '^TestAutoMoveJoin' -count=1`
5. Extend `user_joined_listener_test.go`: registered user visible to existing and new policy; semantic → share → log → automove order; malformed payload remains inert.  
   `go test ./internal/listener -run 'Test(UserJoinedListener|AutoMoveJoin)' -count=1`
6. Factory/registration integration: incomplete engine has no automove registration; fully composed host registers MODERATOR alias; host/replica share state pointer; replica onlineAdd moves an eligible USER trip.  
   `go test ./internal/command ./internal/listener ./internal/core ./internal/factory -count=1`

Final acceptance: `go test ./... -count=1`, focused race test above, `gofmt` only on owned new/changed Go files, and `git diff --check`.

## 10. Verification record (this handoff)

Executed from `/Users/ab/workspace/go-projects/zenbot` after evidence collection:

```text
$ go test ./internal/command ./internal/listener ./internal/core ./internal/factory -count=1
ok  zenbot/internal/command  9.136s
ok  zenbot/internal/listener 0.754s
ok  zenbot/internal/core     0.637s
ok  zenbot/internal/factory  0.959s

$ git diff --check
(exit 0; no output)
```

Citation QA: all material target citations above resolve in the current checkout; Saturn citations resolve under `/Users/ab/workspace/projects/saturn`. The only changed artifact for this task is this handoff file.
