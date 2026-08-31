# Slice 2 AGENT relay topology QA

## Verdict: ACCEPT

The inspected implementation satisfies the Slice 2 contract in `rapid-agent-live-integration-architecture.md` lines 385–492.

## Contract findings

- `model.AGENT` is distinct from MASTER/REPLICA/ZOMBIE and documented as a child relay-only engine.
- `core.EngineImpl` retains `hostRelay` privately; `NewEngineImpl` installs it at construction and exposes only the read-only `HostRelay()` capability.
- `factory.NewEngineWithOptions` rejects `AGENT` without `EngineOptions.HostRelay` and preserves the provided reference on successful construction.
- `relay.NewHost` sends exactly one direct public `SendChatMessage("", author+": "+text, false)` delivery. It leaves valid Go text unmodified, so `SendChatMessage` performs the single JSON serialization step. It checks cancelled contexts, returns transport errors unchanged, and does not retry.
- `RelayAgentMessage` is AGENT-only. MASTER and REPLICA continue; AGENT relays once then claims the message; a missing host logs/claims with zero delivery; a host error is returned and stops the chain.
- The default chain places relay before log, mail, AFK, participation, and command dispatch. Slice 1 participation injection remains at the downstream `AgentParticipation` position; focused live/participation/direct-command/memory coverage passed.

## QA repair

Replaced the tautological `TestDefaultChainOrder` in `internal/listener/message/chain_test.go` with `TestDefaultChainPlacesRelayBeforeAllDownstreamHandlers`. The replacement inspects the actual constructed handler list, verifies relay position and every downstream handler, and verifies that the supplied Slice 1 participation handler is retained. No production behavior was changed by QA.

## Test evidence

```text
$ go test ./internal/relay ./internal/listener/message -run 'Test(RelayAgentMessage|HostRelay|DefaultChainPlacesRelay)' -count=1
ok  zenbot/internal/relay
ok  zenbot/internal/listener/message

$ go test ./internal/factory -run 'Test.*Agent.*Host|TestNewEngineRejectsAgentWithoutHostRelay' -count=1
ok  zenbot/internal/factory

$ go test ./internal/core -run 'TestHostRelayEnqueuesOneJSONEscapedPublicPayload' -count=1
ok  zenbot/internal/core

$ go test ./internal/agent/live ./internal/agent/participation ./internal/command -run 'Test.*(Mention|Direct|Memory|Participation)' -count=1
ok  zenbot/internal/agent/live
ok  zenbot/internal/agent/participation
ok  zenbot/internal/command

$ go test -race ./internal/relay ./internal/listener/message ./internal/core -count=1
ok  zenbot/internal/relay
ok  zenbot/internal/listener/message
ok  zenbot/internal/core

$ go test ./...
PASS: all packages passed (including internal/repository/h2); expected no-test packages reported as such.

$ gofmt -w [all Slice 2 Go files]
$ git diff --check
PASS: no output
```

Focused coverage includes exact newline/quote/backslash/non-ASCII logical and wire payload preservation, one-send/no-retry transport failure, cancellation, non-AGENT pass-through, AGENT host/missing-host/error chain stops, and factory construction behavior.

## Scope exclusions

No relay child lifecycle, ambient/quiet behavior, moderation, tool loop, durable memory/history, remote/replica parity, SQL/H2 implementation, protected-document edits, commit, or push was performed. The full test command exercised existing H2 tests only.

## Dirty-tree preservation

The repository was already extensively dirty before QA across unrelated command, agent, H2, config, runtime, and handoff work. Those changes were preserved. QA changed only `internal/listener/message/chain_test.go` and created this handoff; the Slice 2 implementation files remain uncommitted alongside their pre-existing work.
