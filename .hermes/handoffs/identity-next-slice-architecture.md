# Identity next-slice architecture handoff

## Bounded scope

Implement and verify Saturn parity only for four registered identity command families:

| Canonical | Aliases | Required role |
|---|---|---|
| `register` | `reg`, `register` | `MODERATOR` |
| `authorize` | `authorize`, `auth` | `MODERATOR` |
| `access` | `grant`, `access` | `ADMIN` |
| `messages` | `messages`, `lastmessages` | `MODERATOR` |

This slice ends at command dispatch, the existing `UserService`/`SecurityService` façades, and the supplied identity/Group-B H2 seams. It does not redesign global authorization, schemas, or the general Saturn catalog.

## Observed source contract per command

### `register` / `reg`

- **Saturn:** `RegisterUserCommandImpl` requires two parsed arguments; otherwise `failWithUsage("reg merc g0KY09")`. It checks name/trip registration and has four branches: create both (`REGULAR`), attach a new name, attach a new trip, or report both registered. Only a full-create service code of `1` emits `Something went wrong`.
- **Target:** `registerCommand.Execute` in `internal/command/identity_commands.go` has the same four output strings and trims the two inputs. It returns service errors for all lookup/mutation failures and emits `Something went wrong` for every mutation failure.
- **Dispatch/base contract:** Saturn `UserCommandBaseImpl` strips the prefix/command token, whitespace-splits arguments, sends to the author, and preserves whisper. Target `args` removes `ChatMessage.GetArguments()` index zero; `reply` preserves `IsWhisper || Whisper || Type == "whisper"`.

### `authorize` / `auth`

- **Saturn:** `AuthorizeTripCommandImpl` requires one argument, replies ` example: <prefix>auth cmdTV+` when absent, calls `modService.auth(firstArgument)`, then replies ` authorized trip: <trip>`.
- **Target:** `authorizeCommand.Execute` has the same reply text, trims the first argument, then calls `SecurityService.AuthorizeTrip`. `AuthorizeTrip` ignores blank input; with an authorization repository it persists `ADMIN`, returns repository errors, and only then appends a nonduplicate in-memory admin trip.
- **Parity boundary:** Saturn's `modService.auth` implementation is not part of this slice. Treat target persistence/error behavior as the explicit target contract and test it, rather than claiming unobserved Saturn failure behavior.

### `access` / `grant`

- **Saturn:** `AccessUserCommandImpl` requires exactly two arguments and a non-null invoker trip, otherwise emits literal `\\n Set your trip first. Example: <prefix>grant 8Wotmg ADMIN`. `Role.valueOf` is case-sensitive. One target receives the requested role; a comma target is split without trimming, each target receives `USER`, and Java list formatting is returned. Grant errors are not surfaced by the command.
- **Target:** `accessCommand.Execute` checks nonblank trimmed invoker trip, trims/uppercases role input, accepts six roles (`ADMIN`, `MODERATOR`, `TRUSTED`, `USER`, `REGULAR`, `PEST`), and trims comma targets before granting each `USER`. Its multi-target reply currently uses the original split slice while granting a separately trimmed value.

### `messages` / `lastmessages`

- **Saturn:** `LastMessagesCommandImpl` requires trip and integer count, warns then clamps values over 30, reads `lastMessages(null, trip, count)`, renders `\n<author>#<row-trip>: <message>\n`, truncates a value over 200 Java characters with `...`, and Java-escapes the full payload.
- **Target:** `messagesCommand.Execute` validates/trims the count, clamps above 30 with the same warning, prefers `UserService.SaturnLastMessages(ctx, nil, trimmedTrip, count)` when Group B is available, and otherwise uses legacy `LastMessages`. It truncates by Go byte length and renders every row with the requested target trip, because `SaturnLastMessage` omits `Trip`.

## Target symbol/file map

| File | Symbols / ownership in this slice |
|---|---|
| `internal/command/identity_commands.go` | `registerCommand.Execute`, `authorizeCommand.Execute`, `accessCommand.Execute`, `parseRole`, `messagesCommand.Execute`, `escapeJava` |
| `internal/command/identity_commands_test.go` | focused command contract tests and fakes; extend or replace only for this slice |
| `internal/command/dispatch_adapter.go` | `RegisterUserUtilities`, `legacyAdapter.Execute`; registration is conditional on `Users.GroupB` and `Security` |
| `internal/command/handlers.go` | `args`, `reply`, `newCommand`, `commandDefinitionFor`; concrete construction and output semantics |
| `internal/service/services.go` | `UserService` identity wrappers and `SaturnLastMessages` Group-B compatibility façade |
| `internal/service/security_service.go` | `SecurityService.AuthorizeTrip`, `IsAuthorizedContext` |
| `internal/repository/sql_util_group_b.go` | `SaturnLastMessage`, `SqlUtilGroupBRepository.SaturnLastMessages` contract |
| `internal/repository/h2/sql_util_group_b.go` | `Database.SaturnLastMessages`; compatibility SQL/read mapping |
| `internal/repository/h2/identity.go` | registration transactions and legacy `LastMessages` public-history query |

## Exact deltas and mismatch risks

1. **Group-B history parity is incomplete:** `SaturnLastMessage` carries no row trip, so `messagesCommand` cannot render Saturn's `message.trip()`. Add the trip to the compatibility DTO and query/scan it, or explicitly constrain the Group-B query so the requested trip is the only legal rendered trip. The first option is the direct parity design.
2. **Group-B SQL lacks security/determinism:** `Database.SaturnLastMessages` has no `visibility='PUBLIC'` predicate and orders only `created_on DESC`. Align it with `Database.LastMessages`: public-only, exclude `LEFT`/`JOINED`, and `created_on DESC, id DESC`.
3. **History count contract:** Group-B defaults nonpositive counts to 5 but does not impose the command's 30 cap. Keep the command cap; decide and test whether the repository also caps direct callers. Do not interpolate an unvalidated limit.
4. **Text-length mismatch:** target truncates bytes; Saturn truncates Java UTF-16 code units. Preserve the existing target byte behavior only if tests/documented compatibility require it; otherwise change deliberately with multi-byte test coverage, never by incidental slice semantics.
5. **`access` normalization differs from Saturn:** target accepts lowercase/whitespace role and trims comma targets; Saturn does neither. Keep target normalization only as an explicit compatibility improvement; outputs and multi-target `USER` grants remain parity-critical.
6. **`authorize` error behavior differs in observability:** target propagates persistence failure and does not send a success reply. Saturn command always replies after calling `modService.auth`; unobserved service exception behavior must not be invented.
7. **Registration atomicity:** target H2 operations use `WithTx` and ID re-queries. Verify rollback at every failing statement/link; do not replace the transaction based on Saturn's generated-key implementation.
8. **Registration gate:** `RegisterUserUtilities` registers this family only when Group B and Security are present. This is the runtime condition to test; do not expose unrelated Saturn catalog entries.

## TDD vertical RED-GREEN bullets

1. **RED — dispatch/catalog:** prove each alias resolves case-insensitively, `newCommand` produces the four concrete types, and `RegisterUserUtilities` registers them only with `Users.GroupB` plus `Security`. **GREEN:** make the smallest registration/constructor correction required.
2. **RED — register branches:** table-test usage, both new, new name/existing trip, existing name/new trip, both existing, whitespace, lookup failure, and mutation failure including author/whisper output. **GREEN:** preserve exact output/status/error behavior.
3. **RED — authorize persistence:** test absent/blank input, trimmed grant, duplicate in-memory trip, repository error (no success reply/no append), and persisted `ADMIN`. **GREEN:** keep `AuthorizeTrip` ordering and error propagation explicit.
4. **RED — access:** test wrong arity/no sender trip, all six roles, invalid role, single-target repository error, comma targets, target whitespace, `USER` grants for every multi target, and exact replies. **GREEN:** choose the normalization policy above and encode it in the expected outputs.
5. **RED — messages command:** test usage/non-integer, clamp-warning order, Group-B selected over legacy, escaping, empty result, service error, and 200-boundary including non-ASCII. **GREEN:** render row trip and apply the selected truncation semantics.
6. **RED — Group-B H2 history:** write data differing only in visibility, lifecycle message, row trip, and identical timestamp/id; assert returned columns/filter/order/limit. **GREEN:** adjust DTO, interface, query, and scan as one vertical change.
7. **RED — H2 registration transactions:** force insert/link failures and assert no orphan names/trips/links; cover each registration path. **GREEN:** retain `WithTx` and correct only any demonstrated rollback leak.

## Real-H2 test scope

Run focused tests against the actual H2 database/schema, not fakes alone:

- registration inserts, case-insensitive existence checks, link creation, role values, duplicate constraints, and rollback for full registration/new-name/new-trip paths;
- authorization persistence through `AuthorizeTrip` and grant insert/update behavior used by `access`;
- `SaturnLastMessages` and `LastMessages`: nullable fields, public-only visibility, `LEFT`/`JOINED` exclusion, requested name/trip scope, count default/cap decision, and `(created_on,id)` tie order;
- end-to-end legacy registration gate and command reply behavior using a real bundle where practical.

Minimum closure commands after implementation: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`.

## Error and edge behavior

- A canceled command context returns `FAILED` and its context error before side effects.
- Usage/input validation returns `FAILED`, nil error, and only the documented usage/presence reply; invalid `access` role is silent.
- Register lookup/mutation and single-target grant errors return `FAILED` with the underlying error. Register mutation errors also emit `Something went wrong`.
- Authorize repository error returns `FAILED` with no success reply and does not update `AdminTrips`.
- Multi-target access intentionally ignores individual grant errors today; retain only if this is explicitly accepted, otherwise change it as one tested behavior change rather than accidentally.
- History query/scan/service errors return `FAILED` with the underlying error; no fabricated empty successful response.
- Replies must preserve the incoming whisper flag and target the invoking author.

## Scope exclusions

- No production/test changes in this architecture phase.
- No edits to `MIGRATION_PLAN.md` or `.hermes/migration-audit.md`.
- No global role-ordering redesign, command authorization redesign, schema migration, or expansion to non-identity Saturn commands.
- No assumptions about Saturn `modService.auth` internals beyond the observed command call.

## Standard/High risk decision

**Decision: High risk.** This slice writes identity/role records and exposes message history; Group-B currently has a public-visibility gap and non-deterministic tie ordering. Require real-H2 coverage and the full race/vet/build gate before merge. Do not downgrade based solely on passing fake-backed command tests.

## Acceptance criteria

- [ ] All four canonical commands/aliases resolve to the specified role and concrete type; conditional runtime registration is tested.
- [ ] Command outputs, statuses, whisper routing, and errors match the documented contract for all branch matrices.
- [ ] The selected access normalization and history truncation policies are explicitly tested, not accidental.
- [ ] `SaturnLastMessage`/Group-B history can render the row trip (or an approved alternative demonstrably preserves that output contract).
- [ ] Both H2 history paths enforce public visibility, exclude lifecycle rows, and order ties by `created_on DESC, id DESC`.
- [ ] Registration and authorization write paths have real-H2 commit/rollback coverage with no orphan records.
- [ ] `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...` pass after implementation.
- [ ] No files outside the task-owned implementation/test surface change, apart from an explicitly approved dependency/interface update.

## Task-owned files

Architecture-phase ownership is limited to this file:

- `.hermes/handoffs/identity-next-slice-architecture.md`

For the later implementation phase, the expected owned source/test set is:

- `internal/command/identity_commands.go`
- `internal/command/identity_commands_test.go`
- `internal/command/dispatch_adapter.go` (only if a gate test proves a change is necessary)
- `internal/command/handlers.go` (only if a constructor/argument/reply defect is proven)
- `internal/service/services.go`
- `internal/service/security_service.go`
- `internal/repository/sql_util_group_b.go`
- `internal/repository/h2/sql_util_group_b.go`
- `internal/repository/h2/identity.go`
