# Independent QA — remote `msgchannel` / `msgroom`

## Verdict: PASS

**Scope:** Zenbot `/Users/ab/workspace/go-projects/zenbot`, branch `migration/saturn-zenbot-parity`; Saturn `/Users/ab/workspace/projects/saturn`, branch `develop`. Saturn was inspected read-only. The Zenbot tree was deliberately dirty before QA and remains dirty; no staging, commit, reset, clean, checkout, restore, or Saturn write was performed.

## Source-grounded behavior audit

### Saturn authority inspected

- `src/main/java/org/saturn/app/command/impl/user/MsgChannelCommandImpl.java:23-105`: aliases `msgchannel`/`msgroom`, `REGULAR` authorization, removal of all `?` then trim, current-room branch, remote `msg-<first 8 UUID chars>` join, source vs. target channel mapping, reply-bearing request, and formatted body.
- `src/main/java/org/saturn/app/listener/snapshot/EngineSnapshotSession.java:21-58`: temporary engine is configured with target channel/nick/password and sends raw payloads.
- `src/main/java/org/saturn/app/listener/snapshot/DeliverMessageToRoomOperation.java:15-20`: all-`isMe` exact ` " + target + " is empty`; otherwise one raw chat as `nick:"*"`, then `sent successfully.`
- `src/main/java/org/saturn/app/listener/snapshot/DefaultRoomSnapshotCoordinator.java:129-187`: terminal late-event suppression, generic request-reply failure marker, flush/close/removal.

### Zenbot evidence inspected

- `internal/command/msg_channel.go:17-75` preserves parse/normalization and local delivery, constructs remote source/target workflow requests, carries inbound whisper semantics and the non-empty failure marker, and generates a cryptographically random 32-hex-character workflow ID.
- `internal/command/msg_channel_test.go:30-370` covers aliases/USER registration, dispatcher authorization, normalizer, local no-snapshot path, remote request mapping, `msg-<8>` derivation seam, cancellation, submission failure, and source formatting including `![](`.
- `internal/listener/snapshot/session_factory.go:87-175` uses `SourceChannel` and omits `nick` for descriptor-free legacy requests; a `TemporaryJoin` switches only that session to target and JSON-marshals `nick#password`.
- `internal/listener/snapshot/remote_message_operation.go:15-40` has the exact empty predicate/result and uses `json.Marshal` for a single raw remote chat. `remote_message_operation_test.go:11-44` verifies all-is-me/no raw, one correctly parsed raw payload, and no retry after raw error.
- `internal/listener/snapshot/coordinator.go:153-326` owns terminal state, active-workflow removal, generic failure, close, timer stop, and late-event suppression. `remote_msgchannel_workflow_test.go:29-84` covers success, empty, parser error, raw error, cleanup, and a duplicate late `onlineSet`.
- `internal/listener/snapshot/transport_session_test.go:31-69` verifies fake `test-\\password` is serialized as JSON-safe `msg-12345678#test-\\password`, and descriptor-free join remains credential-free.

## Password containment and capability hardening

### Finding fixed

The implementation handoff asserted a master-only capability, but `SubmitCredentialedRoomSnapshot` was originally a method of `*core.EngineImpl`. Since replicas are also `*core.EngineImpl`, an arbitrary replica could satisfy the credentialed capability interface. This violated the required composition boundary even though the existing test only excluded a temporary session.

**RED → GREEN proof (new tracer):**

```text
$ go test ./internal/core -run '^TestMasterExposesCredentialedRemoteMsgChannelCapability$' -count=1
# zenbot/internal/core [zenbot/internal/core.test]
internal/core/remote_msgchannel_composition_test.go:16:11: undefined: BindCredentialedRoomSnapshotMaster
FAIL    zenbot/internal/core [build failed]
FAIL
```

Minimal fix:

- `internal/core/room_snapshot.go`: removes the credentialed submitter method from `*EngineImpl`; adds unexported `credentialedRoomSnapshotMaster` and `BindCredentialedRoomSnapshotMaster`. The wrapper is created only for `model.MASTER`, has the only credentialed submit method, and feeds the password directly into `TemporaryJoin` for coordinator submission.
- `cmd/zenbot/main.go`: production master command registration now receives only the master-bound wrapper.
- `internal/core/remote_msgchannel_composition_test.go`: proves the bound master has the capability while unbound engine, replica, and temporary session do not.

GREEN:

```text
$ go test ./internal/core -run '^TestMasterExposesCredentialedRemoteMsgChannelCapability$' -count=1
ok      zenbot/internal/core    0.435s
```

Password data-flow after hardening is limited to `EngineImpl.Password` → unexported master-bound submitter → `snapshot.TemporaryJoin` → JSON join serialization in `session_factory.go`. It is absent from `common.Engine`, replies, operation/raw chat body, logging introduced by this slice, persistence, and broad engine interfaces. Static search found only this temporary-join path plus pre-existing host engine configuration/join code and fake test values.

## TDD provenance

- **Implementation tracers 1–4 and 6:** retained implementation-handoff RED→GREEN evidence; independently rerun as regression gates below.
- **Implementation tracer 5:** **initial-green collision / regression-only**, preserved exactly as labeled by the implementation handoff. It is not recast as strict TDD provenance.
- **QA hardening tracer (master-only capability):** new independently observed RED→GREEN evidence above.
- Current green suites are regression evidence, not a replacement for historical strict TDD provenance.

## Executed verification

```text
$ go test ./internal/command -run 'TestMsgChannel|TestSupportRelay|TestReplica' -count=1
ok      zenbot/internal/command                 2.414s

$ go test ./internal/listener/snapshot -run 'Test(RemoteMessage|RemoteMsgChannel|TransportSession|Coordinator|RealCoordinatedSession)' -count=1
ok      zenbot/internal/listener/snapshot       0.395s

$ go test ./cmd/zenbot ./internal/core ./internal/command ./internal/listener/snapshot -run 'Test.*RemoteMsg(Channel|Room)|TestReal.*RemoteMessage' -count=1
ok      zenbot/cmd/zenbot                       0.546s [no tests to run]
ok      zenbot/internal/core                    0.226s
ok      zenbot/internal/command                 0.270s [no tests to run]
ok      zenbot/internal/listener/snapshot       0.446s

$ go test -race ./internal/command ./internal/core ./internal/listener/snapshot ./cmd/zenbot -count=1
ok      zenbot/internal/command                 79.979s
ok      zenbot/internal/core                    2.294s
ok      zenbot/internal/listener/snapshot       2.232s
ok      zenbot/cmd/zenbot                       5.149s

$ go test ./...
PASS: all packages; notable direct packages: cmd/zenbot 1.141s, command 16.637s, core 0.852s, listener/snapshot cached.

$ go vet ./...
exit 0; no output

$ go build ./...
exit 0; no output

$ git diff --check
exit 0; no output

$ git diff --cached --check
exit 0; no output
```

### Coverage note

```text
$ go test -cover ./internal/command ./internal/core ./internal/listener/snapshot
command             76.3% of statements
core                66.9% of statements
listener/snapshot   81.7% of statements
```

## Static/security/exclusion review

- No mine/mining code, scheduler, credential sink, registration, or related change was introduced by this vertical.
- No Whiskey/proxy configuration, dialing, AGENT replica, failover, or registration was introduced by this vertical. Existing dirty `internal/transport/proxy.go`, `internal/command/replica.go`, and their tests predate this QA scope and were not modified here.
- No lifecycle, SQL/H2/schema, agent, repository, service, config, or Saturn source changes were made by QA hardening. The only production composition change is master command registration in `cmd/zenbot/main.go`.
- The snapshot operation sends no raw on all-is-me; non-empty sends once; parser/start/transport/raw failure uses the existing generic `Unable to complete room operation.` reply path when `ReplyMessage` is non-empty. Coordinator terminal state and `coordinatedSession` close-once behavior prevent duplicate output/send on late events.

## Paths touched by this QA task

- `cmd/zenbot/main.go`
- `internal/core/room_snapshot.go`
- `internal/core/remote_msgchannel_composition_test.go`
- `.hermes/handoffs/remote-msgchannel-qa.md` (this artifact)

The pre-existing implementation vertical paths reviewed but not modified by QA include `internal/command/msg_channel.go`, `internal/command/msg_channel_test.go`, `internal/common/room_snapshot.go`, `internal/listener/snapshot/coordinator.go`, `session_factory.go`, `transport_session_test.go`, `remote_message_operation.go`, `remote_message_operation_test.go`, and `remote_msgchannel_workflow_test.go`.

## Exact post-change tree state

`git status --short --branch` still reports `migration/saturn-zenbot-parity...origin/migration/saturn-zenbot-parity`, a large intentionally dirty set of pre-existing modified/untracked files, plus the QA-touched paths above. Nothing is staged. The full status output was captured during QA; this artifact is an additional untracked handoff file.

## Acceptance mapping

PASS: aliases/USER authorization and normal dispatcher are retained; local path remains direct; remote target and source channels are distinct; nick is generated as `msg-<8>` from workflow ID; original whisper flag and generic failure marker are carried; fake credential serialization is JSON-safe; legacy snapshot join remains source-only without `nick`; empty/non-empty/failure/late-event semantics are covered; the credential capability is now master-bound instead of being exposed by every `EngineImpl`; and no excluded vertical was added by QA.
