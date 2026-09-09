# DBZ help parity: bounded implementation specification

## Decision

[RECOMMENDED] Implement **only `dbzhelp`** as the next DBZ vertical. It is fixed-text output: no database read/write, enemy list, character registration, combat, RNG, access/grant principal work, proxy/Whiskey, raw SQL, or semantic moderation.

The current Zenbot branch is observably mismatched with Saturn:

1. Zenbot registers `dbzhelp` only when `Bundle.DBZ` exists; Saturn's `DBZHelpCommandImpl` has no DBZ-service dependency.
2. Zenbot's shared `reply` addresses the invoker and preserves inbound whisper state. Saturn calls `OutService.enqueueMessageForSending(message)` with **one argument**, so it is unaddressed/public and does not inspect inbound whisper state.
3. Zenbot supplies literal `\\n` sequences and an actual U+2009 thin space. Saturn `StringEscapeUtils.escapeJava` produces escaped newlines and `\\u2009`; `OutService.normalizeForChatPayload` then converts only `\\n` to physical newlines, leaving the six-character `\\u2009` sequence intact.

This specification corrects those three differences without changing shared authorization or delivery infrastructure.

## Source contract

[OBSERVED] `../../projects/saturn/src/main/java/org/saturn/app/command/impl/dbz/DBZHelpCommandImpl.java`:

- aliases: `dbzhelp`, `dbz`, `dhelp` (`@CommandAliases`, line 17);
- role: `Role.REGULAR` (lines 24–27);
- arguments: parsed author/trip locals are unused; command arguments are not read; therefore all arguments are ignored;
- status: always `SUCCESSFUL` after enqueue (lines 30–52), absent an unchecked output-queue exception;
- delivery: `engine.outService.enqueueMessageForSending(StringEscapeUtils.escapeJava(dbzhelp))` (line 49), the unaddressed overload.

[OBSERVED] `OutService.enqueueMessageForSending(String)` in `../../projects/saturn/src/main/java/org/saturn/app/service/impl/OutService.java:35-43` calls `normalizeForChatPayload`. That normalizer replaces `\\n` with a real LF (`:85-91`). It does not decode `\\u2009`. This is the source’s final queued chat-text contract.

### Exact final source payload

The following representation is deliberately Go/JSON-style: `\n` denotes one actual line-feed; `\\u2009` denotes the six visible ASCII characters backslash-u-2-0-0-9. The final newline is required.

```text
This is a DBZ universe text based game.\nMain mechanics:\n/train, - training your char in order to level up and gain point (stats)\n/fight <nick>, - fight against a player\n/claim - claim an item that just spawned\n\u2009\n/stats - displays character stats\n/strength <int> - add a point into str\n/agility <int> - add a point into agility\n/vitality <int> - add a point into vitality\n/energy <int> - add a point into energy\n
```

There is no leading newline, no prefix substitution, no recipient address, and no `".\n"` whisper separator.

## Target map and interfaces

| Target seam | Observed responsibility | Required bounded change |
|---|---|---|
| `internal/command/registry.go`, `RegisterAll` | Catalog has `def("dbzhelp", []string{"dbzhelp", "dbz", "dhelp"}, model.REGULAR)` | No change; it already matches aliases/role. |
| `internal/command/dispatch_adapter.go`, `RegisterUserUtilitiesWithDirectAgent` | Exposes all six DBZ commands only when `bundle(e).DBZ != nil` | Expose only `dbzhelp` unconditionally with concrete utilities; leave mutable/read DBZ commands conditional. This shared dirty file needs owner coordination immediately before editing. |
| `internal/command/handlers.go`, `newCommand` | Maps `dbzhelp` to `*dbzCommand`; `reply` is recipient/whisper preserving | No shared-helper change. Retain construction mapping. |
| `internal/command/dbz.go`, `(*dbzCommand).Execute` | Performs a DBZ bundle guard before switch; current `dbzhelp` calls `reply` with wrong text/delivery | Handle `dbzhelp` after context cancellation but before DBZ-bundle acquisition; send its source-shaped text using `Engine.SendChatMessage("", payload, false)`, return failure only if that call errors. Other DBZ cases remain as-is. |
| `internal/common/engine.go`, `Engine.SendChatMessage` | Narrow output interface | Reuse existing interface; no new interface. |
| `internal/core/engine_impl.go`, `SendChatMessage` | Empty author and `false` create unaddressed public output | No change. |
| `internal/listener/message/handlers.go`, `DispatchUserCommand.Handle` | Resolves alias, then authorizes `REGULAR` before executing | No change. Authorization remains a shared security boundary. |
| `internal/listener/message/dbz_dispatch_test.go` | Existing real-H2 DBZ stats tracer fixture (untracked dirty worktree file) | Do not extend it for this no-DB test; create a focused command-level test only if no safe listener test harness exists without borrowing that owned fixture. |
| `internal/command/dbz_test.go` | Existing DBZ aliases/role/construction and strength test | Preferred test home; expand narrowly. |

### Proposed execution

```text
inbound !dbzhelp / !dbz / !dhelp (any arguments, public or whisper)
  -> DispatchUserCommand authorizes resolved active user for REGULAR
  -> legacyAdapter.ExecuteContext
  -> dbzCommand.Execute
  -> special dbzhelp branch (no Bundle.DBZ access)
  -> Engine.SendChatMessage("", exactSourcePayload, false)
```

The shared listener’s authorization occurs before command execution. Registration without DBZ state must not be interpreted as authorization broadening: unauthenticated/denied users remain denied and produce the existing shared unauthorized behavior.

## Strict tracer TDD sequence

**Precondition for every unit:** capture `git status --short`, run `git diff --check && git diff --cached --check`, inspect the relevant local hunk, and coordinate before touching dirty shared `dispatch_adapter.go`. Never stage, commit, reset, checkout, restore, stash, or bulk-format.

Do not write a batch of tests. Complete one RED and its minimal GREEN before starting the next.

### 1. Conditional-registration removal

- **Test:** `TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState`
- **File:** `internal/command/dbz_test.go`
- **Arrange:** a command engine stub with `ServiceBundle() == nil`; call normal `RegisterUserUtilities`; inspect enabled commands.
- **Assert:** exactly the three aliases `dbzhelp`, `dbz`, `dhelp` resolve to one regular, concrete DBZ-help definition. Assert a mutable DBZ alias such as `dbzstats` is absent, proving the slice does not accidentally expose the DBZ family.
- **RED command:**
  ```sh
  go test -count=1 ./internal/command -run '^TestRegisterUserUtilitiesRegistersDBZHelpWithoutDBZState$'
  ```
- **Intended behavioral failure now:** aliases are absent because `dbzhelp` is still behind `b.DBZ != nil`.
- **Minimal GREEN:** add only `dbzhelp` to the unconditional concrete canonical list and remove only `dbzhelp` from the DBZ-conditional append. Do not alter registry metadata, role policy, factories, or other DBZ exposure.
- **GREEN command:** same focused command; require PASS.

### 2. One public exact-payload tracer

- **Test:** `TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload`
- **File:** `internal/command/dbz_test.go`
- **Arrange:** no DBZ bundle; construct the normal `dhelp` definition with `ChatMessage{Name: "goku", Text: "!dhelp ignored", IsWhisper: true}` and a recording `SendChatMessage` engine.
- **Assert:** `SUCCESSFUL`, nil error, exactly one call whose recipient is `""`, `whisper == false`, and whose text is byte-for-byte the payload above (actual LFs, literal `\\u2009`, final LF). This one tracer proves alias construction, ignored arguments, no DBZ prerequisite, exact escaping/newlines, and source delivery despite whispered inbound transport.
- **RED command:**
  ```sh
  go test -count=1 ./internal/command -run '^TestDBZHelpIgnoresArgumentsAndPublishesExactSaturnPayload$'
  ```
- **Intended behavioral failure after Unit 1 GREEN:** zero output because the current DBZ-bundle guard still runs before the `dbzhelp` case. If a temporary local implementation has already moved the branch, the test must instead fail on its current addressed/whispered and incorrectly escaped payload; record the actual mismatch. A fixture compile/setup failure is not a valid RED.
- **Minimal GREEN:** define one DBZ-help payload constant using physical `\n` and literal `\\u2009`; add only a pre-bundle `dbzhelp` branch that calls `SendChatMessage("", payload, false)` and propagates its error/status. Remove the old switch case. Do not use `reply`, `SendWhisperMessage`, `SendAddressedMessage`, service/repository code, or a new transport abstraction.
- **GREEN command:** same focused command; require PASS.

### 3. Focused regression package and dirty-state validation

Only after both GREEN results:

```sh
go test -count=1 ./internal/command ./internal/listener/message
git diff --check && git diff --cached --check
git status --short
```

This is not another RED/GREEN unit. It verifies that command behavior and the shared dispatch package still pass, and that the only intentional new artifacts are the two tracer tests and minimal `dbzhelp` production edits. Existing unrelated dirt is a preservation baseline, not permission to normalize it.

## Risks and scope boundaries

| Area | Complexity | Risk | Control |
|---|---:|---|---|
| Exact text/escaping | Low | High observable-parity risk: U+2009 vs literal `\\u2009`, trailing LF | Byte-exact test; no generic normalizer. |
| Registration | Low | Medium merge risk: dirty shared adapter | Isolate one literal canonical move; coordinate first. |
| Delivery semantics | Low | High user-visible risk: targeted whisper vs unaddressed public source behavior | Assert recipient empty and `false`; do not modify shared `reply`. |
| Authorization | No implementation complexity | High security risk | Preserve `REGULAR` catalog role and listener ordering; no identity/active-user/security edit. |
| Mutable DBZ mechanics | Excluded | High state/concurrency risk | No registration, stats, strength, fight, spawn, schema, service, repository, or H2 changes. |

Explicit exclusions: access/grant principal changes; raw SQL command behavior; proxy/Whiskey; semantic moderation; any mutable DBZ mechanics; DBZ schema/service/repository changes; role-policy broadening; identity inference; factories; transport/helper refactors; staging/commits; cleanup of unrelated dirt.

## Recommended developer tier

**Senior Go maintainer (independent implementation), with shared-worktree coordination.** The code change is small, but exact wire-visible escaping/delivery and the dirty `dispatch_adapter.go` boundary make this unsuitable for an unsupervised junior change. No database or Saturn runtime specialist is needed.

## Evidence and current baseline

[TEST-BACKED] This architecture pass ran:

```text
go test -count=1 ./internal/command ./internal/listener/message
ok   zenbot/internal/command
ok   zenbot/internal/listener/message

git diff --check && git diff --cached --check
exit 0; no output
```

[OBSERVED] `git diff --numstat` shows no current diff in `internal/command/dbz.go`; `dispatch_adapter.go`, `handlers.go`, and `registry.go` are already dirty due to unrelated work. No application source, tests, staging area, or commits were modified by this architecture task.
