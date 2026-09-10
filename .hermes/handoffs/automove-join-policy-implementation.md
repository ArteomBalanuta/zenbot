# AutoMove join policy/action implementation

## Scope

Implemented only the standalone `listener.AutoMoveJoinAutomation` policy/action seam and the narrow `common.AutoMoveJoinPolicy` contract. It is deliberately **not** composed into `UserJoinedListener`, factory setup, `main`, command handling, core lifecycle, H2, or configuration.

## RED → GREEN record

All commands ran from `/Users/ab/workspace/go-projects/zenbot`.

1. `TestAutoMoveJoinMovesEligibleUserFromEnabledConfiguredReplica`
   - RED: `undefined: AutoMoveJoinAutomation`; listener package build failed.
   - GREEN: `ok zenbot/internal/listener 0.428s` (then re-run after narrowing the initial tracer: `0.373s`).
   - Proves exact public literal `your trip is authorized to join ?lounge, you will be moved to ?lounge` followed by typed `KickNickTo` using the dynamic configured destination.
2. `TestAutoMoveJoinDoesNothingForHost`
   - RED: `host actions: queries=1 notices=1 kicks=1`.
   - GREEN: `ok zenbot/internal/listener 0.357s`.
3. `TestAutoMoveJoinGatesDisabledNonSourceNilAndBlankUser`
   - RED: nil-user panic in `AutoMoveJoinAutomation.OnJoin`.
   - GREEN: `ok zenbot/internal/listener 0.357s`.
   - Disabled and non-source cases were already covered by the pre-existing `EligibleReplica` policy boundary; the same focused tracer verifies no query/notice/kick for both.
4. `TestAutoMoveJoinPreCancelledContextSendsNeither`
   - RED: `cancelled actions: queries=1 notices=1 kicks=1`.
   - GREEN: `ok zenbot/internal/listener 0.370s`.
5. `TestAutoMoveJoinLogsActionErrorWithoutRetry`
   - RED: `action error was not logged: ""`.
   - GREEN: `ok zenbot/internal/listener 0.356s`.
6. Focused behavior checks after their owning tracer paths:
   - `TestAutoMoveJoinNoticeFailureStillKicks`: `ok zenbot/internal/listener 0.250s`.
   - `TestAutoMoveJoinRequiresExactCaseTripMatch` and `TestAutoMoveJoinTripQueryErrorSendsNeither`: `ok zenbot/internal/listener 0.247s`.

## Final verification

```text
$ go test -race ./internal/listener -run '^TestAutoMoveJoin' -count=1
ok  zenbot/internal/listener 1.391s

$ go test ./internal/listener -count=1
ok  zenbot/internal/listener 0.244s

$ git diff --check
(exit 0; no output)
```

## Exclusions

No listener composition or production wiring was added. No raw JSON is built and no H2/session access occurs in `OnJoin`. The policy does not hold locks across repository or outbound operations; it relies on the already-synchronized `EligibleReplica` state snapshot boundary.
