# Rapid live-agent ambient QA

## Verdict: ACCEPT

Independent QA accepted the ambient/quiet/latest-wins vertical. No implementation defect requiring a production repair was found in the owned slice.

## Contract coverage verified

- `participation.Pipeline` owns one atomic cadence counter. It advances only after eligibility, quiet-request handling, mention precedence, and the existing moderation pass; already-quiet senders do not advance it. Ambient and quiet paths return pass, while mentions retain claim behavior.
- Quiet state is keyed by room plus the stable `AgentUserIdentity`, expires at the exact deadline, suppresses ambient only, and does not suppress a later mention.
- `APIBridge` converts API invocations once and routes `AMBIENT` to `SubmitAmbient`; normal modes retain ordinary `Submit` behavior.
- The runtime keeps one replaceable pending ambient invocation, does not consume ordinary admission capacity, executes the latest pending invocation rather than stale pending work, preserves ordinary work, and clears/cancels/waits safely on `Close`.
- Ambient runner/finalizer/sink errors do not invoke the reply-required failure sink, retry, or emit fixed user-visible failure text. Blank trimmed output errors before delivery; an exact trimmed no-reply marker is silent; an embedded marker remains visible.
- Enabled configuration constructs the quiet registry and resolved cadence wiring. Disabled configuration returns pass-through participation without constructing a provider/runtime. Ambient paths remain pass-through into command dispatch.

## QA addition

- Added `TestAPIBridgeRoutesAmbientPastFullOrdinaryAdmission` in `internal/agent/runtime/ambient_test.go`. It fills ordinary admission, submits an API-mode ambient invocation through the bridge, proves it is accepted and executes after ordinary work. This covers the bridge-specific bypass, rather than only direct `SubmitAmbient` calls.

## Gates run

| Command | Result |
|---|---|
| `gofmt -w` task-owned Go files | PASS |
| focused participation/live/runtime/cmd tests | PASS |
| `go test -race ./internal/agent/participation ./internal/agent/live ./internal/agent/runtime ./internal/listener/message -count=1` | PASS |
| `go test -race ./internal/agent/runtime -run 'Test(RuntimeAmbient|APIBridgeRoutesAmbient|RuntimeClose)' -count=20` | PASS |
| `go test ./...` | PASS |
| `go build ./...` | PASS |
| `git diff --check` including untracked ambient test | PASS |
| `go vet ./...` | FAILS outside this slice: `internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl` |

## Boundaries retained

No edits to protected migration documents, no commit/push, and no work on H2/SQL/history/tools/moderation/remote/replica. Existing direct `l`, mention, relay, memory, and shared command-path behavior was not changed by this QA work.
