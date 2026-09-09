# Live-agent runtime-service forwarding/lifecycle implementation evidence

## Tracer 4 — runtime-service forwarding/lifecycle

**RED** — after adding `TestRuntimeServiceForwardsToOneRuntimeAndClosesIt` and before production implementation:

```text
$ go test ./internal/agent/live -run '^TestRuntimeServiceForwardsToOneRuntimeAndClosesIt$' -count=1
# zenbot/internal/agent/live [zenbot/internal/agent/live.test]
internal/agent/live/service_test.go:32:13: undefined: RuntimeService
FAIL    zenbot/internal/agent/live [build failed]
FAIL
(exit 1)
```

Expected feature-missing failure: the `live` package did not yet contain a `RuntimeService` implementation of the private `AgentService` boundary.

**GREEN** — after adding only `internal/agent/live/service.go`:

```text
$ go test ./internal/agent/live -run '^TestRuntimeServiceForwardsToOneRuntimeAndClosesIt$' -count=1
ok  zenbot/internal/agent/live  0.443s
(exit 0)
```

`RuntimeService` forwards each submission through `runtime.APIBridge{Runtime: s.Runtime}.Submit` and delegates close to that same runtime. The focused test proves one accepted invocation reaches the supplied runtime once, a full runtime rejects promptly, close cancels accepted work, and late submission is rejected.

## Final verification

After gofmt limited to `internal/agent/live/service.go` and `internal/agent/live/service_test.go` (empty `gofmt -d` output):

```text
$ go test ./internal/agent/live -run '^TestRuntimeServiceForwardsToOneRuntimeAndClosesIt$' -count=1
ok  zenbot/internal/agent/live  0.371s

$ go test ./internal/agent/live ./internal/agent/runtime -count=1
ok  zenbot/internal/agent/live  0.284s
ok  zenbot/internal/agent/runtime  0.486s

$ go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1
ok  zenbot/internal/command  0.330s

$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1
ok  zenbot/internal/agent/live  0.259s

$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
ok  zenbot/internal/agent/live  0.257s

$ git diff --check
(exit 0; no diagnostics)
```

The tracer created only `internal/agent/live/service.go`, `internal/agent/live/service_test.go`, and this evidence file. `service.go` imports only the API and runtime boundaries and has no construction call. Pre-existing dirty semantic/tool, persistence/SQL, transport/proxy/Whiskey, main, and listener-chain paths were not edited by this tracer.

