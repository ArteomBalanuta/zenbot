# S5 Activity Implementation Handoff

## Scope and boundary

Implemented only the Saturn moderator activity vertical from `ActivityCommandImpl` and `SQLServiceImpl`. Accepted identity/history and current last-online/semantic-moderation staging were not rewritten. No config, plan, audit, Saturn source, staging, commit, or push operation was performed.

## Source-grounded contract

- Saturn source: `../projects/saturn/src/main/java/org/saturn/app/command/impl/moderator/ActivityCommandImpl.java`
  - role: `MODERATOR`
  - aliases: `active`, `activity`
  - required, trimmed first argument; missing target replies `Example: <prefix>active 8Wotmg` and returns `FAILED`
  - successful reply: `Stats: \\n` plus `SQLService.executeActivityStats` output, preserving inbound whisper mode.
- Renderer/query source: `../projects/saturn/src/main/java/org/saturn/app/service/impl/SQLServiceImpl.java:executeActivityStats` and `service/impl/util/TableGenerator.java`.
  - no rows render `No activity found.`
  - nonempty rows render a literal `\\n```Text\\n` prefix, the TableGenerator ASCII table, then a literal backslash-n, one space, and three backticks.
  - H2 2.3.232 PostgreSQL-wire compatibility requires `DAY_OF_WEEK(...)` in place of source `EXTRACT(DAY_OF_WEEK FROM ...)`, and quoted `"hour"` because `HOUR` is reserved. The CTE grouping, exact-case grouping, case-insensitive final trip filter, day mapping, ordering, and percentage expression remain source-shaped.

## Implementation

New files:

- `internal/repository/activity.go` — bounded `ActivityRepository` and string-preserving row contract.
- `internal/repository/h2/activity.go` — parameterized real-H2 CTE query (`$1`) and row scan.
- `internal/service/activity.go` — `ActivityService`; source no-row text and Java `TableGenerator`-compatible renderer. UTF-16-width calculation preserves Java `String.length()` padding behavior.
- `internal/command/activity.go` — concrete context-aware command.
- `internal/repository/h2/activity_test.go`
- `internal/service/activity_test.go`
- `internal/command/activity_test.go`

Narrow integration edits:

- `internal/service/services.go` — adds `Bundle.Activity`.
- `internal/factory/engine_factory.go` — attaches `ActivityService` only when the repository implements `ActivityRepository`.
- `internal/command/handlers.go` — maps canonical `active` to `activityCommand`.
- `internal/command/dispatch_adapter.go` — registers the activity aliases only when the Activity service/repository is available.

## Tests and verification

TDD RED was observed before production implementation: the new tests failed on absent `ActivityStat`, `ActivityService`, `Bundle.Activity`, and `Database.ActivityStats` symbols.

Focused GREEN:

```text
go test ./internal/service ./internal/repository/h2 ./internal/command ./internal/factory -run 'Test(Activity|RegisterUserUtilitiesAddsActivity|NewEngine)' -count=1 -v
PASS: service activity rendering/error/no-row tests
PASS: real-H2 activity query, case-insensitive filter, exact-case grouping/order, absent target
PASS: command alias/role/first argument/whisper/usage/error/conditional registration
PASS: factory package focused checks
```

Required final gates:

```text
go test ./...
PASS (including internal/command, internal/repository/h2, internal/service, internal/factory)

git diff --check
PASS
```

## Working-tree note

The repository was already dirty at start, including accepted identity/history/last-online and semantic moderation work. Final full-suite execution passed with those concurrent baseline changes. This slice owns only the activity files and the narrow Activity integration hunks above; it does not claim ownership of adjacent pre-existing hunks in `handlers.go`, `dispatch_adapter.go`, or `services.go`.
