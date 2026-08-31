# H2 fixture isolation implementation handoff

## Outcome

Implemented the AutoPort fixture-isolation correction on `migration/saturn-zenbot-parity` without staging, committing, or pushing. Existing identity-slice modifications were preserved and not changed.

## Implementation

- Added opt-in `h2.Config.AutoPort`; the legacy `Port == 0` assignment remains `5435` when `AutoPort` is false.
- AutoPort launches an owned H2 child with `-pgPort 0`, never pre-dials/adopts a listener, drains both child output streams, strictly accepts `PG server running at pg://<loopback-host>:<nonzero-port> (only local connections)`, and uses the discovered `Server.Addr()` in the PG DSN.
- Startup parse, process-exit, context, and readiness failures stop the owned process with bounded interrupt/kill-and-wait handling.
- `Stop` is idempotent and waits for the exact owned child. It does nothing for the legacy adopted-listener case (`cmd == nil`).
- Added `internal/testutil/h2fixture.Open`, with per-call temporary base directory, H2 jar validation/fallback, loopback AutoPort configuration, stem validation, and cleanup that reports `Close` errors.
- Converted the external real-H2 fixture constructors to the shared utility. The internal H2 package’s `openTestDB` and `TestRealH2PostgresWire` use the equivalent AutoPort lifecycle directly because importing a utility package that imports `internal/repository/h2` back into same-package H2 tests creates a Go test import cycle. All `openTestDB` consumers now use owned AutoPort fixtures and error-reporting cleanup.
- Added AutoPort regression tests for concurrent isolation, sibling close ownership, and ignoring an occupied legacy port. The regression tests use the central fixture utility.

## Strict TDD record

### RED

First added `TestConfigExposesAutoPortOptIn` before the field existed:

```text
=== RUN   TestConfigExposesAutoPortOptIn
    database_autoport_test.go:11: Config must expose an explicit AutoPort opt-in
--- FAIL: TestConfigExposesAutoPortOptIn (0.00s)
FAIL
FAIL    zenbot/internal/repository/h2    0.494s
FAIL
```

After the opt-in field was green, added the three behavior regressions before AutoPort lifecycle implementation. They failed meaningfully against the old fixed/default/adoption path:

```text
=== RUN   TestAutoPortFixturesAreConcurrentAndIsolated
    database_autoport_regression_test.go:47: fixture addresses match: 127.0.0.1:5435
--- FAIL: TestAutoPortFixturesAreConcurrentAndIsolated (0.68s)
=== RUN   TestAutoPortFixtureCloseDoesNotInterruptSibling
    database_autoport_regression_test.go:84: exit status 130
--- FAIL: TestAutoPortFixtureCloseDoesNotInterruptSibling (0.69s)
=== RUN   TestAutoPortDoesNotAdoptOccupiedLegacyPort
panic: test timed out after 45s
```

The occupied-listener timeout occurred because legacy adoption attempted a PG handshake with the test TCP listener; it was the expected pre-fix behavior, not a compilation/setup failure.

### GREEN

```text
=== RUN   TestAutoPortFixturesAreConcurrentAndIsolated
--- PASS: TestAutoPortFixturesAreConcurrentAndIsolated (1.43s)
=== RUN   TestAutoPortFixtureCloseDoesNotInterruptSibling
--- PASS: TestAutoPortFixtureCloseDoesNotInterruptSibling (1.32s)
=== RUN   TestAutoPortDoesNotAdoptOccupiedLegacyPort
--- PASS: TestAutoPortDoesNotAdoptOccupiedLegacyPort (0.68s)
PASS
ok      zenbot/internal/repository/h2    3.951s
```

## Verification outputs

```text
$ go test ./internal/repository/h2 -run 'TestAutoPort(FixturesAreConcurrentAndIsolated|FixtureCloseDoesNotInterruptSibling|DoesNotAdoptOccupiedLegacyPort)' -count=25 -parallel=8 -timeout=2m
ok      zenbot/internal/repository/h2    81.199s

$ go test ./internal/repository/h2 -count=10 -parallel=8 -timeout=5m
ok      zenbot/internal/repository/h2    237.331s

$ go test ./internal/command ./internal/service -count=5 -parallel=8 -timeout=5m
ok      zenbot/internal/command  25.860s
ok      zenbot/internal/service  10.187s

$ go test ./... -count=1 -parallel=8 -timeout=10m
ok      zenbot/internal/command          6.716s
ok      zenbot/internal/repository/h2    25.440s
ok      zenbot/internal/service          3.650s
# all remaining packages passed; test-only h2fixture package has no tests

$ go test -race ./... -parallel=8 -timeout=15m
ok      zenbot/internal/repository/h2    24.474s
ok      zenbot/internal/command          14.347s
ok      zenbot/internal/service          3.756s
# all remaining packages passed
# linker emitted a pre-existing-looking macOS LC_DYSYMTAB warning while linking internal/agent/sql.test; exit status was 0 and no race was reported.

$ go vet ./...
# exit status 0; no output

$ go build ./...
# exit status 0; no output

$ git diff --check
# exit status 0; no output
```

A post-verification process/listener check produced no H2 Java server/listener output.

## Task-owned files created or modified

- `internal/repository/h2/database.go`
- `internal/repository/h2/audit_test.go`
- `internal/repository/h2/database_test.go`
- `internal/repository/h2/database_autoport_test.go` (new)
- `internal/repository/h2/database_autoport_regression_test.go` (new)
- `internal/testutil/h2fixture/h2fixture.go` (new)
- `internal/command/users_nicks_integration_test.go`
- `internal/command/note_notes_parity_test.go`
- `internal/command/mail_group_c_parity_test.go`
- `internal/service/mail_group_c_parity_test.go`

## Remaining caveat

The architecture’s literal request for same-package H2 tests to import `h2fixture.Open` is not buildable in Go: `h2fixture` returns `*h2.Database`, so it imports `h2`; H2’s same-package tests importing it creates `h2 -> h2fixture -> h2`. The internal wrapper is therefore an intentionally minimal equivalent AutoPort implementation. The external five fixture sites and the new external-package regression tests use the centralized helper; all six fixture families have owned AutoPort isolation in the verified gates.
