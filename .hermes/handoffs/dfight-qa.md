# `dfight` independent QA acceptance

## Verdict

**ACCEPTED.** The current `dfight` vertical matches the Saturn production-source contract within the explicitly documented Zenbot transport adaptation. No scope-owned source/test defect was found; no application code was changed by QA.

## Independent source-to-target findings

Direct comparison against Saturn confirms:

- `DBZFightCommandImpl` declares aliases `dfight`/`df`, authorizes `REGULAR`, takes `requiredArgument(0, "dfight enemy")`, calls `fight(enemy)` before `lvlUp(author)`, emits the fixed acknowledgement, and returns success.
- `UserCommandBaseImpl.requiredArgument` rejects missing/null/trimmed-blank input and `failWithUsage` emits exactly `Example: <prefix>dfight enemy` addressed to the author with the inbound whisper mode. Zenbot's `dfight` missing-argument branch does this before any effects.
- Saturn `DBZImpl.fight` is `enemies.remove(name)`: target `DBZService.Fight` removes the first equal entry only and otherwise no-ops. It does not add combat, RNG, an enemy-existence gate, or a result branch.
- Saturn always invokes `lvlUp` after `fight`, including for a nonexistent enemy. Target always invokes `LevelUp` and intentionally ignores its error. H2 retains source ordering: level update first, free stats +5 second, with no transaction or rollback.
- Saturn successful output is an unaddressed `OutService.enqueueMessageForSending` payload. Target emits the exact fixed payload publicly through `SendChatMessage("", payload, false)`.
- The target-only checked-send adaptation is correct: a public-send error returns `FAILED` after fight/level-up effects; it does not roll back or retry.
- Registration remains DBZ-bundle gated; direct dispatch authorizes the resolved `REGULAR` user before execution; denied and pre-cancelled routes do not produce fight, level-up, or output effects.

## Test/TDD audit

The implementation handoff records two valid behavioral RED→GREEN cycles: public rather than addressed/whisper success delivery, and exact missing-input usage. The remaining tests are accurately labelled post-GREEN regression/provenance tests where the behavior already matched source. The current focused tests independently pass and cover alias/first match, missing enemy unconditional level/free+5, missing usage/no effects, ignored level-up failure, send failure after effects, denied authorization, and cancellation.

## Verification run

All passed in the dirty worktree:

```text
go test -count=1 ./internal/command -run '^(TestDBZAliasesAreRegularAndConcrete|TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState|TestDBZFightAcknowledgesAfterLevelUpErrorAndConsumesMatchingEnemy|TestDBZFightReturnsFailedAfterEffectsWhenPublicSendFails)$'
go test -count=1 ./internal/listener/message -run '^TestDispatchUserCommandDBZFight'
go test -count=1 ./internal/service -run 'TestDBZ'
go test -count=1 ./internal/repository/h2 -run 'TestDBZ'
go test -race -count=1 ./internal/command ./internal/listener/message ./internal/service ./internal/repository/h2
go test -count=1 ./...
go vet ./...
go build ./...
git diff --check && git diff --cached --check
```

No staging, reset, restore, clean, stash, or commit was performed. The worktree remains extensively dirty from parallel migration work; this QA added only this handoff.
