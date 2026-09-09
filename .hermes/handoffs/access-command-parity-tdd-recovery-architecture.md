# `grant` / `access` core strict-TDD recovery architecture

## Decision

Core `grant` / `access` tracers 1–5 are functionally green but are **not strict-TDD-proven**. Recover their provenance only in a new clean worktree rooted at the pre-foundation baseline. Do not use the existing dirty worktree to simulate historical RED evidence.

Tracer 6—the writable-authorization registration capability gate—is accepted as independently RED→GREEN proven. It is not core-command provenance and must neither be rerun as part of core recovery nor used to claim proof for tracers 1–5.

This recovery is access-only. It preserves the approved Saturn `ADMIN` shared-dispatch requirement and deliberately does not invent an identity, authorization, transport, agent, or persistence policy.

## Observed baseline and dirty-worktree safety

At planning time, the active worktree is:

```text
path:   /Users/ab/workspace/go-projects/zenbot
branch: migration/saturn-zenbot-parity
HEAD:   f9079caf4890c67f9e298369a23c42409151def8
state:  37 tracked files modified plus many unrelated untracked files
```

The forensic baseline before the parity-foundation introduction is:

```text
foundation commit: 48777d8029be8741d430b07559b37352177d793a
recovery baseline: 41b8b12e3b0df0ac84f15f6b40bc802f1567c0f3
```

The live worktree currently passes:

```sh
go test ./internal/command -run '^TestAccess(ParityTracer|Command)' -count=1
git diff --check
```

Those are read-only baseline observations, not recovery evidence. Do not `reset`, `restore`, `clean`, `stash`, `checkout`, stage, amend, commit, or otherwise mutate that worktree. In particular, do not touch DBZ files or tests: DBZ is outside this recovery boundary.

## Isolated recovery setup

Create the recovery worktree outside the dirty checkout, with a unique branch and an explicit absolute path. The implementation owner must choose an unused sibling path; the following is the required shape:

```sh
repo=/Users/ab/workspace/go-projects/zenbot
target=/Users/ab/workspace/go-projects/zenbot-access-tdd-recovery
branch=tdd-recovery/access-core

git -C "$repo" worktree add -b "$branch" "$target" 41b8b12e3b0df0ac84f15f6b40bc802f1567c0f3
cd "$target"
git status --short
git rev-parse HEAD
git diff --check
```

Required setup evidence:

* `git status --short` is empty before every RED edit and before every handoff checkpoint.
* `git rev-parse HEAD` initially prints exactly `41b8b12e3b0df0ac84f15f6b40bc802f1567c0f3`.
* The recovery branch contains only access-specific production/test changes and its evidence log; no cherry-pick, patch import, copy from the dirty checkout, or reuse of the existing untracked parity test is allowed.
* Record every command, exit code, and focused test output in a recovery evidence file in the isolated branch (for example, `.hermes/handoffs/access-command-parity-tdd-recovery-evidence.md`). The evidence file is a record, not a substitute for running the test.

Before writing each test, inspect the baseline’s nearest existing command/listener/H2 test fixtures and write only the smallest test needed for that tracer. The test must fail because the behavior is absent—not because a fixture does not compile, a nil setup was accidental, or a test helper is broken. If it errors or passes immediately, repair/discard the test and establish a meaningful RED before any production edit.

## Fixed behavior boundaries

### Authorization and identity

* `grant` and `access` are aliases of canonical `access` with role `model.ADMIN`.
* The only authorization decision is the existing shared `message.DispatchUserCommand` gate: `IsUserAuthorized(c.Author, cmd.GetRole())` before command execution.
* Do not add a command-local authorization check, authenticated-principal model, name/trip/hash binding policy, session identity, user-lookup alteration, or dispatch rewrite.
* Preserve the current Zenbot behavior that `ResolveUserMetadata` selects its active user case-insensitively. It is observed scope, not a target of this recovery.

### Access command behavior

* Exactly two parsed arguments and an author trip are required; preserve existing target handling of blank/whitespace-only trip as absent.
* The exact failure reply is `\n Set your trip first. Example: <prefix>grant 8Wotmg ADMIN`, sent to the message author through the existing reply helper.
* Only exact uppercase Saturn role literals are accepted: `ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, `PEST`. Lowercase/invalid roles fail with no write and no reply.
* A single target calls `Security.Authorization.GrantTrip(ctx, target, requestedRole)`, returns its error as `FAILED`, and emits exactly `\n Granted new Role: ROLE to trip: TARGET` only after success.
* A comma-containing raw target is split on commas; trailing empty elements are removed, leading/interior empty elements remain, and each item is written as `USER` regardless of the requested role. Per-item errors remain ignored; it replies successfully with `\n Granted new Roles: ROLE to trips: [item1 item2]`.
* The existing reply path owns recipient and whisper state. Do not add a sender, alternate whisper flag, or transport route.

### Explicitly excluded changes

Do not alter:

* roles, hierarchy, configured `AdminTrips`/`x` semantics, protected-target or escalation policy;
* repository interfaces, H2 schema/SQL, transaction/retry/error policy, or `SecurityService.AuthorizeTrip` behavior;
* agent commands, agent tool exposure, allowlists, direct-agent submission, or generic `saturnCommand` fallback;
* DBZ, registration, messages, moderation, proxy/whiskey, factory capability design, or unrelated command registration.

The only allowed production seams for core recovery are access-specific portions of:

* `internal/command/registry.go`
* `internal/command/dispatch_adapter.go`
* `internal/command/identity_commands.go`

The only allowed test seams are new or access-focused portions of command/listener/H2 test files needed by the tracer under recovery. Keep baseline tests untouched unless the smallest test-harness extension is required to reach the exact behavior.

## Strict sequential tracer recovery

Do not write all tests first. Run one complete RED→GREEN vertical slice at a time. Do not begin tracer N+1 until tracer N’s focused test is green, its scope/diff is reviewed, and the test output is recorded.

### Tracer 1 — catalog, aliases, concrete registration, shared `ADMIN` gate

**RED test:** Add one listener-to-command test that sends an authorized active author’s `!grant target ADMIN`. It must prove that `grant` resolves to the concrete canonical `access` definition with `model.ADMIN`, and reaches `GrantTrip` only through the pre-existing shared dispatch authorization gate.

**Focused command:**

```sh
go test ./internal/command -run '^TestAccessParityTracer1CatalogDispatchesGrantThroughAdminGate$' -count=1
```

**Expected RED:** the test fails because the baseline lacks concrete access catalog/registration behavior. It must not fail from new identity infrastructure, a transport fixture, or a synthesized authorization policy.

**GREEN change:** Add only the access catalog definition (`access`, aliases `grant,access`, role `ADMIN`) and concrete registration necessary to expose it under the existing writable composition prerequisites. Reuse shared dispatch unchanged.

**GREEN verification:** rerun the exact command; then run `go test ./internal/command -run '^TestAccessParityTracer1CatalogDispatchesGrantThroughAdminGate$' -count=1` again after any cleanup. Review `git diff --check` and `git diff -- internal/command/registry.go internal/command/dispatch_adapter.go` before continuing.

### Tracer 2 — single target, both aliases, exact reply, whisper routing

**RED test:** Add one end-to-end listener tracer with subtests for `grant` and `access`. For each, use an authorized author and whisper input; prove one `GrantTrip(target, ADMIN)` mutation, exact singular reply, recipient equal to the author, and preserved whisper state.

**Focused command:**

```sh
go test ./internal/command -run '^TestAccessParityTracer2AliasesGrantAndWhisperExactReply$' -count=1
```

**Expected RED:** the concrete command cannot yet produce the required single-target behavior/reply through the registered aliases.

**GREEN change:** Introduce only `accessCommand` behavior and its reuse of the existing `reply` helper needed to satisfy this tracer. Do not add a new send path or authorization decision.

**GREEN verification:** rerun the exact command; inspect the scoped diff in `internal/command/identity_commands.go` and the tracer test; run `git diff --check`.

### Tracer 3 — input failures and case-sensitive role parsing

**RED test:** Add the smallest direct command tests that prove each independent outcome:

1. missing invoker trip or wrong argument count returns `FAILED`, makes no grant, and sends the exact `Set your trip first` reply;
2. lowercase `admin` returns `FAILED`, makes no grant, and sends no reply.

Keep the two assertions as separate test/subtest cases so a failure identifies one policy.

**Focused command:**

```sh
go test ./internal/command -run '^TestAccessParityTracer3InputFailuresMatchSaturn$' -count=1
```

**Expected RED:** absent/mismatched input validation or non-source case handling causes the specified assertion to fail.

**GREEN change:** Add only the exact two-argument/trip guard and uppercase-only role mapping. Do not broaden the trip policy beyond the pre-approved blank-as-absent target behavior.

**GREEN verification:** rerun the exact test and `git diff --check`; verify the production diff is confined to access parsing/guard code.

### Tracer 4 — comma target source quirk

**RED test:** Add one direct command tracer for `!access first,second, ADMIN`. It must assert, in order:

* writes `first` and `second` as `USER`, not `ADMIN`;
* returns successful and emits exactly `\n Granted new Roles: ADMIN to trips: [first second]`;
* a trailing comma creates no extra empty write/display item.

Use a purpose-built fake that records every requested trip and role; do not mock away the command behavior.

**Focused command:**

```sh
go test ./internal/command -run '^TestAccessParityTracer4CommaTargetsKeepSaturnUserQuirk$' -count=1
```

**Expected RED:** the baseline has no source-shaped comma behavior.

**GREEN change:** Add only raw-target comma branching, trailing-empty removal, `USER` writes, ignored comma-branch write errors, and the plural reply. Preserve leading/interior blank items; do not trim each comma member.

**GREEN verification:** rerun the exact test and `git diff --check`; inspect the access-only diff for accidental persistence or policy changes.

### Tracer 5 — normal mutation error propagation

**RED test:** Add one direct command test with a fake `GrantTrip` that returns a sentinel normal-branch error for a single target. Assert `FAILED`, `errors.Is(returned, sentinel)`, no success reply, and no false success output. If using H2 for this test instead of a fake, additionally assert no committed partial state; do not alter H2 merely to manufacture an error.

**Focused command:**

```sh
go test ./internal/command -run '^TestAccessParityTracer5NormalGrantErrorDoesNotReply$' -count=1
```

**Expected RED:** the missing core command cannot propagate the mutation error with the required no-reply result.

**GREEN change:** Propagate only the normal single-target `GrantTrip` error from `accessCommand`. Do not swallow it, add retries, change a transaction, or make it match Saturn’s logged/swallowed SQL defect.

**GREEN verification:** rerun the exact test and `git diff --check`; inspect that no repository, schema, service, identity, or transport file changed.

## Per-slice and final verification

After every green slice:

```sh
git status --short
git diff --check
git diff --name-only
git diff -- internal/command/registry.go internal/command/dispatch_adapter.go internal/command/identity_commands.go
```

The status/diff review must show only the current access tracer’s test/evidence file and access-allowed production seams. If unrelated files appear, stop; do not compensate by reverting anything in the dirty original worktree. Fix the isolated recovery branch only after identifying how the extra file entered it.

After tracer 5 green, run all recovered tracers together, then the established affected-package regression set:

```sh
go test ./internal/command -run '^TestAccessParityTracer[1-5]' -count=1
go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message
git diff --check
git status --short
git diff --name-only
```

Acceptance requires all commands to exit zero, no whitespace errors, and a reviewable isolated diff containing only the access recovery tests, access-specific implementation seams, and the evidence log. Current functional green in the dirty worktree cannot replace the recorded five sequential RED→GREEN runs.

## Failure handling

* If a RED test passes immediately, it is tests-after coverage or it targets existing baseline behavior; discard/reframe it without production edits.
* If a RED test errors, correct the fixture/harness until it fails for the intended absent access behavior.
* If a green implementation requires any excluded seam, stop recovery and return for architecture review rather than expanding scope.
* If more than two minimal green attempts fail for one tracer, stop and investigate the actual boundary and test setup; do not pile fixes onto the access command.
* Do not merge this evidence with tracer 6 or claim a historical TDD record. The result is a newly recorded isolated recovery proof for tracers 1–5.
