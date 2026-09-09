# Live-agent shared-runtime `l` implementation evidence

This file records strict sequential tracer RED→GREEN evidence.

## Tracer 1 — command admission

**RED** (`go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1`, exit 1):
```
# zenbot/internal/command [zenbot/internal/command.test]
internal/command/handlers_test.go:229:29: cannot use submitter (variable of type *recordingDirectAgentSubmitter) as DirectAgentInvoker value in argument to directLDefinition: *recordingDirectAgentSubmitter does not implement DirectAgentInvoker (missing method Invoke)
FAIL	zenbot/internal/command [build failed]
FAIL
```
Expected feature-missing type failure: command registration still required synchronous `DirectAgentInvoker.Invoke`.

**GREEN** (same command, exit 0):
```
ok  	zenbot/internal/command	0.522s
```
Command execution now trims and submits exactly once; no command-side provider, delivery, or persistence branch remains.

## Tracer 2 — trusted invocation construction

**RED** (`go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1`, exit 1):
```
# zenbot/internal/agent/live [zenbot/internal/agent/live.test]
internal/agent/live/direct_submission_test.go:27:13: undefined: DirectSubmissionAdapter
FAIL	zenbot/internal/agent/live [build failed]
FAIL
```
Expected feature-missing symbol failure.

**GREEN** (same command, exit 0):
```
ok  	zenbot/internal/agent/live	0.463s
```
The adapter constructs one factory-derived DIRECT invocation from the canonical message and copied trusted snapshot, then submits it once.

**HARD STOP — strict tracer violation discovered before tracer 3.** `direct_submission.go` includes canceled-context, nil-message/service/factory/snapshot, blank-prompt, and blank-room validation guards. The first three named guards belong to tracer 3 but were introduced while making tracer 2 green. Per the architecture's explicit instruction not to patch forward when production exists ahead of its tracer, no tracer 3 test or additional implementation was started.

