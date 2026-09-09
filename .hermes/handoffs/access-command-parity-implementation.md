# `grant` / `access` Saturn-parity implementation

## Delivered scope

- Tightened the existing concrete `access` registration gate in `internal/command/dispatch_adapter.go` to require:

  ```go
  b != nil && b.Users != nil && b.Users.GroupB != nil &&
  b.Security != nil && b.Security.Authorization != nil
  ```

- This preserves the existing `access` definition (`grant`, `access`; `ADMIN`), shared `DispatchUserCommand` authorization gate, existing `accessCommand` parsing/reply/mutation behavior, and existing H2 `GrantTrip` implementation.
- No dispatch, identity, transport, role-policy, repository/schema/SQL, transaction, or agent surface was changed.

## Tracer record

`internal/command/access_command_parity_test.go` adds focused behavioral coverage.

1. Catalog / shared `ADMIN` dispatch: passes through `UserChatListener`; verifies `grant` is concrete and invokes the shared authorization call before `GrantTrip`.
2. Both aliases / exact singular response / whisper: passes for `grant` and `access`; asserts `ADMIN`, recipient, exact response, and incoming `type:"whisper"` routing.
3. Source input failures: passes for missing trip and wrong argument count with the exact usage response; lowercase `admin` fails with no grant or reply.
4. Comma quirk: passes for `first,second,`; asserts trailing empty removal, `USER` writes, requested-role plural response.
5. Normal mutation error: passes with propagated error and no success reply.
6. Registration negative: initial RED observed:

   ```text
   --- FAIL: TestAccessParityTracer6DoesNotExposeWithoutWritableAuthorization
   grant must not be registered without Security.Authorization
   ```

   Minimal GREEN was the added `b.Security.Authorization != nil` condition. The tracer then passed, including no listener dispatch/output.

Tracers 1–5 target behavior already existed in `HEAD` before this task (including `accessCommand` and the catalog); their newly added focused tests therefore went green immediately after fixture corrections. I did not delete or sabotage existing implementation merely to manufacture false RED evidence. The only production behavior gap found was tracer 6.

## Verification

Passed:

```text
go test ./internal/command -run '^TestAccessParityTracer6DoesNotExposeWithoutWritableAuthorization$' -count=1
go test ./internal/command -run '^TestAccessParityTracer' -count=1
go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/message
go test ./internal/agent/...
git diff --check
```

`git grep -n -E 'grant|access' -- internal/agent` found no grant/access command or tool surface (only unrelated generic accessor terms).

## Files changed by this task

- `internal/command/dispatch_adapter.go`
- `internal/command/access_command_parity_test.go`
- `.hermes/handoffs/access-command-parity-implementation.md`

The repository was already dirty. No reset, checkout, restore, clean, stash, staging, or commit was performed.
