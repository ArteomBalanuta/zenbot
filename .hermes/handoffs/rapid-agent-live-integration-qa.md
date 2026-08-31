# Slice 1 live-agent mention QA

## ACCEPT

Slice 1 acceptance gates are green. No SQL/H2, relay topology, ambient/quiet activation, tools, durable memory/history, or remote/replica behavior was added by this QA pass.

## Concrete QA fixes

1. **Explicit runtime validation was being bypassed.** `Resolve` applied defaults after runtime reads, so explicitly configured `0` values for positive participation fields silently became defaults. Defaults now apply before runtime lookup; explicit blank/zero values reach enabled-agent validation.
2. **Zero-user rooms could never submit a mention.** `InvocationFactory.Create` copied an empty trusted user list with `append([]string(nil), ...)`, producing nil and violating the API context's non-nil user-list contract. It now copies with `make`/`copy`, preserving a non-nil empty snapshot.
3. **Disabled agent configuration blocked startup.** `directAgentInvoker` returned an error when disabled, causing `main` to exit before the pass-through room integration could be installed. It now returns a nil invoker without error, so utilities register without `l` and disabled participation remains pass-through.

## Added/strengthened coverage

- `internal/config/agent_config_participation_test.go`: explicit runtime blank/zero/negative participation values are rejected rather than defaulted.
- `internal/agent/live/participation_test.go`: public mention claim/submission, trusted snapshot capabilities and immutable users, non-mentions, command-prefix/blank pass-through, and claimed submission error.
- `internal/listener/message/chain_test.go`: pass/claim/error mapping at `AgentParticipation`.
- `cmd/zenbot/live_agent_test.go`: disabled direct-agent construction does not block startup.

Existing focused tests cover exact trimmed marker silence, embedded-marker reply content, empty mention failure, cancellation, reply-required failure sink mode gating, admission, serialization, success sink behavior, default handler order, and shutdown rejection.

## Verification (executed from repository root)

```text
gofmt -w [all Slice-1 owned Go files]
go test ./internal/config -run 'TestAgentConfig.*Participation|TestAgent.*Resolve' -count=1
ok   zenbot/internal/config

go test ./internal/agent/live ./internal/agent/runtime ./internal/agent/participation -count=1
ok   zenbot/internal/agent/live
ok   zenbot/internal/agent/runtime
ok   zenbot/internal/agent/participation

go test ./internal/listener/message ./internal/listener -run 'Test.*Participation|TestDefaultChain' -count=1
ok   zenbot/internal/listener/message
ok   zenbot/internal/listener [no tests to run]

go test ./cmd/zenbot -run 'Test.*LiveAgent|TestDirectAgentInvokerDisabled' -count=1
ok   zenbot/cmd/zenbot

go test -race ./internal/agent/runtime ./internal/agent/live ./internal/listener/...
ok   zenbot/internal/agent/runtime
ok   zenbot/internal/agent/live
ok   zenbot/internal/listener
ok   zenbot/internal/listener/info
ok   zenbot/internal/listener/message
ok   zenbot/internal/listener/snapshot

go test ./...
PASS: all listed packages passed; expected no-test packages reported as such.

git diff --check
PASS (no output)
```

## Configuration prerequisite

For enabled room-agent operation, `agent.enabled=true` must resolve valid endpoint/model/provider credentials plus non-blank `creatorTrip` and `noReplyMarker`; positive `ambientEveryMessages`, `quietMinutes`, `contextMessageLimit`, and `maxConcurrentRequests`; and non-negative `queueCapacity`. Defaults are `595754`, `8`, `15`, `60`, `[[SATURN_NO_REPLY]]`, `1`, and `0`. Disabled configuration does not require provider credentials and passes all messages through.

## Deferred boundary

Relay topology remains unchanged and deferred to Slice 2. Ambient/quiet policy activation, moderation, tool loops, durable context/history, SQL/H2 work, and replica/remote behavior remain out of scope.
