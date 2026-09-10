# DBZ Migration QA Handoff

## Verdict
QA PASS for the exercised DBZ migration slice, backed by the artifact and command results below. No production-code defect was found in the inspected DBZ implementation. The implementation's intentional source-compatibility choices remain documented: non-atomic registration, duplicate names permitted, strength-only free-stat decrement, and process-local enemy state.

## Scope and source review
Reviewed:
- `.hermes/handoffs/dbz-architecture.md`
- `.hermes/handoffs/dbz-implementation.md`
- `internal/repository/dbz.go`
- `internal/repository/h2/dbz.go`
- `internal/repository/h2/dbz_test.go`
- `internal/service/dbz.go`
- `internal/service/dbz_test.go`
- `internal/service/services.go`
- `internal/command/dbz.go`
- `internal/command/dbz_test.go`
- `internal/command/registry.go`
- `internal/command/handlers.go`
- `internal/command/dispatch_adapter.go`
- `internal/command/dispatch_integration_test.go`
- `internal/factory/engine_factory.go`
- `internal/repository/h2/database.go`
- `internal/listener/message/handlers.go`
- `internal/core/engine_impl.go`

Verified implementation details:
- H2 DBZ SQL uses PostgreSQL-wire placeholders. Integer mutation values are explicitly converted and `CAST(... AS INTEGER)` is used, avoiding H2/pgx unknown-parameter typing failures.
- Registration is deliberately independent character insert -> exact-name lookup -> stats insert, preserving duplicate/non-atomic Saturn semantics.
- Level-up is two independent updates; strength decrements free stats, while agility/vitality/energy do not.
- Stats rendering includes the exact field order and final newline; missing reads map to the documented no-row values.
- Enemy spawn/fight state is process-local, mutex-protected, removes only one matching duplicate, and safely no-ops when absent.
- All required DBZ aliases are catalogued with `model.REGULAR`; concrete construction routes DBZ canonicals to `dbzCommand`.
- DBZ registration in `RegisterUserUtilities` is conditional on `Bundle.DBZ != nil`; DBZ-less engines do not expose generic DBZ placeholders.
- Dispatch authorization remains in the existing `DispatchUserCommand` path before command execution.

## QA changes
Only one QA test file was changed by this pass:
- `internal/repository/h2/dbz_test.go`

Added coverage for:
- All four stat mutations and their source quirks (strength consumes points; agility/vitality/energy preserve free stats).
- Duplicate registration permitting two character rows and two stats rows.
- Missing `Stats` and `FreeStats` read semantics.

No Saturn, lifecycle, replica, identity, or unrelated files were modified by this QA pass. The worktree was already intentionally dirty with unrelated migration files; those changes were preserved.

## Actual verification results

Command:
```text
gofmt -w internal/repository/h2/dbz_test.go internal/command/dbz_test.go
```
Result: completed successfully. `gofmt -l` over all intentional DBZ implementation/test files returned no output. `git diff --check` returned no output and exit 0.

Command:
```text
go test -count=1 ./internal/command ./internal/repository/h2 ./internal/service
```
Result:
```text
ok  zenbot/internal/command  2.338s
ok  zenbot/internal/repository/h2  11.538s
ok  zenbot/internal/service  1.175s
```

Command:
```text
go test -race ./...
```
Result: exit 0. All packages passed; command, H2, service, listener, core, agent, model, and repository packages reported `ok` or `[no test files]`. H2 and service race-sensitive tests passed.

Command:
```text
go vet ./...
```
Result: exit 0, no output.

Command:
```text
go build ./...
```
Result: exit 0, no output.

Command:
```text
cmp -s internal/repository/h2/schema-h2.sql resources/schema-h2.sql
```
Result: `schema_consistency_exit=0`; schemas are byte-identical.

## Remaining gaps
- No dedicated injected mid-registration failure test was added; the documented non-atomic policy is verified by source review and duplicate behavior, but failure-between-inserts remains a separate gap.
- Full H2 information-schema assertions for every DBZ column/type/nullability/FK are not DBZ-specific yet.
- Listener-level golden tests for every DBZ alias, whisper payload, unauthorized regular user, and enabled-service end-to-end state mutation remain limited compared with the architecture checklist; existing DBZ metadata/dispatch and authorization suites passed.
- Existing DBZ command behavior intentionally reports registration success even when the service swallows an error, matching Saturn's compatibility contract.

## Final worktree note
The repository remains intentionally dirty. The DBZ-related paths shown by `git status` are the implementation handoff's task-owned files plus the single QA test update above; pre-existing unrelated dirty/untracked paths were not cleaned or reverted.
