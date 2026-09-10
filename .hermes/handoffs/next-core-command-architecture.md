# Next core command parity vertical: support-relay `ws` / `wsa`

## Decision

**Implement the two-command support-replica relay vertical next:** Saturn user commands `WhiskeySayUserCommandImpl` (`ws`, `wsay`) and `WhiskeyAnonUserCommandImpl` (`wsa`, `wsayanon`, `anonsay`).

This is the smallest remaining source-grounded concrete vertical after accepted DBZ and `grant`/`access` work that can use existing configured identity and managed-replica foundations without inventing temporary-session credentials, proxy transport, a new authorization model, or host lifecycle policy.

It is one bounded relay vertical, not implementation of the ADMIN `whiskey` proxy-replica command. It sends text only to an **already running managed replica named `support`**. It must remain capability-gated and unregistered when that replica gateway is absent.

## Current inventory refresh

### Accepted / no longer candidates

The current dirty worktree and newer QA records show concrete bounded acceptance for:

- DBZ: `dbzhelp`, `dbzregister`, `dbzstr`, `dfight`, `dspawn`, and `dbzstats`; latest stats QA is `.hermes/handoffs/dbzstats-tdd-recovery-qa.md`.
- identity/history including `register/reg`, `authorize/auth`, `messages/lastmessages`, and accepted functional baseline `grant/access`; latest access QA is `.hermes/handoffs/access-command-parity-qa.md`.
- user `lastonline`; moderation/admin state slices including prefix, activity, automove, nuke/resurrect, and generic replica lifecycle.
- `replica`, `replicaoff`, and `replicastatus`; `.hermes/handoffs/replica-qa.md` accepts their bounded lifecycle behavior.

The checked-in dispatch state confirms that `RegisterUserUtilitiesWithDirectAgent` registers the accepted DBZ, access, last-online, moderation, prefix, automove, and generic replica canonicals only when their existing capabilities are present (`internal/command/dispatch_adapter.go`). `newCommand` selects concrete handlers for those accepted canonicals (`internal/command/handlers.go`).

### Remaining concrete gaps relevant to command priority

- **Support relay user commands:** `ws`/`wsay` and `wsa`/`wsayanon`/`anonsay` are in the catalog but currently fall through `saturnCommand`; they are not registered by the adapter. The generic fallback only groups them with `msgchannel` today (`internal/command/registry.go`).
- **Remote message delivery:** `msgchannel`/`msgroom` only delivers to the current room; a remote target returns `remote room delivery is not configured`. Saturn needs a temporary credentialed LIST_CMD session using the host password and a generated nick.
- **Remaining ADMIN generic rows:** `mine`, `restart`, `shutdown`, `sql`, and `whiskey` remain the exact transitional generic fallback allowlist (`internal/command/admin_moderator_catalog_guard_test.go`).

## Candidate ranking

| Rank | Vertical | Recommendation | Why / blocker |
|---|---|---|---|
| 1 | **`ws`/`wsa` support relay** | **Proceed next** | Two small source command classes, direct Saturn tests, existing managed-replica lifecycle, no H2/schema/config/proxy/session/lifecycle work. A narrow capability can find only an existing `support` replica and send one message. |
| 2 | `msgchannel`/`msgroom` remote delivery | Defer | Requires temporary-session join identity: Saturn creates a generated nick and injects `engine.password`; Zenbot temporary sessions currently join `req.SourceChannel` using only `{"cmd":"join","channel":...}`. Implementing remote delivery now would require an identity/password request extension and target-channel correction. Same-room behavior is already concrete. |
| 3 | `mine` | Defer | Requires generated credentials, local secret persistence, a process-wide scheduler, credentialed temporary sessions, and an unresolved proxy/path policy; see `.hermes/handoffs/mine-current-architecture.md`. |
| 4 | ADMIN `whiskey` | Defer | Requires ordered proxy configuration, proxy-capable transport, named AGENT construction, readiness, and bounded backup lifecycle. Existing `WhiskeyProxyOrder` does not dial a proxy or own a replica; see `.hermes/handoffs/whiskey-current-architecture.md`. |
| 5 | `restart` / `shutdown` | Blocked | Process ownership, teardown/rebuild order, caller feedback, and terminal semantics need an explicit product decision; see `.hermes/handoffs/restart-shutdown-current-architecture.md`. |
| 6 | `sql` | Blocked | Raw operational SQL requires explicit disclosure, read-only grammar, privilege, timeout/cap, auditing, and error-delivery policy; see `.hermes/handoffs/sql-current-architecture.md`. |

No candidate above rank 1 is smaller while satisfying the requested constraint against invented identity, transport, security, proxy, or lifecycle policy.

## Saturn contract

### `ws` / `wsay`

**[OBSERVED]** `src/main/java/org/saturn/app/command/impl/user/WhiskeySayUserCommandImpl.java` declares exactly `ws`, `wsay` and overrides required role to `REGULAR`.

1. It derives `author` from the inbound nick.
2. It considers the caller an admin only when the inbound trip is present in configured comma-split `engine.adminTrips`; it does **not** consult the database role for this formatting choice.
3. It calls inherited `renderArguments(isAdmin)`: configured admin input preserves special characters; other callers have every non-ASCII-alphanumeric/non-space character removed. The inherited renderer joins arguments with a trailing space.
4. It looks up `engine.replicasMappedByChannel.get("support")`.
5. It queues exactly `author + ": " + StringEscapeUtils.escapeJava(message)` on that support engine and flushes it immediately with `shareMessages()`.
6. It sends no acknowledgement to the invoking room and returns successful.

**[TEST-BACKED]** `src/test/java/org/saturn/app/command/impl/user/WhiskeySayUserCommandImplTest.java` creates a `support` replica, executes `*ws test message`, and asserts success and an empty queue after the immediate flush. It does not characterize exact escaped payload or missing-support behavior.

### `wsa` / `wsayanon` / `anonsay`

**[OBSERVED]** `src/main/java/org/saturn/app/command/impl/user/WhiskeyAnonUserCommandImpl.java` declares exactly `wsa`, `wsayanon`, `anonsay`, has the same `REGULAR` role and argument-rendering branch, uses the same `support` lookup/flush, but queues:

```text
anon_from_hc: <Java-escaped rendered message>
```

It likewise returns successful and sends no caller-facing response.

**[TEST-BACKED]** `WhiskeyAnonUserCommandImplTest.java` proves success plus immediate queue flush with an existing `support` replica. It has the same limitations.

### Relevant source limitation

**[OBSERVED]** Neither source class checks for absent `support`; Java dereferences the map result, so that condition escapes as an unchecked failure rather than a fabricated success or message. Do not add a “support unavailable” chat reply. In Go, a capability/configuration error with `FAILED`, logged by the existing `legacyAdapter`, and no reply is the controlled equivalent; record it as an intentional target error adaptation rather than claim literal nil-pointer parity.

## Existing Zenbot seams

| Required concern | Observed target seam | Design consequence |
|---|---|---|
| Catalog | `internal/command/registry.go:RegisterAll` has exact source aliases and `model.USER` metadata for `ws` and `wsa`. | Retain aliases/role; do not add a command. |
| Authorization | `internal/listener/message/handlers.go:DispatchUserCommand` authorizes resolved callers before execution. | Reuse the normal USER gate; no command-local role lookup. |
| Generic gap | `newCommand` has no `ws`/`wsa` cases; `saturnCommand.Execute` handles them under the remote-message generic branch. | Add only concrete `ws` and `wsa` handler cases. |
| Registration | `RegisterUserUtilitiesWithDirectAgent` does not list either canonical. | Register only behind the new narrow relay capability. |
| Existing replica ownership | `cmd/zenbot/main.go` creates one `core.ReplicaManager`; `EngineImpl.SetReplicaController` installs a `ManagedReplicaController`. `ReplicaManager.ManagedEngines` snapshots managed engines by channel. | Reuse this live managed-replica inventory; do not create, stop, reconnect, or mutate replicas. |
| Sending | `core.ManagedEngine` includes `common.Engine`; `EngineImpl.SendChatMessage` emits an outbound chat payload and returns transport errors. | The relay sends once through the existing managed `support` engine; there is no queue/flush API to reproduce. Immediate send is the target equivalent of Saturn queue + `shareMessages()`. |
| Existing configuration | `config.Config.AdminTrips` is already passed into main/factory composition. | Preserve the source formatting distinction only through that existing config list; do not infer it from H2 ADMIN roles or introduce a new credential/permission source. |

## Proposed architecture

### Narrow capability and ownership

Add a small command-facing contract under `internal/common`, suggested shape:

```go
type SupportRelayRequest struct {
    Author    string
    Trip      string
    Arguments []string // excludes command alias
    Anonymous bool
}

type SupportReplicaRelay interface {
    RelayToSupport(context.Context, SupportRelayRequest) error
}
```

**Recommended ownership:** a composition-installed implementation on the master `*core.EngineImpl`, or a small core-owned adapter it exposes. It receives immutable configured admin trips at construction and the already installed replica controller/manager. It must:

1. reject a cancelled context before any lookup/send;
2. look up only the managed engine keyed exactly `"support"`;
3. return a non-nil unavailable error if there is no managed support engine; do not create a replica, initiate a WebSocket connection, use a temporary session, or reply;
4. compute configured-admin status using the exact existing `Config.AdminTrips` values (case-sensitive membership unless a source audit shows otherwise);
5. render the source argument sequence: admin preserves special characters; regular caller strips everything except ASCII `A-Z`, `a-z`, `0-9`, and space; retain the source trailing space;
6. apply a dedicated Java-string escaping helper compatible with `StringEscapeUtils.escapeJava` before building the route-specific prefix;
7. send publicly through the support engine with no author-addressing and no whisper; return the actual send error.

The new relay must not access H2 authorization, the agent gateway, private agent tools, proxy configuration, raw `*sql.DB`, temporary snapshot sessions, or credentials. Existing dispatch authorization remains the only access gate; the source-config trip list is used exclusively for the observed text-sanitization branch.

### Command handlers and registration

Create `internal/command/support_relay.go` with two concrete command types (or one parameterized type):

- canonical `ws` routes with `Anonymous: false`;
- canonical `wsa` routes with `Anonymous: true`.

Each handler must check `ctx.Err`, type-assert `common.SupportReplicaRelay`, pass `args(c.message)`, `Name`, and `Trip`, return `FAILED,error` on absent capability/relay failure, and otherwise return `SUCCESSFUL` **without `reply`**.

In `internal/command/handlers.go:newCommand`, route only `ws` and `wsa` to these types. In `internal/command/dispatch_adapter.go`, append both canonicals only if `e` implements `common.SupportReplicaRelay`. The aliases arrive through their catalog definitions; do not manually create alias registrations.

Delete `ws` and `wsa` from the `saturnCommand.Execute` generic remote-message branch. Keep `msgchannel` there: only its same-room branch is concrete today, and remote delivery is not part of this slice.

### Composition

At `cmd/zenbot/main.go`, create the relay after the replica manager/controller exists and before `RegisterUserUtilitiesWithDirectAgent`. Install it on the master using the existing `Config.AdminTrips`; it must refer to the one manager already owned by main. Keep `ReplicaFactory`, generic replica commands, transport config, and process lifecycle unchanged.

The engine need not manufacture a support replica. Operators obtain it through the already accepted `!replica support` lifecycle path. Its absence is a normal disabled-capability/operation failure, not a reason to add configuration, identity, credentials, retries, or autonomous lifecycle behavior.

### Target sequence

```text
regular caller: !ws hello! <tag>
  -> DispatchUserCommand resolves active caller and verifies USER
  -> capability-gated ws legacy adapter
  -> supportRelayCommand.Execute
  -> SupportReplicaRelay.RelayToSupport
       -> find existing managed "support" replica
       -> regular rendering: "hello tag "
       -> Java escape
       -> support.SendChatMessage("", "caller: hello tag ", false)
  -> SUCCESSFUL; no response in caller room

configured admin caller: !wsa hello! <tag>
  -> same shared dispatch
  -> configured-admin rendering preserves special characters
  -> support.SendChatMessage("", "anon_from_hc: hello! <tag> ", false)
  -> SUCCESSFUL; no caller-room response
```

## Exact exclusions and blockers

- Do not register or implement ADMIN `whiskey`; it is proxy-replica work and remains capability-blocked.
- Do not implement remote `msgchannel`/`msgroom`; this slice must not add generated nicks, host-password copying, credentialed joins, or session-factory request changes.
- Do not create a missing `support` replica, change generic replica construction, or add reconnect/backup policy.
- Do not use agent execution, expose these USER relay aliases to `AgentCommandGateway`, or broaden `publicAgentCommandAliases`.
- Do not make H2 ADMIN role substitute for source `AdminTrips` formatting behavior, and do not add a new access control list.
- Do not alter `mine`, `restart`, `shutdown`, `sql`, proxy configuration, transport dialers, snapshot coordination, schema, persistence, lifecycle, or any accepted vertical.

## File map

| Path | Change |
|---|---|
| `internal/common/support_relay.go` (new) | Narrow `SupportReplicaRelay` request/capability contract only. |
| `internal/core/support_relay.go` (new) | Existing-managed-`support` lookup, source rendering/escaping, one outbound public send. |
| `internal/core/engine_impl.go` | Minimal capability installation/delegation only, if needed; no replica/lifecycle rewrite. |
| `internal/factory/engine_factory.go` or `cmd/zenbot/main.go` | Install the relay once with existing `AdminTrips` and manager. Prefer main if the manager remains composition-local. |
| `internal/command/support_relay.go` (new) | Concrete `ws`/`wsa` command handlers. |
| `internal/command/handlers.go` | Two constructor cases only. |
| `internal/command/dispatch_adapter.go` | Conditional `ws`, `wsa` registration only. |
| `internal/command/registry.go` | Remove only `ws`/`wsa` generic behavior after concrete tests pass. |
| Focused `*_test.go` files | New command/core/composition tests listed below. |

No repository, service, H2, schema, migration, config-file-format, proxy, agent, snapshot, or transport production file belongs in the initial implementation map.

## Strict TDD delivery plan

Implement one RED→GREEN tracer at a time; do not prewrite all production code or expose aliases before the capability exists.

1. **RED — capability-gated registration and concrete construction**
   - Add `TestSupportRelayAliasesAreUnregisteredWithoutCapability` in `internal/command/support_relay_test.go`.
   - Assert a plain engine and a generic `ReplicaController` alone expose none of `ws`, `wsay`, `wsa`, `wsayanon`, `anonsay`.
   - With a recording `SupportReplicaRelay`, assert every exact alias is present, has USER role, and resolves to a concrete handler rather than `*saturnCommand`.
   - Run `go test ./internal/command -run '^TestSupportRelayAliases' -count=1`; expected RED is absent registration/concrete routing.
   - GREEN only the contract, two handler cases, and conditional adapter registration.

2. **RED — command forwarding/no-reply contract**
   - Add direct command tests for every `ws` and `wsa` alias: first command token is excluded; all remaining parsed tokens are forwarded; success produces one relay request and zero caller replies, including inbound whisper.
   - Add pre-cancelled and absent-capability cases: `FAILED`, non-nil error, zero relay requests, zero caller replies.
   - Verify relay error produces `FAILED`, no fabricated success/reply.
   - GREEN only the thin command delegation.

3. **RED — core relay source semantics**
   - Add `internal/core/support_relay_test.go` with a controlled managed-support sender and fixed configured `AdminTrips`.
   - Assert configured-admin input preserves punctuation, while non-admin strips `!`, punctuation, emoji, and markup characters according to `UserCommandBaseImpl.renderArguments`; both retain the observed trailing space.
   - Assert `ws` output is `author + ": " + escaped`; anonymous output is `anon_from_hc: ` plus the escaped body; each sends public/unaddressed text exactly once.
   - Characterize Java escaping before Green with representative quote, backslash, newline, tab, and control-character vectors. Do not assume `strconv.Quote` is fully equivalent without those assertions; implement a small compatible helper if needed.
   - Assert missing support, send failure, and cancelled context produce an error with no send and no replica mutation.
   - GREEN only the relay implementation.

4. **RED — composition and existing-replica integration**
   - Add a focused main/factory composition test proving one master relay uses the existing manager and becomes usable only after a managed `support` engine is present.
   - Add an integration test with a fake managed support engine or local test transport: dispatch `!ws` and `!wsa` through `listener.NewUserChatListener`; assert normal USER authorization precedes relay invocation, support receives the exact outbound body, and caller room receives no message.
   - Include denied-user proof: no relay invocation and existing dispatch denial response only.
   - GREEN the smallest installation seam. Do not start real remote/proxy sessions.

5. **Regression gates**
   - Re-run existing generic replica and runtime failure tests to prove this lookup-only relay did not change lifecycle behavior.
   - Run command/core/factory/main packages and then the full suite.

### Required verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation:

```sh
gofmt -w internal/common/support_relay.go internal/core/support_relay.go internal/core/engine_impl.go internal/command/support_relay.go internal/command/support_relay_test.go internal/core/support_relay_test.go internal/command/handlers.go internal/command/dispatch_adapter.go internal/command/registry.go

go test ./internal/command -run 'TestSupportRelay|TestReplica' -count=1
go test ./internal/core -run 'TestSupportRelay|TestReplicaManager' -count=1
go test ./internal/factory ./cmd/zenbot -count=1
go test ./...
git diff --check
git diff --cached --check
git status --short
```

Acceptance requires focused RED records followed by green reruns, exact alias/role/registration evidence, source-shaped rendering and escaping coverage, normal-dispatch authorization proof, no-reply caller behavior, missing-support fail-closed behavior, no replica mutation, and a clean diff check. No staging, reset, clean, restore, checkout, commit, or Saturn-source change is part of this vertical.

## Baseline / evidence record

**[TEST-BACKED]** Before this handoff, the read-only baseline passed:

```text
go test ./internal/command ./internal/core ./internal/factory ./cmd/zenbot -count=1
ok  zenbot/internal/command
ok  zenbot/internal/core
ok  zenbot/internal/factory
ok  zenbot/cmd/zenbot

git diff --check
git diff --cached --check
# both exited 0 with no output
```

The worktree was already deliberately dirty with accepted DBZ/access and other migration work. This architecture pass changes only `.hermes/handoffs/next-core-command-architecture.md`.
