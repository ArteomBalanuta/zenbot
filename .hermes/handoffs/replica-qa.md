# Replica lifecycle parity QA

## Verdict: PASS

Independent QA accepts the bounded `replica` / `replicaoff` / `replicastatus` slice.

## Audit result

- Saturn authority was checked at `ReplicaCommandImpl.java`, `ReplicaOffCommandImpl.java`, and `ReplicaStatusCommandImpl.java`.
- Concrete routes replace the three generic fallback cases; the bounded fallback guard now permits only deferred `mine`, `restart`, `shutdown`, `sql`, and `whiskey`.
- Catalog aliases and ADMIN role are preserved. Registration remains fail-closed when the engine lacks `ReplicaController`.
- Tests cover first-argument-only handling, reply-to-author whisper delivery, success replies, missing/blank/host/duplicate/absent paths, lifecycle add/remove errors without a success reply, sorted status with `none`, authorization denial, and concrete rather than generic route construction.
- Existing real-WebSocket coverage passed for inbound replica creation and runtime transport failure cleanup/report-once.

## Defect found and hardened

`ChatMessage.GetArguments()` uses `strings.Fields`, so `!replica   ` and `!replicaoff   ` were collapsed into the no-argument path and produced usage replies. Saturn distinguishes supplied blank input from missing input.

Strict TDD evidence:

```text
$ go test ./internal/command -run '^TestReplicaSaturnFailuresReplyAndFail$/blank$' -count=1 -v
FAIL: got "Example: !replica lounge"

$ go test ./internal/command -run '^TestReplicaOffAliasesRepliesAndFailures$/blank$' -count=1 -v
FAIL: got "Example: !replicaoff lounge"
```

The minimal fix adds `hasBlankReplicaChannel`, preserving trailing-whitespace intent and routing blank add/off requests to the exact Saturn host/blank replies. Both targeted tests then passed.

## Verification

```text
$ go test ./internal/command -run '^TestReplica' -count=1 -v
PASS

$ go test ./internal/command -run '^TestReal' -count=1 -v
PASS
  TestRealInboundWebSocketReplicaCommandReachesManager
  TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry

$ go test -race ./internal/command ./internal/core ./internal/factory -count=1
PASS
  command 42.993s; core 1.600s; factory 2.143s

$ go test ./... -count=1
PASS

$ go vet ./... && go build ./... && git diff --check
PASS

$ go test ./internal/command -cover -run 'TestReplica|TestReal.*Replica' -count=1
PASS; package statement coverage: 9.1%
```

The focused coverage percentage is package-wide (not a slice-only threshold); all named replica behavior branches above are explicitly asserted.

## Paths

QA modified:

- `internal/command/replica.go`
- `internal/command/registry.go`
- `internal/command/replica_test.go`
- `.hermes/handoffs/replica-qa.md`

Reviewed slice paths:

- `internal/command/handlers.go`
- `internal/command/admin_moderator_catalog_guard_test.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/real_websocket_replica_test.go`
- `internal/command/real_websocket_replica_failure_test.go`
- `internal/core/replica_manager.go`
- `internal/core/replica_controller.go`
- `internal/factory/replica_factory.go`

The checkout was already dirty. `git diff --name-only -- internal/core internal/factory cmd/zenbot/main.go` reports pre-existing dirty paths, but this QA made no change outside the command slice and did not modify core manager/controller/factory/main behavior.
