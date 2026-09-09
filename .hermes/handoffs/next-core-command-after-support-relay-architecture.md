# Next core command vertical after support relay: local `msgchannel` / `msgroom`

## Decision

**Select the source-tested local-room branch of Saturn user `MsgChannelCommandImpl` next.** Implement the exact aliases `msgchannel` and `msgroom` for a target normalized to the currently served room. This is a small concrete command vertical: parse/validate, source-shaped anonymous-mail rendering, one addressed send, normal USER authorization, and no persistence or new runtime capability.

This is deliberately **not** an acceptance claim for Saturn's remote-room branch. Remote delivery requires the source's credential-bearing temporary `LIST_CMD` session and must remain separately unavailable after this slice. The bounded local branch is the smallest remaining source-grounded core command behavior after accepted `ws`/`wsa` support relay that does not require proxy/Whiskey activation, credentials, temporary remote sessions, raw SQL policy, restart/shutdown policy, or a new identity/auth design.

## Current inventory and ranking

`MIGRATION_PLAN.md` gives concrete command behavior priority over catalog placeholders and requires focused evidence per delivered capability (`MIGRATION_PLAN.md:13-25,175-181`). Newer support-relay QA accepts `ws`/`wsay` and `wsa`/`wsayanon`/`anonsay` as a bounded completed vertical (`.hermes/handoffs/support-relay-command-parity-qa.md`). Direct `l`, DBZ, identity/history, moderation, prefix, activity, automove, replica lifecycle, nuke/resurrect, and last-online have their own newer accepted handoffs; they are not candidates for this selection.

| Rank | Remaining behavior | Decision | Source/target reason |
|---|---|---|---|
| 1 | **`msgchannel` / `msgroom`, normalized target equals current room** | **Proceed** | One USER command class; direct Saturn tests prove local delivery and missing-message usage. Zenbot already has aliases, normal authorization, `Engine.GetChannel`, and `SendChatMessage`. No H2, schema, service, proxy, credential, snapshot, or lifecycle change is needed. |
| 2 | `msgchannel` / `msgroom`, remote target | Defer | Saturn creates a temporary `LIST_CMD` session with generated nick and host password. Zenbot's existing snapshot request/session join is credential-free; this is explicitly excluded. |
| 3 | `mine` | Blocked/defer | Requires a process-owned scheduler, generated credentialed temporary sessions, secret file policy, and unresolved proxy disposition; see `.hermes/handoffs/mine-current-architecture.md`. |
| 4 | ADMIN `whiskey` | Blocked/defer | Requires ordered proxy configuration, proxy-capable dialing, named AGENT construction/readiness, and owned backup/recovery policy; see `.hermes/handoffs/whiskey-current-architecture.md`. |
| 5 | `restart` / `shutdown` | Blocked | Requires an approved process/supervisor ownership, teardown, terminal-state, feedback, and authority policy; see `.hermes/handoffs/restart-shutdown-current-architecture.md`. |
| 6 | `sql` | Blocked | Raw SQL disclosure, grammar/read-only enforcement, limits, auditing, delivery, and database-principal policy are undecided; see `.hermes/handoffs/sql-current-architecture.md`. |

No lower-ranked item satisfies the requested exclusions with a smaller source-tested command scope. The selected branch has a stable current-room predicate and does not need to activate the source remote workflow to make that predicate concrete.

## Saturn authority and exact bounded behavior

### Catalog and authorization

**[OBSERVED]** `src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java` declares exactly `msgchannel` and `msgroom`, returns `Role.REGULAR`, and inherits normal `UserCommandBaseImpl` authorization. `UserCommandBaseImpl.execute` resolves the alias and authorizes before calling the concrete command (`src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`).

Zenbot already preserves canonical `msgchannel`, both aliases, and `model.USER` in `internal/command/registry.go:RegisterAll`. Normal inbound dispatch resolves an active author and calls `Engine.IsUserAuthorized` before command execution (`internal/listener/message/handlers.go:ResolveUserMetadata,DispatchUserCommand.Handle`). Reuse that gate; do not add command-local trip, role, database, or agent checks.

### Parse, validation, and source output

**[OBSERVED]** `MsgChannelCommandImpl.execute` receives post-alias whitespace-delimited arguments.

1. Fewer than two arguments (missing room or missing message) sends the invoking author exactly:

   ```text
    Example: <prefix>msgroom your-room your message
   ```

   and returns `FAILED`.
2. It normalizes the first argument by removing **all** `?` characters and trimming. A blank normalized room sends exactly `Room name cannot be blank.` to the invoking author and returns `FAILED`.
3. It joins all remaining arguments with one ASCII space and trims the result.
4. When the normalized room exactly equals `engine.channel`, it sends one addressed message using the source's original whisper flag and returns `SUCCESSFUL`.
5. The local body is:

   ```text
   anonymous mail from: ?<current-channel> message: <rendered-message>
   ```

   unless the rendered message contains the literal substring `![](`, in which case it is:

   ```text
   <rendered-message>\n anonymous mail from: ?<current-channel>
   ```

6. Saturn calls `enqueueMessageForSending(author() + " ", body, isWhisper())`; that deliberate trailing space on the recipient produces two spaces after the addressed name in public output. The target must pass `message.Name + " "` to `Engine.SendChatMessage`, not use the generic `reply` helper, so `EngineImpl.SendChatMessage` preserves the observed recipient-spacing shape (`internal/core/engine_impl.go:SendChatMessage`).

**[TEST-BACKED]** `src/test/java/org/saturn/app/command/impl/user/MsgChannelCommandImplTest.java` proves successful current-room delivery for `*msgchannel programming test message` and the exact usage reply when the body is absent. It does not test question-mark normalization, blank room, markdown-image body, whisper mode, remote behavior, send failures, or authorization; those are source-derived requirements for focused target tests.

### Remote behavior: explicitly not selected

**[OBSERVED]** A source target different from `engine.channel` invokes `deliverToRemoteRoom`: it creates a UUID workflow, generated `msg-<8-char>` nick, `EngineSnapshotSession` with the host `engine.password`, a `LIST_CMD` online-set parser, and `DeliverMessageToRoomOperation` (`MsgChannelCommandImpl.deliverToRemoteRoom`). `UserCommandBaseImpl.setupEngine` shows the shared temporary-session pattern copies the host password.

**[LIMITATION / target gap]** Zenbot's current generic fallback implements only a same-room direct send and returns `remote room delivery is not configured` otherwise (`internal/command/registry.go:saturnCommand.Execute`). `internal/listener/snapshot/RoomSnapshotRequest` and the session factory are existing remote-workflow seams, but current temporary joins are credential-free. This vertical must preserve the controlled target failure for a remote normalized target: `FAILED`, non-nil error, and no outbound chat. It must not claim source remote success or construct a session.

## Current target gap and boundaries

| Concern | Observed Zenbot seam | Required bounded change |
|---|---|---|
| Catalog | `internal/command/registry.go:RegisterAll` | Retain canonical/aliases/USER metadata exactly; add no command definition. |
| Current behavior | `internal/command/registry.go:saturnCommand.Execute` | The generic code parses via `ParseMsgChannel`, sends unformatted text locally, and is not live-registered. Remove only this `msgchannel` case after concrete coverage exists. |
| Concrete construction | `internal/command/handlers.go:newCommand` | Add only canonical `msgchannel` -> a dedicated `msgChannelCommand`. |
| Live registration | `internal/command/dispatch_adapter.go:RegisterUserUtilitiesWithDirectAgent` | Add `msgchannel` to the existing concrete USER canonical list. It needs no optional external capability beyond `common.Engine`; do not register a generic fallback. |
| Parse helper | `internal/command/replica.go:ParseMsgChannel` | It removes only one leading `?`, does not trim the body, and returns errors rather than source replies. Either replace it only after preserving existing helper callers/tests, or add a command-owned parser that removes all `?`, trims the joined body, and carries the two source-visible validation branches. |
| Delivery | `internal/common/engine.go:Engine.SendChatMessage`; `internal/core/engine_impl.go:SendChatMessage` | Send once with `author == message.Name+" "`, source body, and inbound whisper. Propagate a send error as `FAILED`; do not fabricate success/retry. |
| Authorization | `internal/listener/message/handlers.go:DispatchUserCommand.Handle` | Existing USER authorization remains before command execution. |

No repository, service, H2, schema, migration, config, agent, transport, proxy, snapshot, replica-manager, or lifecycle production file belongs in this local branch.

## Proposed target sequence

```text
allowed user: !msgroom ?programming hello world
  -> ResolveUserMetadata / DispatchUserCommand
  -> USER authorization
  -> registered legacyAdapter -> msgChannelCommand.Execute
  -> normalize "?programming" => "programming"
  -> target equals Engine.GetChannel()
  -> body "anonymous mail from: ?programming message: hello world"
  -> Engine.SendChatMessage("alice ", body, inboundWhisper)
  -> SUCCESSFUL; exactly one local outbound chat

allowed user: !msgchannel other-room hello
  -> same parse
  -> target differs from Engine.GetChannel()
  -> FAILED + controlled unavailable error; zero sends
  -> no temporary session, generated nick, password, proxy, or snapshot operation
```

For a missing/blank local request, the handler sends the source-shaped addressed usage/blank-room reply and returns `FAILED`. A cancelled direct context returns `FAILED` with `ctx.Err()` before parse/send; legacy inbound compatibility still supplies its established context through `legacyAdapter`.

## Strict one-at-a-time RED -> GREEN tracer sequence

Do not prewrite all production work. Add and run each tracer before its matching minimal Green change.

1. **RED — concrete routing and registration**
   - Add `internal/command/msg_channel_test.go` cases proving `msgchannel` and `msgroom` are registered by `RegisterUserUtilities`, retain USER role, and construct `*msgChannelCommand`, not `*saturnCommand`.
   - Prove a normal inbound allowed author reaches the concrete handler and an unauthorized author has zero sends and only the existing dispatch denial.
   - Run: `go test ./internal/command -run '^TestMsgChannel(Registration|DispatchAuthorization)' -count=1`.
   - GREEN only the new command type, constructor switch, and one concrete registration entry.

2. **RED — exact local parse/render/delivery**
   - Table-test both aliases; all-`?` removal plus trim; exact current-room equality; joined-and-trimmed text; ordinary body and literal `![](` body; original whisper value; and the recipient trailing-space contract.
   - Assert success means exactly one `SendChatMessage("alice ", body, whisper)` call and no raw send/session/replica mutation.
   - Assert source direct test vectors: local `programming test message` and missing body's exact ` Example: !msgroom your-room your message` reply/status.
   - GREEN only parser/render helper and local command execution.

3. **RED — source failure and controlled exclusion semantics**
   - Missing room/body: exact usage reply, `FAILED`, no local delivery.
   - `? ?`/all-question-mark room: exact `Room name cannot be blank.` reply, `FAILED`, no delivery.
   - Remote normalized target: `FAILED` plus non-nil controlled error, zero sends. Record this as intentionally incomplete remote parity, not a source-compatible remote result.
   - Pre-cancelled context and send error: `FAILED`, returned error, no fabricated reply/retry; cancellation makes zero sends.
   - GREEN only failure/error handling necessary for those tests.

4. **RED — listener integration regression**
   - Dispatch JSON through `listener.NewUserChatListener` with an active authorized USER and each alias. Assert exact addressed local payload shape, including public versus whisper selection.
   - Dispatch as a denied resolved user. Assert no command send and only existing authorization denial.
   - GREEN only if the existing registration/adapter seam needs a minimal fix; do not change listener ordering or authorization.

5. **Regression gates**
   - Re-run `TestMsgAndWhiskeyParsing` if `ParseMsgChannel` changes; retain generic replica, support-relay, and listener authorization tests.
   - Do not activate remote delivery just to make a remote test green.

## Exact implementation file map

| Path | Intended change |
|---|---|
| `internal/command/msg_channel.go` (new) | Concrete local command, source-shaped parse/render, one addressed send, and controlled remote-unavailable error. |
| `internal/command/msg_channel_test.go` (new) | Tracers above: aliases, construction, local behavior, exact failures, cancellation/send failure, and listener authorization integration. |
| `internal/command/handlers.go` | Add exactly `case "msgchannel"` returning the concrete type. |
| `internal/command/dispatch_adapter.go` | Add exactly canonical `msgchannel` to concrete user registration. |
| `internal/command/registry.go` | Remove only generic `case "msgchannel"` after the concrete tests pass; preserve catalog definition. |
| `internal/command/replica.go` and `internal/command/replica_test.go` | Change only if the existing exported `ParseMsgChannel` is deliberately retained as the command parser; otherwise leave it and its current parsing test unchanged. |
| `internal/command/dispatch_integration_test.go` or new command test | May hold an end-to-end registration assertion if that existing fixture is the least invasive fit. |

`internal/listener/message/handlers.go`, `internal/common/engine.go`, and `internal/core/engine_impl.go` are inspected constraints, not planned production edits. `internal/listener/snapshot/**`, `internal/transport/**`, `internal/config/**`, `internal/repository/**`, `internal/service/**`, `internal/agent/**`, `cmd/zenbot/main.go`, and Saturn are out of scope.

## Exclusions and invariants

- Do not implement remote `msgchannel`/`msgroom`, generated nicknames, copied host passwords, credentialed joins, `LIST_CMD`, snapshot submission, remote delivery operation, temporary session cleanup, or a new room capability.
- Do not activate proxy/Whiskey, `mine`, raw SQL, restart/shutdown, replica lifecycle, agent routes/tools, H2/schema/repository/service work, or identity/auth redesign.
- Keep standard USER authorization solely in normal dispatch; do not use configured admin trips, H2 roles, an ACL, or source `getAdminAndUserTrips` as a new local branch.
- Do not reinterpret a remote failure as successful anonymous mail, send an invoking-room acknowledgement for remote failure, or create a no-op handler.
- Preserve source body markers and spacing exactly for local behavior. In particular, do not sanitize content, add a recipient `@` manually, remove the deliberate recipient trailing space, normalize channel case, or substitute `SendAddressedMessage`/generic `reply`.
- Do not alter accepted support-relay behavior or its capability gate.

## Verification required after implementation

Run from `/Users/ab/workspace/go-projects/zenbot`:

```sh
gofmt -w internal/command/msg_channel.go internal/command/msg_channel_test.go internal/command/handlers.go internal/command/dispatch_adapter.go internal/command/registry.go

go test ./internal/command -run 'TestMsgChannel|TestMsgAndWhiskeyParsing|TestSupportRelay|TestReplica' -count=1
go test ./internal/listener/message ./internal/listener -run 'Test.*(Dispatch|Authorization|UserChat)' -count=1
go test ./internal/command ./internal/listener/message ./internal/listener -count=1
go test ./...
go vet ./...
go build ./...
git diff --check
git diff --cached --check
git status --short
```

Acceptance requires recorded RED failures followed by matching Green reruns; exact alias/USER metadata; normal dispatch authorization proof; direct Saturn local test behavior plus source-derived edge coverage; exact local body/recipient/whisper behavior; controlled remote non-activation; and a clean diff check. A future remote vertical needs separate architecture, implementation, and QA evidence before `msgchannel` as a whole can be called complete.

## Evidence and baseline record

**[OBSERVED]** This document is the only task-owned write. The repository was already dirty before this architecture task with tracked and untracked application/test/handoff changes. No application or test code was staged, reset, cleaned, restored, checked out, committed, or otherwise modified here.

**[OBSERVED]** Every cited Zenbot and Saturn path was opened in the current trees. The selected source authority and direct source test are `MsgChannelCommandImpl.java` and `MsgChannelCommandImplTest.java`; its common parsing/authorization dependency is `UserCommandBaseImpl.java`.

**[TEST-BACKED]** Post-write read-only baseline, run from `/Users/ab/workspace/go-projects/zenbot`:

```text
$ go test ./internal/command ./internal/listener/message ./internal/listener -count=1
ok  zenbot/internal/command           11.918s
ok  zenbot/internal/listener/message  16.132s
ok  zenbot/internal/listener          0.447s

$ git diff --check
$ git diff --cached --check
# both exited 0 with no output
```

`git status --short -- .hermes/handoffs/next-core-command-after-support-relay-architecture.md` reports this document as the untracked task artifact. The tracked `git diff --name-only` still lists only pre-existing dirty application paths and does not include this untracked handoff. The file was verified non-empty; no application/test path was written by this architecture pass.
