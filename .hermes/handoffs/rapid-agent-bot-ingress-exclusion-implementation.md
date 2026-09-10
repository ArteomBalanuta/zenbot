# Resolved bot-author ingress exclusion — implementation

## Outcome

Implemented the approved, bounded ingress filter before live agent invocation creation. A public event with a resolved active-room `Context.Author.IsBot == true` now passes through agent participation without constructing or submitting an invocation.

## Touched paths

- `internal/agent/participation/invocation.go`
  - Added immutable `Event.AuthorIsBot bool`, documented as listener-resolved active-user metadata.
  - Added it to the existing post-`Monitor` eligibility predicate.
- `internal/agent/live/participation.go`
  - Populates the event bit exactly as `c.Author != nil && c.Author.IsBot` during live event construction.
- `internal/agent/participation/policies_test.go`
  - Covers monitor-before-filter behavior, zero submission for resolved bot authors, and unchanged ambient cadence for the next human event.
- `internal/agent/live/participation_test.go`
  - Covers live propagation from a resolved bot author and nil-author normal mention behavior.
- `.hermes/handoffs/rapid-agent-bot-ingress-exclusion-implementation.md`

## Trust and ordering semantics

`ResolveUserMetadata` remains the sole active-user lookup. `RoomParticipation` copies only the already resolved `Context.Author.IsBot` fact; it does not inspect raw chat payload fields, rescan engine users, or expose the bit to snapshots, contexts, prompts, tools, runtime, or persistence.

`Pipeline.Handle` still calls `Monitor(e)` first. It then returns `Pass` for `AuthorIsBot`, before context/factory creation, quiet mutation, mention parsing/submission, moderation submission, ambient counter mutation, or any runtime work. Conventional-name filtering remains an independent OR condition. A nil/false author bit retains existing behavior.

## Strict TDD evidence

### RED 1: pipeline behavior

Added `TestPipelinePassesResolvedBotAuthorAfterMonitoringWithoutAdvancingAmbientCadence`, then ran:

```text
go test ./internal/agent/participation -run '^TestPipelinePassesResolvedBotAuthorAfterMonitoringWithoutAdvancingAmbientCadence$' -count=1
# zenbot/internal/agent/participation [zenbot/internal/agent/participation.test]
internal/agent/participation/policies_test.go:105:3: unknown field AuthorIsBot in struct literal of type Event
internal/agent/participation/policies_test.go:117:3: unknown field AuthorIsBot in struct literal of type Event
FAIL	zenbot/internal/agent/participation [build failed]
FAIL
```

GREEN added the event field and one eligibility condition. Re-run:

```text
ok  	zenbot/internal/agent/participation	0.452s
```

### RED 2: live provenance

Added `TestRoomParticipationUsesResolvedBotAuthorMetadataOnly`, then ran:

```text
go test ./internal/agent/live -run '^TestRoomParticipationUsesResolvedBotAuthorMetadataOnly$' -count=1
--- FAIL: TestRoomParticipationUsesResolvedBotAuthorMetadataOnly (0.00s)
    participation_test.go:69: resolved bot claimed=true err=<nil> invocations=1
FAIL
FAIL	zenbot/internal/agent/live	0.358s
FAIL
```

GREEN added only `AuthorIsBot: c.Author != nil && c.Author.IsBot` to `RoomParticipation.Handle`. Formatted owned Go files and re-ran both new tests:

```text
ok  	zenbot/internal/agent/live	0.356s
ok  	zenbot/internal/agent/participation	0.203s
```

The finalized pipeline and live tests use the nonconventional sender `automaton`; it avoids the existing conventional-bot-nick fallback, so the resolved flag is the authority under test. The initial field-absence RED used `otherBot`; after GREEN, the fixture was strengthened to `automaton` and all focused, race, full-suite, build, format, and diff gates were rerun.

## Gates

Passed:

```text
go test ./internal/agent/participation -run 'Test(Pipeline.*Bot|PipelinePrecedence|PipelineRejectsCaseInsensitiveSelfAndConventionalBotAuthors)' -count=1
ok  	zenbot/internal/agent/participation	0.229s

go test ./internal/agent/live -run 'TestRoomParticipation.*(Bot|Mention|Ambient|Quiet)' -count=1
ok  	zenbot/internal/agent/live	0.257s

go test ./internal/listener/message -run 'Test(DefaultChain|AgentParticipation)' -count=1
ok  	zenbot/internal/listener/message	0.335s

go test ./cmd/zenbot -run 'Test.*LiveAgent' -count=1
ok  	zenbot/cmd/zenbot	0.386s

go test -race ./internal/agent/participation ./internal/agent/live ./internal/listener/message -count=1
ok  	zenbot/internal/agent/participation	1.369s
ok  	zenbot/internal/agent/live	2.002s
ok  	zenbot/internal/listener/message	1.535s

go test ./... -count=1
PASS (all packages; repository H2 package completed in 31.896s)

go build ./...
PASS

gofmt -w internal/agent/participation/invocation.go internal/agent/participation/policies_test.go internal/agent/live/participation.go internal/agent/live/participation_test.go
PASS

git diff --check
PASS
```

Informational pre-existing unrelated vet failure persists:

```text
internal/core/engine_impl.go:95:22: NewEngineImpl passes lock by value: zenbot/internal/core.EngineImpl
```

## Exclusions preserved

No changes to listener chain ordering, relay, provider, tools, command execution, runtime, delivery/finalization, persistence, configuration, SQL/H2, resources, protected migration documents, Saturn source, commits, or pushes.
