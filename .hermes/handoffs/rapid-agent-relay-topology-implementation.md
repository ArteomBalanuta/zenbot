# Slice 2 AGENT relay topology implementation

## Scope

Implemented only the Slice 2 AGENT-child-to-host relay topology specified in `rapid-agent-live-integration-architecture.md` lines 385–492. Slice 1 participation behavior was left intact; no configuration, SQL/H2, memory, tool, moderation, ambient/quiet, remote/replica, or protected-document work was added.

## Implementation

- Added `model.AGENT`, documented as a child connection whose inbound chat is relayed to its host and never independently runs room automation or commands.
- Added `internal/relay/topology.go` as the sole owner of the narrow contract:
  - `relay.HostRelay`
  - `relay.AgentHostRef`
  - `relay.NewHost`, adapting a host's existing `SendChatMessage("", author+": "+text, false)` direct transport.
- The host adapter checks cancellation before sending, returns transport errors unchanged, and does not retry.
- No pre-JSON escaping is applied. `SendChatMessage` remains the only JSON-escaping layer.
- `core.EngineImpl` holds the relay privately. `core.NewEngineImpl` takes it at construction; `HostRelay` exposes read-only access.
- `factory.EngineOptions.HostRelay` is required for `model.AGENT`; `NewEngineWithOptions` rejects a missing relay.
- `RelayAgentMessage` now occurs in its existing pre-log/mail/AFK/participation/command position. For AGENT it:
  - invokes the host once with the unmodified author/text pair;
  - stops the chain on success;
  - logs and stops with no delivery if a compatibility-created AGENT has no host;
  - returns the host transport error and stops, with no retry.
  Non-AGENT engines continue unchanged.

## Files added

- `internal/relay/topology.go`
- `internal/relay/topology_test.go`
- `internal/listener/message/relay_agent_message_test.go`
- `rapid-agent-relay-topology-implementation.md`

## Files modified

- `internal/model/engine_type.go`
- `internal/core/engine_impl.go`
- `internal/core/engine_impl_test.go`
- `internal/factory/engine_factory.go`
- `internal/factory/engine_factory_test.go`
- `internal/listener/message/handlers.go`

## TDD evidence

### RED

Before production implementation:

```text
== relay RED ==
internal/relay/topology_test.go:25:10: undefined: NewHost
internal/relay/topology_test.go:38:12: undefined: NewHost
internal/relay/topology_test.go:50:12: undefined: NewHost
FAIL zenbot/internal/relay [build failed]

== listener RED ==
zenbot/internal/relay: no non-test Go files in .../internal/relay
FAIL zenbot/internal/listener/message [build failed]

== factory RED ==
zenbot/internal/relay: no non-test Go files in .../internal/relay
FAIL zenbot/internal/factory [build failed]

== core payload RED ==
zenbot/internal/relay: no non-test Go files in .../internal/relay
FAIL zenbot/internal/core [build failed]
```

### GREEN and verification

```text
gofmt -w [all Slice-2-owned Go files]

go test ./internal/relay ./internal/listener/message -run 'TestRelayAgentMessage|TestHostRelay' -count=1
ok zenbot/internal/relay
ok zenbot/internal/listener/message

go test ./internal/factory -run 'TestNewEngine.*Agent.*Host' -count=1
ok zenbot/internal/factory

go test ./internal/core -run 'TestHostRelayEnqueues' -count=1
ok zenbot/internal/core

go test ./internal/relay ./internal/listener/message -run 'TestRelayAgentMessage' -count=1
ok zenbot/internal/relay [no tests to run]
ok zenbot/internal/listener/message

go test ./internal/relay -run 'TestHostRelay' -count=1
ok zenbot/internal/relay

go test ./internal/factory -run 'Test.*Agent.*Host' -count=1
ok zenbot/internal/factory

go test ./internal/core -run 'TestHostRelayEnqueues' -count=1
ok zenbot/internal/core

go test -race ./internal/relay ./internal/listener/message ./internal/core -count=1
ok zenbot/internal/relay
ok zenbot/internal/listener/message
ok zenbot/internal/core

go test ./...
PASS: every package passed; expected no-test packages reported as such.

git diff --check
PASS (no output)
```

Focused coverage proves MASTER/REPLICA pass-through, AGENT one-delivery relay and chain stop, no-host claim/no delivery, host error stop/no retry, factory rejection and exact host installation, cancellation, and newline/quote/backslash/non-ASCII logical plus JSON-wire payload preservation.

## Deferrals

- Ambient cadence/coalescing and quiet-request activation.
- Moderation, bot presence, join automation, agent tools/tool loops, durable memory/history/context, SQL/H2, and remote/replica parity.
- Any direct configuration or main-runtime construction path for AGENT children; this slice exposes the explicit factory contract but does not create a new child lifecycle.

## Repository state

The working tree already contained extensive unrelated modifications and untracked handoffs before this slice. They were preserved. No commit or push was performed.
