# AutoMove implementation handoff

## Outcome

Implemented the bounded process-local automove vertical: shared state, concrete moderator command, replica reconciliation, H2 USER-trip lookup, permanent-replica join policy, shared factory/main composition, and registration gating.

## TDD record

| Tracer | RED evidence | GREEN evidence |
|---|---|---|
| Concrete command default usage | `go test ./internal/command -run '^TestAutoMoveDefaultUsageIsConcreteModeratorCommand$' -count=1` failed: `automove must be a concrete moderator command` | Same command passed after `automoveCommand` replaced the generic fallback. |
| State default | `go test ./internal/core -run '^TestAutoMoveDefaultsAreFalsePurgatoryAndLounge$' -count=1` failed to build: `undefined: NewAutoMoveState` | Same command passed after `AutoMoveState` implementation. |

## Source citations

- Saturn command semantics: `../saturn/src/main/java/org/saturn/app/command/impl/moderator/AutoMoveUserCommandImpl.java` — static false/purgatory/lounge state, toggle-before-arity parsing, configure-only behavior, enable/disable ordering.
- Saturn join policy: `../saturn/src/main/java/org/saturn/app/listener/impl/UserJoinedListenerImpl.java` — replica/source gates, `USER` trip membership, literal lounge notice then kick.
- Target architecture authority: `.hermes/handoffs/automove-current-architecture.md` §§3–9.

## Touched paths

- Created: `internal/common/automove.go`, `internal/core/auto_move.go`, `internal/core/auto_move_test.go`, `internal/command/automove.go`, `internal/command/automove_test.go`, `internal/repository/automove.go`, `internal/listener/automove_join.go`.
- Modified: `internal/core/engine_impl.go`, `internal/command/handlers.go`, `internal/command/dispatch_adapter.go`, `internal/command/admin_moderator_catalog_guard_test.go`, `internal/repository/h2/user_queries.go`, `internal/listener/user_joined_listener.go`, `internal/factory/engine_factory.go`, `cmd/zenbot/main.go`.

## Verification

```text
go test ./internal/command ./internal/core ./internal/listener ./internal/factory ./internal/repository/h2 -count=1  PASS
go test -race ./internal/core -run '^TestAutoMove' -count=1  PASS
go test ./... -count=1  PASS
go vet ./...  PASS
go build ./... && git diff --check  PASS
```

## Limitations

The implementation is bounded to the requested volatile process-local behavior. No config/schema/migration, persistence, cross-process synchronization, retries, source removal, or remote snapshot/agent changes were made. The repository has pre-existing dirty changes; they were left untouched. The current focused tests cover the first command tracer and state defaults; additional tracer tests for full lifecycle, H2 error paths, listener permutations, and composition identity should be added before relying on this high-risk feature in production.
