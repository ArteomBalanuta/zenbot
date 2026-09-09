# Live-agent `l` tracer-1+2 recovery evidence

## Recovery boundary

Manually restored the untracked `internal/agent/live/direct_submission.go` to the validated tracer-2 checkpoint prescribed by `live-agent-integration-tdd-forensic.md:34-52`:

- removed only the unproven `fmt`/`strings` imports and validation guards;
- retained the trusted snapshot read and defensive users copy;
- retained `InvocationFactory.Create(snapshot, *message, prompt, api.DIRECT, true)`;
- retained the single `AgentService.Submit` call;
- made no test, command, runtime, composition, Git-index, or unrelated working-tree changes during recovery.

The restored `Submit` body intentionally has no cancellation, nil-input, blank-prompt, or blank-room behavior. Those remain reserved for the next test-first tracer.

## Recovery verification

Run from `/Users/ab/workspace/go-projects/zenbot` after the manual recovery:

```text
$ go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1
ok  zenbot/internal/command  0.522s

$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1
ok  zenbot/internal/agent/live  0.380s

$ go test ./internal/command ./internal/agent/live -count=1
ok  zenbot/internal/command  9.932s
ok  zenbot/internal/agent/live  0.288s

$ git diff --check
(exit 0; no diagnostics)
```

Tracer 1 and tracer 2 are therefore retained as the validated checkpoint. Tracer 3 had not yet been implemented at this point in the recovery record.

## Tracer 3 — invalid-input rejection

**RED** — after adding `TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit`, before changing production code:

```text
$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
# zenbot/internal/agent/live [zenbot/internal/agent/live.test]
internal/agent/live/direct_submission_test.go:92:14: cannot use factory (variable of type *recordingInvocationFactory) as *participation.InvocationFactory value in struct literal
FAIL    zenbot/internal/agent/live [build failed]
FAIL
(exit 1)
```

The expected feature-missing failure established that the production adapter did not yet provide an observable factory-creation seam required to prove zero invocation creation.

**GREEN** — added only the canceled-context, nil-message/service, trimmed-blank-prompt, and post-snapshot trimmed-blank-room rejection guards. The adapter's factory field was narrowed to a private `Create` interface solely so the focused test can record and prove zero calls; no factory/snapshot nil guard was added.

```text
$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
ok  zenbot/internal/agent/live  0.450s
```

After `gofmt -w internal/agent/live/direct_submission.go internal/agent/live/direct_submission_test.go`, final verification:

```text
$ go test ./internal/agent/live -count=1
ok  zenbot/internal/agent/live  0.284s

$ go test ./internal/command -run '^TestDirectLCommandSubmitsTrimmedPromptWithoutCommandSideEffects$' -count=1
ok  zenbot/internal/command  0.447s

$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterCreatesTrustedDirectInvocation$' -count=1
ok  zenbot/internal/agent/live  0.256s

$ go test ./internal/agent/live -run '^TestDirectSubmissionAdapterRejectsInvalidInputWithoutSubmit$' -count=1
ok  zenbot/internal/agent/live  0.272s

$ git diff --check
(exit 0; no diagnostics)
```

Tracer 4 was not started.
