# Row #324 Group B — Bounded Acceptance

## Verdict

**ACCEPTED: authorized Group B only.** This acceptance records the bounded five-constant compatibility slice for row **#324**, based on the implementation handoff and independent QA PASS. It is not acceptance of any broader migration work.

**Full row #324 remains NOT COMPLETE. The overall Saturn-to-Zenbot migration remains NOT COMPLETE.** Group A, Group C, and all other migration rows retain their separate status and are not implicitly accepted here.

## Authorized constants

Only these five Saturn `SqlUtil` constants are accepted in this slice:

1. `DELETE_TRIP_NAMES`
2. `DELETE_TRIP`
3. `DELETE_NAME`
4. `SELECT_NAME_TRIP_REGISTERED`
5. `SELECT_LAST_N_MESSAGES`

The SQL was source-transcribed from the read-only Saturn implementation at `src/main/java/org/saturn/app/util/SqlUtil.java` and contract-tested. No Saturn source was modified.

## Exact Group B target files and artifacts

### Implementation target files

- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/sql_util_row324_group_b_test.go`

No other application source, schema, caller, command, listener, provider, router, transport, agent, or production-registration file is part of this accepted slice.

### Evidence artifacts

- `.hermes/handoffs/sql-util-group-b-authorized-architecture.md` — authorized architecture and boundaries.
- `.hermes/handoffs/sql-util-group-b-implementation.md` — implementation and execution evidence.
- `.hermes/handoffs/sql-util-group-b-qa.md` — independent QA handoff with **PASS (bounded Group B only)**.
- `.hermes/handoffs/sql-util-group-b-acceptance.md` — this bounded acceptance record.

## Accepted implementation behavior

- The five constants are preserved as package-bounded SQL definitions and checked against Saturn semantics.
- `DeleteResult` reports separate affected-row fields: `TripNamesRows`, `TripRows`, and `NameRows`.
- `DeleteIdentity` is fail-closed without the package-private authorization capability. It runs the three delete statements atomically in Saturn order: link rows, trip parent, then name parent.
- Any statement failure returns a zero result and rolls back all prior statements; no partial delete is reported as success.
- `DELETE_TRIP_NAMES` retains Saturn's broad `trip = ? OR name = ?` link scope. Parent deletes retain exact single-column equality scope.
- Blank, absent, and injection-like identity values remain parameter data and are no-op results under the tested behavior.
- `SaturnRegisteredUsers` is a separate `Name,Trip` result shape with `DISTINCT` and `Trip DESC`; existing `RegisteredUsers` remains unchanged.
- `SaturnLastMessages` is a separate three-field result shape `(Name, Message, CreatedOn)`, accepts nullable name input, defaults non-positive count to `5`, excludes only `LEFT` and `JOINED`, orders by `created_on DESC`, and does not add Zenbot's `PUBLIC` predicate or `id DESC` tie-break.
- The existing public-only rich `LastMessages` path remains untouched and is not fed Saturn-shaped results.
- Identity values are parameter-bound. The H2 PostgreSQL-wire limitation is handled by using `$n` placeholders and rendering only the validated integer limit expression; no identity/name/trip value is interpolated.

## Actual passing gates

The implementation and independent QA handoffs record these actual results:

### TDD and focused Group B gates

```text
go test ./internal/repository/h2 -run 'TestGroupB' -count=1
ok  zenbot/internal/repository/h2  4.275s

go test -race ./internal/repository/h2 -run 'TestGroupB' -count=1
ok  zenbot/internal/repository/h2  5.282s
```

The focused tests cover exact constants, authorization denial/no mutation, typed affected rows, OR link scope, unrelated-row preservation, exact parent scope, absent/blank/injection-like input, injected mid-transaction rollback, Saturn projection/order, nullable-name/default count, event filtering/order, and separation from existing public-only rich history.

### Repository-wide gates

```text
go test ./... -count=1
PASS — all packages green; h2 package ok 18.708s

go test -race ./... -count=1
PASS — all packages green; h2 package ok 18.914s

 go vet ./...
PASS — no output, exit status 0

go build ./...
PASS — exit status 0

gofmt -l .
PASS — no output

git diff --check
PASS — no output

Untracked Group B whitespace check via git diff --no-index --check
PASS — no whitespace errors
```

The race run emitted the existing macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`) but exited with status 0. Existing focused identity/history, user-query, and bounded Group A tests remained green as part of the full suite.

## Security, result-shape, and transaction boundaries

### Security boundary

The Saturn SQL contains no authorization predicates. The accepted delete operation therefore requires an already-authorized caller/context and fails closed when the capability is absent. The compatibility seam is intentionally unwired: no production caller can mint the package-private capability, and no raw delete primitive is exposed through `agent/sql`, commands, listeners, generic database callers, or the existing service path.

The accepted Saturn last-message read is separately named and must not be substituted for Zenbot's existing public-only history API or used to disclose whisper/history rows. No authorization policy, command behavior, or production registration was invented by this slice.

### Result shapes

- Delete results are typed and per-statement (`TripNamesRows`, `TripRows`, `NameRows`), not Saturn's untyped `0/1` convention.
- Registered-user compatibility results are explicitly `Name,Trip`; existing Zenbot `RegisteredUsers` remains `Trip,Name`.
- Last-message compatibility results are exactly the restricted three-field shape `(Name,Message,CreatedOn)`; they are not coerced into rich `model.Message` values.
- Saturn's equal-timestamp ordering remains unspecified because only `created_on DESC` is accepted for exact parity.

### Transaction boundary

The composite delete is one transaction with required links-first ordering because `trip_names` has foreign keys without cascade. Commit occurs only after all three statements succeed. Any error, including an injected mid-sequence failure or parent/FK failure, rolls back all prior mutations and returns a zero result.

## Explicit exclusions and limitations

Not accepted or changed here:

- Group A beyond its separately bounded acceptance.
- Group C.
- Row **#325** (`Util`).
- Full row #324 beyond these five Group B constants/contracts.
- `internal/agent/sql` policy changes.
- New commands, listeners, factories, providers, routers, transports, remote-room, Whiskey work, or unrelated production registration.
- Rewriting existing `RegisteredUsers` or `LastMessages` contracts/callers.
- Authorization-policy design or service-wide error mapping.
- Schema changes or visibility migration.
- Any Saturn behavior not evidenced by the cited constants/callers.
- Public standalone delete primitives.
- Saturn equal-timestamp tie ordering beyond `created_on DESC`.

The compatibility interface remains deliberately unwired, so this acceptance does not claim an authorized production service path exists.

## Preservation and changed-file scope

- No Saturn source was modified; the Saturn `SqlUtil.java` preservation check was clean.
- Protected `MIGRATION_PLAN.md` and `.hermes/migration-audit.md` were unchanged. QA recorded their observed hashes as:
  - `MIGRATION_PLAN.md`: `a4a2bfadba585ea0fd7e67208f8f320da76a6c72`
  - `.hermes/migration-audit.md`: `4970d4f31f52b2d84d5928a348f04b7cea1e86e3`
- Target schema files were unchanged.
- Pre-existing dirty, staged, and untracked work was preserved and is not attributed to this acceptance.
- This task adds only this handoff artifact; it does not edit application source or protected documents.

## Final status statement

**Bounded acceptance: PASS for row #324 Group B only.**

**Full row #324: NOT COMPLETE.**

**Overall Saturn-to-Zenbot migration: NOT COMPLETE.**
