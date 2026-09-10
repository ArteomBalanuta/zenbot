# `grant` / `access` strict-TDD forensic verdict

## Verdict

**Do not strictly accept core `grant` / `access` behavior as TDD-proven. Recovery is required if strict RED → GREEN provenance is an acceptance criterion.**

This is a provenance verdict, not a functional-regression verdict. The current core behavior is focused-test green, but the record establishes neither a pre-implementation failing tracer nor a sequential RED → GREEN cycle for core tracers 1–5. The only observed/red-recorded slice is the later registration capability gate (tracer 6).

## Exact evidence

### Historical implementation and test provenance

* `48777d8029be8741d430b07559b37352177d793a` (`2026-08-31T10:34:02+03:00`, `feat: migrate Saturn parity foundation`) **introduced together**:
  * `internal/command/identity_commands.go`, including `accessCommand` at lines 97–150;
  * `internal/command/registry.go`, including canonical `access`, aliases `grant,access`, and role `ADMIN`;
  * `internal/command/dispatch_adapter.go`, including conditional concrete `access` registration; and
  * `internal/command/identity_commands_test.go`.
* The test shipped in that same foundation commit, `TestAccessCommandPropagatesGrantFailure`, only establishes the normal-branch error return. It invokes `!grant target admin`; the then-current implementation uppercased the role before parsing, so it does **not** provide a recorded RED run for the later raw case-sensitive parsing behavior.
* `f9079caf4890c67f9e298369a23c42409151def8` (`2026-08-31T19:10:41+03:00`, `feat: advance Saturn parity migration`) changed core access behavior and its tests in one commit:
  * `roleName := strings.ToUpper(strings.TrimSpace(a[1]))` became `roleName := a[1]`;
  * comma handling began dropping trailing empty items and stopped trimming each comma item before the write;
  * the test was corrected to use `ADMIN`, and two tests were added for lowercase rejection and comma/Java-split semantics.
* Git commits and file/blame establish introduction order only. They do not contain focused command output, an intentional failing assertion, or a commit/CI record that demonstrates each test first failed for the missing behavior and then passed after the minimal production change. No historical RED → GREEN evidence was found for catalog/shared dispatch, aliases/singular reply/whisper, input failures, comma quirk, or normal mutation-error behavior.

### Current tracer evidence

* `internal/command/access_command_parity_test.go` is an **untracked dirty-worktree** test file. It contains tracers 1–6.
* The implementation handoff explicitly records that tracers 1–5 “already existed in `HEAD`” and “went green immediately after fixture corrections”; it records only this RED result:

  ```text
  --- FAIL: TestAccessParityTracer6DoesNotExposeWithoutWritableAuthorization
  grant must not be registered without Security.Authorization
  ```

* The current uncommitted change in `internal/command/dispatch_adapter.go` adds `b.Security.Authorization != nil` to the existing registration predicate. That is the minimal GREEN for tracer 6. It is a separate composition/capability defect, not evidence that the access-command core was implemented test-first.
* Current source keeps the existing core in `internal/command/identity_commands.go:97–150`: exact two-argument/nonblank-trip guard, uppercase-only role map, normal `GrantTrip` error propagation, source-shaped comma-to-`USER` behavior, and reply formatting.

## Focused current baseline

Run in the dirty worktree without edits:

```text
go test ./internal/command -run '^TestAccess(ParityTracer|Command)' -count=1
ok   zenbot/internal/command  1.015s

go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message
ok   zenbot/internal/command
ok   zenbot/internal/service
ok   zenbot/internal/repository/h2
ok   zenbot/internal/listener/message

git diff --check
exit 0
```

Thus current behavior is regression-backed; it is not strictly TDD-backed.

## Dirty-safe recovery boundary

Do **not** reset, restore, clean, stash, stage, amend, or modify the current dirty worktree to manufacture RED evidence. The live worktree contains unrelated tracked and untracked migration work.

If strict acceptance is required, perform recovery in a separate clean worktree/branch and limit it to the access slice:

1. Establish a baseline before access implementation (the parent of `48777d8`) or an equivalent isolated baseline that lacks the concrete access implementation and registration.
2. Recover one tracer at a time, recording actual focused RED output before the minimal GREEN change:
   1. catalog / aliases / concrete registration and shared `ADMIN` dispatch;
   2. singular mutation, exact reply, and whisper routing for both aliases;
   3. missing invoker trip / wrong count and lowercase-role no-write/no-reply behavior;
   4. comma targets, `USER` writes, plural reply, and trailing-empty removal;
   5. normal `GrantTrip` error propagation with no success reply.
3. Constrain production edits to the access-specific portions of `internal/command/registry.go`, `internal/command/identity_commands.go`, and `internal/command/dispatch_adapter.go`; constrain test edits to access-focused tests. Do not alter identity resolution, shared authorization policy, repository/schema, transport, agent surfaces, or unrelated command registrations.
4. Keep tracer 6 as its own already-proven registration-capability slice. It need not be reclassified as core behavior and must not be used to infer historical RED evidence for tracers 1–5.
5. After each GREEN, run the individual tracer; at the end run:

   ```sh
   go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message
   git diff --check
   ```

Absent that isolated recovery record, accept the core only as **functionally green / tests-after regression coverage**, not as strict-TDD-proven behavior.
