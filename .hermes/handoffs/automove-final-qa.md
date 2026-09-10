# AutoMove final independent QA

## Verdict: PASS — strict tracer-level TDD acceptance

The recovered AutoMove parity vertical is acceptable. This fresh audit verified the current source, the Saturn authority, every recovery handoff, and the new join-policy recovery record. Every current AutoMove production behavior has an antecedent, behavior-specific RED followed by a GREEN. The three branches that blocked the previous QA now have genuine one-at-a-time recovery cycles, not retroactive passing-test claims.

No application or test source was changed during this QA. The only QA-owned artifact is this handoff. The pre-existing dirty worktree was not reset, cleaned, checked out, staged, or committed.

## Authorities inspected

- Architecture: `.hermes/handoffs/automove-current-architecture.md`
- Prior QA and forensic/recovery records: `.hermes/handoffs/automove-qa.md`, `automove-tdd-forensic.md`, `automove-tdd-recovery.md`, `automove-join-tdd-forensic.md`, and `automove-join-tdd-recovery.md`
- Recovery implementation records: state mutation, core lifecycle, command, H2, join policy, listener order, and factory/main handoffs.
- Saturn source: `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/AutoMoveUserCommandImpl.java`, `.../listener/impl/UserJoinedListenerImpl.java`, and `.../util/SqlUtil.java`.

## Current contract audit

- **State:** `NewAutoMoveState` is disabled with exact `purgatory` source and `lounge` destination. `Configure` trims/rejects blank input, adds sources, replaces destination, and returns sorted isolated snapshots. Exact channel membership is case-sensitive.
- **Lifecycle:** enable/disable reject pre-cancelled contexts, serialize reconciliation, transition state before actions, add only absent replicas, remove only present configured replicas, and retain the first stop error after continuing remaining removals.
- **Command:** `automove` resolves to a concrete `MODERATOR` command with sole alias `automove`; registration is capability-gated. Toggles are case-insensitive and precede arity; trailing toggle arguments are ignored; configuration is exactly two non-toggle arguments; controller/action errors produce no success reply.
- **H2:** `UserTrips` uses `QueryContext`, preserves case, returns only USER rows, and uses the literal Saturn SQL `SELECT trip FROM trips WHERE type = 'USER';`.
- **Join policy:** only enabled configured replicas query trips; host, disabled/non-source, nil/blank, and pre-cancelled joins are inert. Membership is exact/case-sensitive. It sends the literal public (non-whisper) lounge notice, then uses typed `KickNickTo` with the dynamic destination. Query failure performs neither action; notice failure is logged and still kicks once; kick failure logs without retry.
- **Listener/factory/main:** permanent join order is active-user registration → semantic automation → share → presence log → automove. Master and future permanent replicas share one state pointer; temporary online-set sessions retain dummy listeners/no AutoMove state; main composes the complete master so the command registers.
- **Exclusions verified:** no AutoMove config/schema/migration/persistence, cross-process state, retry loop, source removal, remote/snapshot AutoMove composition, or temporary-session AutoMove composition was added. Chat dispatch retaining `context.Background()` is deliberate legacy behavior, not an AutoMove cancellation propagation claim.

## Strict TDD evidence matrix

| Current behavior slice | Antecedent RED → GREEN evidence | Result |
|---|---|---|
| Default state, exact source eligibility | `automove-tdd-recovery.md` retained original state tracer; `TestAutoMoveDefaultsAreFalsePurgatoryAndLounge` failed for absent state, then passed | PASS |
| Configure trim/blank/additive/replacement/sorted immutable snapshot | `automove-state-mutation-implementation.md`: `TestAutoMoveStateConfigure` RED for absent `Configure`, then GREEN | PASS |
| Enable ordering and missing-only reconciliation | `automove-core-lifecycle-implementation.md`: `TestEngineEnableAutoMoveMarksStateBeforeAddingOnlyMissingSources` RED, then GREEN | PASS |
| Lifecycle cancellation | same handoff: `TestEngineAutoMovePreCancelledContextDoesNotMutateOrAct` RED, then GREEN | PASS |
| Disable ordering, present-only removal, continue/first error | same handoff: `TestEngineDisableAutoMoveMarksDisabledRemovesPresentSourcesAndReturnsFirstStopError` RED, then GREEN | PASS |
| Concurrent enable/disable serialization | same handoff: `TestEngineAutoMoveConcurrentEnableDisableReconcilesToDisabledWithoutReplicas` RED, then GREEN | PASS |
| Concrete command/default usage | retained original tracer `TestAutoMoveDefaultUsageIsConcreteModeratorCommand` RED against generic behavior, then GREEN | PASS |
| Toggle parsing, off, configure, usage, cancellation, capability failure, registration/role/alias | `automove-command-implementation.md`: seven individual named RED → GREEN cycles | PASS |
| USER-trip repository, case preservation | `automove-h2-implementation.md`: `TestUserTripsReturnsOnlyUserTripsWithOriginalCase` RED for absent method, then GREEN | PASS |
| Context-aware H2 query | H2 handoff: `TestUserTripsPropagatesCanceledContext` RED against non-context query, then GREEN | PASS |
| Saturn-exact SQL literal | prior QA: `TestUserTripsUsesSaturnLoungeTripQuery` RED against the non-literal query, then GREEN | PASS |
| Join happy path/public notice/dynamic typed kick | `automove-join-policy-implementation.md`: `TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica` RED for absent automation, then GREEN; prior QA separately RED→GREEN corrected whisper `true` to public `false` | PASS |
| Join host/gates/cancellation/kick-error logging | join-policy handoff documents individual RED → GREEN for host, inert gates, cancellation, and no-retry logging | PASS |
| Exact-case trip mismatch | `automove-join-tdd-recovery.md`: isolated test RED against `EqualFold`, then GREEN after exact `==` | PASS |
| Query error with returned matching trip sends neither | join recovery: strengthened isolated test RED against ignored error, then GREEN after explicit error return | PASS |
| Notice failure still kicks | join recovery: isolated test RED against temporary return, then GREEN after fall-through restoration | PASS |
| Listener active visibility and semantic → share → log → AutoMove order | `automove-listener-order-implementation.md`: isolated ordering RED for absent constructor, then GREEN; existing malformed-input test covers the shared parse-return boundary | PASS |
| Permanent factory composition/state identity/end-to-end join; temporary inertness | `automove-factory-main-implementation.md`: factory tracer RED for absent option/accessor, then GREEN | PASS |
| Main creates one shared state and complete host registration | factory/main handoff: main tracer RED for absent helper, then GREEN | PASS |

## Fresh execution record

```text
$ go test -race ./internal/core ./internal/listener ./internal/factory ./internal/command ./cmd/zenbot -count=1
ok  zenbot/internal/core      1.673s
ok  zenbot/internal/listener  1.579s
ok  zenbot/internal/factory   1.951s
ok  zenbot/internal/command   39.897s
ok  zenbot/cmd/zenbot         3.869s

$ go test -race ./internal/repository/h2 -run '^(TestUserTrips|TestUserQueries)' -count=1
ok  zenbot/internal/repository/h2  3.384s

$ go test ./... -count=1
PASS: all packages; internal/repository/h2 38.489s

$ go vet ./...
PASS (exit 0)

$ go build ./...
PASS (exit 0)

$ git diff --check
PASS (exit 0; no output)
```

## Coverage (fresh focused-package run)

```text
internal/core           59.1%
internal/listener       50.9%
internal/factory        63.1%
internal/command        71.9%
internal/repository/h2  74.9%
cmd/zenbot              34.6%
```

Coverage is package-wide statement coverage, not an AutoMove-only percentage; the acceptance decision rests on the focused behavior and race suites above.

## Changes and limitations

- QA change: `.hermes/handoffs/automove-final-qa.md` only.
- The repository remains dirty from concurrent migration work; `git diff --check` is clean.
- The generic `saturnCommand` legacy switch still contains an `automove` acceptance fallback, but normal catalog construction resolves `automove` through the concrete `newCommand` case and production registration is capability-gated. The concrete-command tracer proves the reachable path; this vestigial unreachable fallback does not alter the accepted vertical.
- Inbound chat dispatch still deliberately invokes commands with `context.Background()` through `legacyAdapter`; direct policy/controller cancellation behavior is covered, while end-to-end inbound cancellation propagation remains outside this bounded parity slice.
