# Remote `msgchannel` / `msgroom` implementation handoff

## Scope and transition

Implemented the selected remote branch only. The stale `TestMsgChannelFailuresAndRemoteExclusion` assertion was renamed to `TestMsgChannelFailuresAndRemoteCapability` in tracer 2: remote now fails closed only when the master-owned credentialed snapshot capability is absent, rather than claiming all remote delivery is unavailable.

## TDD evidence

### Tracer 1 — retained, independently rerun

Historical RED (from `deleg_e9927426/task-0.log`):

```text
--- FAIL: TestMsgChannelRemoteCapability/...capability-bearing_engine_submits_one_remote_workflow
status=FAILED err=remote room delivery is not configured
```

Retained GREEN rerun:

```text
$ go test ./internal/command -run '^TestMsgChannelRemote(Capability|KeepsLocalPath)' -count=1
ok zenbot/internal/command 0.436s
```

### Tracer 2 — identity, request semantics, cancellation, submission failure

RED:

```text
undefined: msgChannelWorkflowID
request.RemoteMessage undefined
FAIL zenbot/internal/command [build failed]
```

GREEN:

```text
$ go test ./internal/command -run '^TestMsgChannelRemote(Request|Cancellation|SubmissionFailure|Authorization)' -count=1
ok zenbot/internal/command 0.550s
$ go test ./internal/command -run '^TestMsgChannelRemote(Capability|KeepsLocalPath)' -count=1
ok zenbot/internal/command 0.353s
```

### Tracer 3 — credentialed join and legacy behavior

RED:

```text
unknown field TemporaryJoin in RoomSnapshotRequest
undefined: TemporaryJoin
FAIL zenbot/internal/listener/snapshot [build failed]
```

GREEN:

```text
$ go test ./internal/listener/snapshot -run '^TestTransportSession(CredentialedRemoteMessageJoin|LegacyJoinRemainsCredentialFree)$' -count=1
ok zenbot/internal/listener/snapshot 0.474s
```

### Tracer 4 — remote operation

RED:

```text
undefined: NewRemoteMessageOperation
FAIL zenbot/internal/listener/snapshot [build failed]
```

GREEN:

```text
$ go test ./internal/listener/snapshot -run '^TestRemoteMessageOperation' -count=1
ok zenbot/internal/listener/snapshot 0.469s
```

### Tracer 5 — coordinator workflow collision

The proposed `TestRemoteMsgChannelWorkflow` passed on its first run (`ok zenbot/internal/listener/snapshot 0.483s`) using independently TDD-proven tracers 3 and 4. Per the strict rule, no production modification was attributed to tracer 5 and no fabricated RED/GREEN claim is made. It is retained as regression coverage of success, all-is-me, parser/raw failure, cleanup, and late-event suppression.

### Tracer 6 — master capability

RED:

```text
--- FAIL: TestMasterExposesCredentialedRemoteMsgChannelCapability
master does not expose credentialed snapshot submission
```

GREEN:

```text
$ go test ./cmd/zenbot ./internal/core ./internal/command ./internal/listener/snapshot -run 'Test.*RemoteMsg(Channel|Room)|TestReal.*RemoteMessage' -count=1
ok zenbot/cmd/zenbot [no tests to run]
ok zenbot/internal/core
ok zenbot/internal/command [no tests to run]
ok zenbot/internal/listener/snapshot
```

## Implementation

- Remote command formatting and source channel mapping are preserved; remote submission carries author, original whisper semantics, failure marker, random workflow ID, and message body.
- `CredentialedRoomSnapshotSubmitter` is the narrow master-owned capability. `EngineImpl` alone derives temporary `msg-<first eight workflow ID chars>#<host password>` join material.
- `TemporaryJoin` is optional. Descriptor-free legacy snapshot requests still join `SourceChannel` without a `nick` property.
- `RemoteMessageOperation` returns exact empty/success results and emits one JSON-encoded raw `{cmd:chat,nick:*,text:...}`; raw failure is returned without retry.

## Security review

The password is not added to `common.Engine`, replies, errors, logs, persistence, or the remote message raw chat. It exists only in `EngineImpl.Password`, the temporary descriptor, and serialized temporary join; test passwords are fake. JSON encoding is through `encoding/json`; tests cover quotes and backslashes. No mine, Whiskey/proxy, lifecycle, SQL/H2, agent, or unrelated application file was changed by this slice.

## Touched-file inventory

- `internal/command/msg_channel.go`
- `internal/command/msg_channel_test.go`
- `internal/common/room_snapshot.go`
- `internal/core/room_snapshot.go`
- `internal/core/remote_msgchannel_composition_test.go`
- `internal/listener/snapshot/coordinator.go`
- `internal/listener/snapshot/session_factory.go`
- `internal/listener/snapshot/transport_session_test.go`
- `internal/listener/snapshot/remote_message_operation.go`
- `internal/listener/snapshot/remote_message_operation_test.go`
- `internal/listener/snapshot/remote_msgchannel_workflow_test.go`
- this handoff

## Verification completed so far

```text
ok zenbot/internal/command 2.318s  # TestMsgChannel|TestSupportRelay|TestReplica
ok zenbot/internal/listener/snapshot 0.297s  # specified snapshot focus
ok zenbot/internal/command 80.398s  # race
ok zenbot/internal/core 2.309s  # race
ok zenbot/internal/listener/snapshot 2.243s  # race
ok zenbot/cmd/zenbot 5.160s  # race
git diff --check: exit 0
git diff --cached --check: exit 0
```

Final run:

```text
$ go test ./...
PASS: all packages (including command 18.650s, core 2.207s, snapshot 2.387s, H2 42.436s)
$ go vet ./...
exit 0; no output
$ go build ./...
exit 0; no output
```
