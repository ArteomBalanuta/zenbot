# S0 Admin/Moderator Catalog Guard — Implementation Handoff

## Scope completed

- Added `internal/command/admin_moderator_catalog_guard_test.go`; no production source was changed.
- The test contains the source-shaped inventory verified against read-only Saturn `develop` at `src/main/java/org/saturn/app/command/impl/{admin,moderator}`:
  - 11 admin classes
  - 23 moderator classes
  - 34 scoped command definitions total
- It compares every source class's canonical target, exact ordered aliases, and role with `RegisterAll`, and rejects omitted, duplicate, or extra ADMIN/MODERATOR definitions.
- It instantiates every scoped command through `newCommand` and bounds `*saturnCommand` generic fallback to the exact known transitional set. A newly generic routed scoped command fails the guard; replacing a fallback requires removing that canonical from the allowlist and adding its behavior-level slice test.

## Current bounded fallback set (not parity approval)

`mine`, `prefix`, `replica`, `replicaoff`, `replicastatus`, `restart`, `shutdown`, `sql`, `whiskey`, `automove`, `captcha`, `color`, `deauthorize`, `flair`, `mute`, `nuke`, `overflow`, `resurrect`, `shadowbanlist`, `shadowban`, `unmute`, `unshadowban`.

The guard deliberately distinguishes these current transitional routes from concrete handler construction. It does not claim behavior parity for them.

## Test evidence

- Focused guard:
  `go test ./internal/command -run 'TestAdminModeratorCatalog(MatchesSaturnSource|GenericFallbackIsExplicitlyBounded)$' -count=1 -v`
  — PASS.
- Command package:
  `go test ./internal/command -count=1`
  — FAILS in concurrent S5 work's `TestActivityCommandPreservesAliasesRoleFirstArgumentAndWhisper`: its expected rendered activity text does not match the received text. The focused S0 guard passes in that same package checkout.
- Full suite:
  `go test ./... -count=1`
  — FAILS on the same command activity test and concurrent S5 `internal/repository/h2` `TestActivityStatsUsesSourceCaseInsensitiveTripQueryAndOrdersWeekHour` (returned three case variants where the test expected one). The focused S0 guard passes; all failures are outside S0-owned files.

## Boundaries preserved

- Did not modify production files, existing semantic/reversal/last-online work, configuration, plans, audits, or Saturn.
- Did not stage, commit, or push.
- Pre-task dirty files were recorded before the S0 edit; final diff/status verification is required after this handoff write.
