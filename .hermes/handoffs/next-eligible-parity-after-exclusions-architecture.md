# Next eligible Saturn → Zenbot parity slice after exclusions: remote `msgchannel` / `msgroom`

**Decision:** select the **remote-room branch** of Saturn user `MsgChannelCommandImpl`—only when the normalized requested room differs from the serving room. It is the smallest remaining eligible vertical after excluding every mining (`mine`) and Whiskey/proxy/`whiskey` vertical.

This is a new branch of an already accepted *local* command, not a claim that all of `msgchannel` is accepted. It must use the existing temporary snapshot lifecycle, but it must not require or activate mining, a proxy, Whiskey, an AGENT replica, a scheduler, SQL, lifecycle restart/shutdown, H2 work, or agent routing.

## 1. Verified inventory: accepted vs. unaccepted, blocked, and deferred

| Scope | Current disposition | Evidence |
|---|---|---|
| Local `msgchannel` / `msgroom` | **Accepted, bounded local-room branch only.** Concrete USER route, local formatting, authorization, failure cases, and explicit remote rejection are covered. | `.hermes/handoffs/local-msgchannel-parity-qa.md:5-17,19-39`; `internal/command/msg_channel.go:14-40`; `internal/command/msg_channel_test.go:13-253`. |
| Support relay `ws` / `wsa` aliases | **Accepted, bounded.** | `.hermes/handoffs/support-relay-command-parity-qa.md:3-26,28-66`; `internal/command/support_relay.go:11-34`. |
| Replica lifecycle, AutoMove, nuke, resurrect | **Accepted only within each named vertical.** Do not reopen as prerequisites. | `.hermes/handoffs/replica-qa.md:3-14`; `.hermes/handoffs/automove-final-qa.md:3-24`; `.hermes/handoffs/resurrect-qa.md:3-16`. |
| SQL/restart/shutdown concrete command paths | **Functionality exists and current focused baseline is green, but full lifecycle acceptance and historic strict-TDD provenance are not established.** The lifecycle-completion record stopped because tracer 4 was already green; it requires an explicit clean rebaseline or dirty-safe manual reduction before a new strict tracer sequence. | `internal/command/sql.go:11-42`; `internal/command/restart_shutdown.go:23-57`; `.hermes/handoffs/sql-restart-shutdown-lifecycle-completion-implementation.md:110-131`. |
| Remote `msgchannel` / `msgroom` | **Unaccepted and unimplemented.** Current remote input returns `FAILED` with `remote room delivery is not configured`, no output. | `internal/command/msg_channel.go:28-31`; `internal/command/msg_channel_test.go:162-172`; `.hermes/handoffs/local-msgchannel-parity-qa.md:5,12-13,39`. |
| Remote `list` | **Unaccepted/deferred.** It needs the same credentialed temporary-session substrate plus its own formatting operation, so it is larger than the selected remote-message operation. | `internal/command/handlers.go:152-168`; Saturn `src/main/java/org/saturn/app/command/impl/user/ListUserCommandImpl.java:41-88`. |
| `mine` | **Excluded by current user directive.** No prerequisite or preparatory work is selected. | `internal/command/admin_moderator_catalog_guard_test.go:113-119`; `.hermes/handoffs/mine-current-architecture.md:3-6,160-184`. |
| Whiskey/proxy/`whiskey` | **Excluded by current user directive.** No proxy configuration, dialer, AGENT replica, or whiskey-command work is selected. | `internal/command/admin_moderator_catalog_guard_test.go:113-119`; `.hermes/handoffs/whiskey-current-architecture.md:3-7,69-100,131-133`. |

**Important provenance boundary:** current green tests, concrete source, and a working runtime path establish current behavior only. They do **not** establish a historical one-at-a-time RED→immediate-GREEN sequence for lifecycle work. This specification neither accepts lifecycle parity nor asks an implementer to reconstruct history.

## 2. Evidence that mining and Whiskey were excluded

1. The live admin/moderator catalog guard permits only `mine` and `whiskey` to remain generic: `allowedScopedGenericFallbacks = {"mine", "whiskey"}` (`internal/command/admin_moderator_catalog_guard_test.go:113-136`).
2. `newCommand` has no concrete `mine` or `whiskey` case; `whiskey` remains a controlled generic failure (`internal/command/handlers.go:276-389`; `internal/command/registry.go:163-169`).
3. The selected source class is user `MsgChannelCommandImpl`, not `MineTripCommandImpl`, `WhiskeyReplicaCommandImpl`, `WhiskeySayUserCommandImpl`, or `WhiskeyAnonUserCommandImpl` (`src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java:23-107`).
4. This slice must not add a proxy field, proxy dialer, Whiskey capability, AGENT replica construction, mine scheduler, mined-trip file, mine credentials, or mine registration. The only temporary credentials are the source-required host-password join credentials for this one `msgchannel` remote session; they are neither mining nor a proxy mechanism.

## 3. Candidate ranking from current source and live handoffs

| Rank | Candidate | Decision | Current evidence and reason |
|---:|---|---|---|
| 1 | **Remote `msgchannel` / `msgroom`** | **Select.** | One unaccepted branch of one already live USER command. Its local parser/rendering and dispatcher authorization are already concrete. The missing work is a credentialed target-room temporary join and one remote delivery operation. Saturn authority: `MsgChannelCommandImpl.deliverToRemoteRoom` and `DeliverMessageToRoomOperation`. |
| 2 | Remote `list` | Defer behind selected slice. | It duplicates the credentialed temporary-session gap and additionally needs Saturn list sorting/dedup/rendering in a new operation. It is not smaller. Saturn `ListUserCommandImpl.joinChannel/printUsers:63-111`. |
| 3 | Lifecycle completion | Reject for this next slice. | A functional-baseline/TDD-provenance decision remains required by the live lifecycle implementation record. It is materially broader (fresh master graph, rebinding, exclusive ownership) and not eligible as a strict next tracer without that decision. |
| 4 | `mine` | Reject: explicit exclusion. | Also requires scheduler, generated credentials, local secret persistence, and a temporary session. |
| 5 | Whiskey/proxy/`whiskey` | Reject: explicit exclusion. | Requires proxy configuration/dialing, custom-named AGENT replicas, readiness, backup ownership, and recovery policy. |

### Rejection of stale recommendations

`.hermes/handoffs/next-core-command-after-memory-reconciliation-architecture.md:1-8,22-27,56-61` concludes there is no eligible command because it treats remote `msgchannel` and credentialed temporary sessions as previously deferred. That is not the current selection authority: the present user directive excludes only mine and Whiskey/proxy verticals, not remote `msgchannel` or credentialed snapshot sessions. Its accepted-command reconciliation remains useful; its blanket remote-message deferral is superseded for this specification.

The older `next-core-command-after-support-relay-architecture.md` is also stale as a *next* selection because its selected local branch is now accepted. Its Saturn source analysis and target seam observations remain valid where confirmed against current source.

## 4. Exact selected vertical

Implement source-equivalent remote delivery for:

```text
!msgchannel <room-not-equal-to-current> <message...>
!msgroom    <room-not-equal-to-current> <message...>
```

The existing local behavior remains unchanged when normalized room equals `Engine.GetChannel()`.

### Saturn behavior to reproduce

**[OBSERVED]** `MsgChannelCommandImpl` declares `msgchannel` and `msgroom`, authorizes at `Role.REGULAR`, removes every `?` from the room then trims it, and joins/trims message tokens (`src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java:23-76`). The existing target has already matched these local parse rules (`internal/command/msg_channel.go:18-35`).

For a remote room, Saturn:

1. generates a UUID workflow ID and nick `msg-<first 8 chars>`;
2. creates a temporary `LIST_CMD` `EngineSnapshotSession` for the **target room**, using that nick and the serving engine password;
3. preserves source channel as the serving engine channel, target channel as the requested remote room, and passes the inbound chat message as the non-null reply message;
4. parses an `onlineSet` users payload;
5. if every snapshot user is `isMe`, replies ` <target> is empty` and sends no raw message;
6. otherwise sends one raw remote payload equivalent to `{"cmd":"chat","nick":"*","text":<formatted body>}` and replies `sent successfully.`;
7. flushes/closes the temporary session; source workflow failure replies `Unable to complete room operation.` because the request has a reply message.

**Citations:** Saturn `MsgChannelCommandImpl.deliverToRemoteRoom:83-105`; `EngineSnapshotSession.create:21-58`; `DeliverMessageToRoomOperation.apply:15-21`; `DefaultRoomSnapshotCoordinator.Workflow.receive/fail:129-188`.

### Proposed end-to-end target sequence

```text
[PROPOSED, source-shaped]
allowed USER !msgroom remote-room hello
  -> existing DispatchUserCommand authorization
  -> existing msgChannelCommand parse/format
  -> normalized room != Engine.GetChannel()
  -> credentialed RoomSnapshotRequest
       SourceChannel = current serving room
       TargetChannel = remote-room
       TemporaryJoin = {Channel: remote-room, Nick: msg-<8>, Password: host password}
       ReplyMessage = non-empty (enables source generic failure reply)
  -> existing coordinator registers one temporary session
  -> session sends join for remote-room as msg-...#<host password>
  -> first correlated onlineSet snapshot
       all isMe -> addressed ` remote-room is empty`; no raw chat
       otherwise -> one raw {cmd:chat,nick:*,text:<source-formatted-body>}
                      addressed `sent successfully.`
  -> flush, close, registry removal; late payload ignored
```

## 5. Target gap and exact source-to-target mapping

| Saturn symbol / behavior | Current Zenbot seam | Required target change |
|---|---|---|
| `MsgChannelCommandImpl.deliverToRemoteRoom` | `internal/command/msg_channel.go:28-31` returns a controlled unavailable error. | Replace only the remote failure branch with capability-gated workflow submission. Keep the existing local branch byte-for-byte behaviorally unchanged. |
| `EngineSnapshotSession.create(id, config, room, nick, password, sink)` | `internal/listener/snapshot/session_factory.go:86-115,150-161` creates a session that joins only `req.SourceChannel` and emits no nick/password. | Add a narrowly typed optional temporary-join descriptor to `RoomSnapshotRequest`, including target join channel, nick, and password. With descriptor absent, retain the exact existing credential-free `SourceChannel` join for existing workflows. With descriptor present, join descriptor channel and encode `nick#password` safely. |
| Serving host password | `config.Config.Password` is composed into `core.EngineImpl.Password` (`internal/config/config.go:10-25`; `internal/core/engine_impl.go:48-55`; `internal/factory/engine_factory.go:59-62`). `common.Engine` intentionally has no password getter (`internal/common/engine.go:5-57`). | Add a narrow capability owned by the snapshot/master boundary—not an unrestricted password getter on `common.Engine`—that supplies a credentialed snapshot submission or one-shot join material. It must fail closed if unavailable. The command must not log, reply with, persist, or otherwise expose the password. |
| `DeliverMessageToRoomOperation.apply` | `internal/listener/snapshot` has Nuke and Kick/Resurrect operations (`nuke_operation.go`, `kick_or_resurrect_operation.go`) and coordinator outcome/reply/cleanup (`coordinator.go:145-325`). | Add `DeliverMessageToRoomOperation` (Go name may be `RemoteMessageOperation`) with source exact empty predicate, JSON-encoded raw chat, and exact `Empty(" "+target+" is empty")` / `Success("sent successfully.")` results. Do not use a permanent engine or an addressed `SendChatMessage` for the remote payload. |
| Source failure fallback | Coordinator only sends generic failure if `ReplyMessage != ""` (`coordinator.go:273-295`). | Set remote message request `ReplyMessage` non-empty. This makes session/parser/transport/operation error reply `Unable to complete room operation.` through the existing reply sink and original whisper bit. |
| Current parser / authorization | `msgChannelCommand` and normal registration are concrete; `RegisterUserUtilitiesWithDirectAgent` registers `msgchannel` unconditionally (`handlers.go:379-380`; `dispatch_adapter.go:57-64`; `msg_channel_test.go:13-80`). | Preserve aliases, USER metadata, normal dispatcher authorization, local command tests, and capability-free local path. Do not change catalog registration merely because remote capability is absent. A remote direct call without required capability must fail closed with no raw/chat side effect beyond source-required failure behavior only after a request was accepted. |

## 6. Scope exclusions

- Do not implement or prepare `mine`, any mining scheduler, mined credential sink, mine count/start/stop behavior, or mine registration.
- Do not implement Whiskey, proxy configuration, proxy dialing, `WhiskeyProxyOrder`, custom-named AGENT replicas, proxy probing, backup proxy lifecycle, or any `whiskey` registration.
- Do not modify accepted local `msgchannel` rendering/parsing, support relay, generic replica lifecycle, nuke, resurrect, AutoMove, DBZ, access, SQL, restart, shutdown, lifecycle composition, agent runtime, listener ordering, or H2 schema/repository/service contracts.
- Do not expose the master password in logs, replies, tests outside a fake fixture, H2, metrics, command audit, or public interfaces. Tests must inspect only serialized fake join payloads with test-only values.
- Do not turn remote delivery into a local-room fallback, an acknowledgement-only no-op, a permanent replica, a proxy connection, a retry loop, or a new delivery protocol.
- Do not modify Saturn source, existing handoffs, migration/audit files, Go source/tests/config in this architecture task, or unrelated dirty files.

## 7. File map for the implementation slice

| Path | Intended responsibility |
|---|---|
| `internal/command/msg_channel.go` | Retain local branch; construct and submit a source-shaped remote request only when normalized room differs. Generate workflow/nick through injectable seams; return `FAILED` for direct cancellation/capability/submission errors. |
| `internal/command/msg_channel_test.go` | Add sequential remote-branch tracers while retaining local acceptance regressions. |
| `internal/common/room_snapshot.go` **or new narrow adjacent common file** | Define the minimal credentialed snapshot submission capability/request boundary. Do not add password access to broad `common.Engine`. |
| `internal/core/room_snapshot.go` and `internal/core/engine_impl.go` | Implement the narrow master-owned capability by routing to its existing coordinator and sourcing the already composed host password without exposing it elsewhere. |
| `internal/listener/snapshot/coordinator.go` | Extend `RoomSnapshotRequest` only with an optional typed join descriptor; preserve request validation, correlation, terminal cleanup, replies, and outcome behavior. |
| `internal/listener/snapshot/session_factory.go` | Select source-channel credential-free join for legacy requests, descriptor target/nick/password join for this vertical; preserve registry close-once and error routing. |
| `internal/listener/snapshot/remote_message_operation.go` **new** | Source-shaped empty check and exactly one raw remote chat payload; return source-shaped operation result. |
| `internal/listener/snapshot/*_test.go` | Isolated join serialization/credential non-leak tests, operation tests, and coordinator cleanup/late-payload regressions. |
| `cmd/zenbot/main.go` and/or `internal/factory/engine_factory.go` | Inspect-only unless actual composition proves the master already exposes the capability. If a narrow installation is necessary, compose it only on the master; do not add it to replicas or sessions. |

No `internal/config/**`, `internal/transport/**`, `internal/repository/**`, `internal/service/**`, SQL/schema, proxy, mining, Whiskey, or agent file is planned.

## 8. Complexity, risk, and routing recommendation

**Complexity:** medium. The command/parser/local formatter already exists; the change crosses command → master capability → snapshot request/session → operation → coordinator reply/cleanup. It is smaller than remote `list` because it has one raw operation and no remote user rendering.

**Risks and mitigations:**

1. **Credential leak (high):** make join material an unexported/typed request field consumed only by the temporary session factory; redact it from errors and avoid `%+v` request logging. Test payload serialization with fake password but no production logging.
2. **Wrong room joined (high):** current factory joins `SourceChannel` (`session_factory.go:98,154-159`) while Saturn remote message joins target. The optional descriptor must override only this new request; nuke/resurrect regressions must prove their old request shape/join behavior is unchanged.
3. **Premature success or duplicate send (high):** successful command submission is not delivery completion. Preserve coordinator first-snapshot correlation, one operation application, flush/close, and late-event rejection (`coordinator.go:225-271`).
4. **Source-visible replies (medium):** preserve empty, success, and generic failure strings and original whisper routing. Do not use a generic local reply as the remote chat payload.
5. **Dirty-tree/TDD provenance (medium):** route to an implementer able to make a clean, strictly sequential tracer record. Do not touch lifecycle artifacts that need a separate functional-baseline decision.

**Routing recommendation:** assign this to a command/snapshot implementer with authority over `internal/command`, `internal/common`, `internal/core`, and `internal/listener/snapshot`, followed by independent QA. It is independent of the blocked lifecycle-provenance decision and contains no excluded dependency.

## 9. Strict one-at-a-time RED → immediate GREEN tracer plan

Do **not** add the next tracer or production change before the current named test has been run RED, then made minimally GREEN, and its focused package test has passed. Fakes only; no live credentials, endpoint, proxy, mining, or process action.

1. **RED — remote command boundary and capability gate**
   - Add a remote-only test proving a capability-bearing command submits exactly one request for `!msgroom other hello`, while an engine lacking the new narrow capability returns `FAILED` and produces no raw/chat/session side effect.
   - Assert existing local `msgroom programming hello` remains direct local delivery without snapshot submission.
   - Run: `go test ./internal/command -run '^TestMsgChannelRemote(Capability|KeepsLocalPath)' -count=1`.
   - **GREEN:** add only the narrow capability lookup and remote request construction; do not yet add credentialed transport or operation behavior.

2. **RED — request identity and source-shaped command semantics**
   - Assert both aliases preserve USER registration/dispatcher authorization; remote request uses serving source room, requested target, original whisper routing, non-empty failure-enabling reply marker, generated `msg-<8>` nick, and source formatted body including the `![](` variant.
   - Assert direct cancellation produces `FAILED, context.Canceled`, no request, and no output; submission error returns failure without fabricated success.
   - Run: `go test ./internal/command -run '^TestMsgChannelRemote(Request|Cancellation|SubmissionFailure|Authorization)' -count=1`.
   - **GREEN:** add only deterministic generator injection/request mapping and error handling.

3. **RED — credentialed temporary join, legacy non-regression**
   - Add session-factory tests proving a remote-message descriptor serializes exactly one JSON join for the remote target with `nick` equal to `msg-...#test-password`; values requiring JSON escaping must stay valid JSON.
   - In the same test file, prove existing descriptor-free nuke/resurrect-style request still joins its legacy `SourceChannel` and contains no `nick` field.
   - Run: `go test ./internal/listener/snapshot -run '^TestTransportSession(CredentialedRemoteMessageJoin|LegacyJoinRemainsCredentialFree)$' -count=1`.
   - **GREEN:** add only typed optional join material and serialization selection. No proxy/dialer/config work.

4. **RED — remote delivery operation**
   - `all isMe` snapshot: result exactly `EMPTY` / `" <target> is empty"`, zero raw sends.
   - non-empty snapshot: one JSON raw `chat` with nick `*` and source-formatted body; result exactly `SUCCESS` / `sent successfully.`.
   - raw-send error: operation returns error/failed result so coordinator routes generic failure; no retry or second raw send.
   - Run: `go test ./internal/listener/snapshot -run '^TestRemoteMessageOperation' -count=1`.
   - **GREEN:** add the operation only, using `json.Marshal` rather than manual JSON.

5. **RED — coordinator lifecycle and user-visible replies**
   - With fake session/coordinator/reply sink, prove remote success produces one `sent successfully.` reply after raw send; all-is-me produces exact empty reply; parser/start/transport error produces exactly `Unable to complete room operation.` because request has a reply marker.
   - Prove flush/close/registry removal happen once and duplicate/late `onlineSet` events produce no second raw or reply.
   - Run: `go test ./internal/listener/snapshot -run '^TestRemoteMsgChannelWorkflow' -count=1`.
   - **GREEN:** only integration glue required by observed failures.

6. **RED — master composition and real in-process transport regression**
   - Use an in-process WebSocket/fake transport equivalent to `TestRealCoordinatedSessionUsesWebSocketAndCoordinatorSink`: verify target-room credentialed join, correlated `onlineSet`, one remote raw chat, author-facing success preserving whisper, and terminal cleanup.
   - Assert master exposes the capability while an ordinary command stub/replica/temporary session does not.
   - Run: `go test ./cmd/zenbot ./internal/core ./internal/command ./internal/listener/snapshot -run 'Test.*RemoteMsg(Channel|Room)|TestReal.*RemoteMessage' -count=1`.
   - **GREEN:** minimal composition only if current master wiring cannot already supply the capability.

After each GREEN, rerun the preceding focused tests. Never mark remote `list`, mine, Whiskey, proxy, SQL, or lifecycle work as a dependency or incidental completion.

## 10. Final verification and acceptance conditions

Run from `/Users/ab/workspace/go-projects/zenbot` after the final Green; retain actual output in implementation and QA handoffs:

```text
gofmt -w <only implementation-owned Go files>
go test ./internal/command -run 'TestMsgChannel|TestSupportRelay|TestReplica' -count=1
go test ./internal/listener/snapshot -run 'Test(RemoteMessage|RemoteMsgChannel|TransportSession|Coordinator|RealCoordinatedSession)' -count=1
go test -race ./internal/command ./internal/core ./internal/listener/snapshot ./cmd/zenbot -count=1
go test ./...
go vet ./...
go build ./...
git diff --check
git diff --cached --check
git status --short
```

Acceptance requires all of the following:

1. Both aliases remain one USER canonical command and normal dispatcher authorization remains before remote submission.
2. Existing accepted local behavior remains unchanged and proves no snapshot submission.
3. Remote input no longer returns the old controlled-unavailable error when the master has the required capability; it creates one source-shaped credentialed target-room workflow.
4. The password exists only in the temporary join payload path and test fakes; it is absent from logs/replies/errors/persistence and broad interfaces.
5. `onlineSet` all-is-me yields exactly ` <target> is empty` and no raw chat; a non-empty snapshot emits exactly one JSON remote chat then `sent successfully.`; workflow failure yields existing `Unable to complete room operation.` behavior.
6. Session start, parse, transport error, timeout, close, and late event cleanup remain single-terminal and do not regress existing credential-free workflows.
7. Tests provide newly observed, sequential RED→immediate-GREEN evidence for each tracer. A final green suite alone is not evidence for historical TDD provenance.
8. No mine, Whiskey/proxy, lifecycle, SQL, H2/schema, agent, or unrelated dirty-tree change is introduced.

## Baseline and command output recorded for this architecture

Read-only inspection was performed on Zenbot branch `migration/saturn-zenbot-parity` and Saturn `develop`; Saturn source was not modified. The Zenbot tree was already dirty before this handoff. No Go source, test, config, migration/audit document, Saturn source, or existing handoff was modified by this task.

```text
$ go test ./internal/command ./internal/listener ./internal/core ./cmd/zenbot -run 'Test(MsgChannel|Replica|AutoMove|Nuke|Resurrect|SQLRestartShutdownFinalCatalogIntegration|HostSupervisor|MainProductionHostLifecycle|HostLifecycle)' -count=1 && go vet ./... && go build ./... && git diff --check && git diff --cached --check
ok  zenbot/internal/command   3.935s
ok  zenbot/internal/listener  0.714s
ok  zenbot/internal/core      1.072s
ok  zenbot/cmd/zenbot         0.380s
# vet/build/diff checks exited 0 with no output

$ go test ./internal/command -run '^TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded$' -count=1
ok  zenbot/internal/command  0.416s
```

The focused baseline demonstrates current build/test health only; it is not a migration-completion assertion and is not historic TDD evidence.
