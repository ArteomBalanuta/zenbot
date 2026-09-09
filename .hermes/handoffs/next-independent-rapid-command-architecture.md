# Next Independent Rapid Command Architecture: `lastonline` / `lastseen`

**Decision:** deliver exactly the Saturn regular-user `lastonline` command (aliases `lastonline`, `seen`, `last`, `online`, `lastseen`) as the next isolated parity slice. Route implementation to **@developer**. This is a bounded read-only command with an existing Zenbot catalog, message-history schema, H2 repository, `UserService`, utility date formatter, service bundle, command adapter, and listener registration seam.

## Why this slice now

[OBSERVED] `MIGRATION_PLAN.md` lines 13–25 explicitly prioritizes missing command behavior and names **user last-online**. Zenbot already advertises `lastseen <name>` in `internal/command/help.go`, defines all five aliases in `internal/command/registry.go: RegisterAll`, and has a source-compatible `messages` table plus query/service wiring for related `users` and `nicks` reads. It is currently not live: `RegisterUserUtilitiesWithDirectAgent` in `internal/command/dispatch_adapter.go` does not include `lastonline`; the catalog fallback in `internal/command/registry.go: saturnCommand.Execute` emits a placeholder `user: <argument>` if manually constructed.

[RECOMMENDED] Do not select an admin/moderation/lifecycle/remote-room slice next. Those require stateful runtime/protocol seams. Do not activate the catalog placeholder: that would violate the migration plan's prohibition on generic acknowledgements.

## Saturn evidence

### Command contract and output

[OBSERVED] `src/main/java/org/saturn/app/command/impl/user/LastOnlineUserCommandImpl.java: LastOnlineUserCommandImpl`:

- declares aliases exactly `{"lastonline", "seen", "last", "online", "lastseen"}`;
- returns `Role.REGULAR` from `getAuthorizedRole()`;
- takes only the first parsed argument; `IdentityUtil.normalizeNickTarget` trims whitespace and one leading `@`, and rejects blank input;
- on omitted/blank target, enqueues `"\\n Example: " + prefix + "lastseen merc"` addressed to the caller and returns `Status.FAILED`;
- otherwise calls `engine.userService.lastOnline(target)`, sends the returned text to the caller using the inbound whisper flag, and returns `Status.SUCCESSFUL`.

[TEST-BACKED] `src/test/java/org/saturn/app/command/impl/user/LastOnlineUserCommandImplTest.java` proves the two direct command boundaries:

- `executeWithArgumentsQueuesLastSeenMessage` injects a `UserService` returning `merc was online 1 minute ago` and expects addressed output `@testAuthor merc was online 1 minute ago` with successful status.
- `executeWithoutArgumentsReturnsFailure` expects `@testAuthor \\n Example: *lastseen merc` and failed status.

### Source persistence/format behavior

[OBSERVED] `src/main/java/org/saturn/app/service/UserService.java: UserService.lastOnline(String)` and `src/main/java/org/saturn/app/service/impl/UserServiceImpl.java: lastOnline` provide the actual lookup behavior:

1. query `SqlUtil.SELECT_LAST_SEEN` with the normalized target bound as both name and trip;
2. query the most recent row matching `(name = ? OR trip = ?)`, excluding literal `LEFT` and `JOINED` messages;
3. if present, format its millisecond `created_on` in UTC RFC-1123 style, JSON-escape the last message without quote delimiters, and compute `DateUtil.getDifference(nowUTC, lastSeenUTC)`;
4. query `SqlUtil.SELECT_SESSION_JOINED` with the same bindings for the latest literal `JOINED` row, then render its UTC RFC-1123 timestamp and duration to now;
5. render the fixed multiline payload. No lookup result leaves every derived field at the DTO default `" - "`.

[OBSERVED] exact SQL is in `src/main/java/org/saturn/app/util/SqlUtil.java`:

```sql
SELECT message,created_on FROM messages
WHERE (name = ? or trip = ?) and (message not in ('LEFT','JOINED'))
order by created_on desc limit 1;

SELECT created_on FROM messages
WHERE (name = ? or trip = ?) and message = 'JOINED'
order by created_on desc limit 1;
```

[OBSERVED] `src/main/java/org/saturn/app/model/dto/LastSeenDto.java` establishes the default rendered values (`" - "`) and `src/main/java/org/saturn/app/util/DateUtil.java: getDifference` establishes the exact `"<days> days, <hours> hours, <minutes> minutes, <seconds> seconds"` duration components. Zenbot already matches those helpers through `internal/util/date.go: Difference` and `FormatRFC1123`.

[LIMITATION] Saturn has no focused `UserServiceImpl.lastOnline` persistence/format test in the inspected source. Therefore the exact full historical payload is source-derived from the implementation, while the command success and missing-argument boundaries are test-backed.

## Zenbot gap and seams

[OBSERVED] the target has these compatible seams:

| Need | Existing target seam | Current gap |
|---|---|---|
| Catalog metadata | `internal/command/registry.go: RegisterAll` | all aliases and USER role already exist |
| Inbound authorization | `internal/listener/message/handlers.go: DispatchUserCommand` | standard `IsUserAuthorized(author, cmd.GetRole())` will enforce the catalog USER role; no command-local privilege branch is needed |
| Concrete command construction | `internal/command/handlers.go: newCommand` | no `lastonlineCommand` case; falls through to `saturnCommand` placeholder |
| Live registration | `internal/command/dispatch_adapter.go: RegisterUserUtilitiesWithDirectAgent` | `lastonline` absent from the concrete canonical list, so no alias dispatch exists |
| Service composition | `internal/factory/engine_factory.go: NewEngineWithOptions` | always composes `service.UserService{Queries: q}` when H2 is available |
| User-query contract | `internal/repository/user_queries.go: UserQueryRepository` | currently only registered users, nicks, and basic user data; needs a typed last-seen read |
| H2 implementation | `internal/repository/h2/user_queries.go: Database` | existing read-only `QueryContext` conventions and `messages` access; needs two typed query methods or one aggregate query method |
| Schema and indexes | `internal/repository/h2/schema-h2.sql: messages` | existing `trip/name/created_on/message` columns and `idx_messages_trip_created_on`, `idx_messages_name_created_on`; no migration required |
| Formatting | `internal/util/date.go: Difference`, `FormatRFC1123` | no `LastOnline` renderer/clock injection |
| Output | `internal/command/handlers.go: reply` | already addresses normal replies and preserves whisper (`IsWhisper || Whisper || Type == "whisper"`) |

[OBSERVED] `internal/repository/h2/user_queries.go: BasicUserData` demonstrates the appropriate `QueryContext`, scan, and error-propagation style. `internal/command/users_nicks.go` and `internal/command/users_nicks_integration_test.go` demonstrate the `bundle(engine) -> Users -> repository` command path and a real-H2 listener-dispatch test fixture.

## Proposed file map

Only these implementation/test files should be changed or created by the slice:

1. **Modify** `internal/repository/user_queries.go`
   - add a typed `LastOnline(ctx context.Context, target string) (repository.LastOnlineRecord, error)` (or equivalent record name) to `UserQueryRepository`;
   - record carries nullable/optional latest non-presence message timestamp/content and latest joined timestamp. Keep presentation fields out of the repository.
2. **Modify** `internal/repository/h2/user_queries.go`
   - implement the typed read with parameterized `QueryRowContext`/`QueryContext`, exact Saturn predicates, descending `created_on`, and limit one;
   - represent missing rows without an error (nil pointer fields or explicit validity booleans), and propagate database/scan/context errors.
3. **Create** `internal/repository/h2/last_online_test.go`
   - real H2 evidence for target matching by name and trip, latest-row selection, exclusion of `LEFT`/`JOINED` from last-message selection, latest `JOINED` selection, and absent data.
4. **Modify** `internal/service/services.go`
   - expose `UserService.LastOnline(ctx, target)`;
   - inject/use a clock at the service boundary (for example `Now func() time.Time` on `UserService`, defaulting to `time.Now().UTC()`), call the repository read, and render the source-shaped payload using existing `util.Difference`, `util.FormatRFC1123(..., util.UnitMilliseconds, "UTC")`, and JSON string escaping without surrounding quotes.
5. **Create** `internal/service/last_online_test.go`
   - fake the small `UserQueryRepository` contract and use a fixed UTC clock to lock rendering of populated and all-missing records, including escaped quote/backslash/newline content.
6. **Create** `internal/command/last_online.go`
   - implement `lastonlineCommand` with `commandBase`; validate only `args(c.message)[0]` after trim/one leading `@` removal; call `userService(c.engine).LastOnline`; reply to the requester through `reply`; return command and context errors as `FAILED`.
7. **Modify** `internal/command/handlers.go`
   - return `&lastonlineCommand{b}` for canonical `lastonline`.
8. **Modify** `internal/command/dispatch_adapter.go`
   - add canonical `lastonline` only when `bundle(e).Users` and `Users.Queries` are non-nil, following existing conditional registration; this prevents exposing a generic/no-op handler on engines without persistence.
9. **Create** `internal/command/last_online_test.go`
   - command-level exact usage/success/whisper/error tests plus alias registration/listener dispatch against real H2.

**Do not change:** `MIGRATION_PLAN.md`, frozen audit records, configuration, agent/live/moderation files, semantic-moderation staging/reversal artifacts, or Saturn. No schema change, H2 migration, new protocol command, or listener-order change belongs to this slice.

## Interface, data flow, authorization, and output

```text
chat/whisper "!lastseen @merc"
  -> message.ResolveUserMetadata
  -> message.DispatchUserCommand
       -> registry alias => canonical lastonline (USER)
       -> engine.IsUserAuthorized(author, USER)
       -> legacyAdapter.Execute(background context)
       -> lastonlineCommand.Execute
       -> UserService.LastOnline(ctx, "merc")
       -> UserQueryRepository.LastOnline(ctx, "merc")
       -> h2.Database reads messages twice
       -> UserService renders source-shaped text
       -> reply(commandBase, text)
       -> Engine.SendChatMessage(author, text, inbound-whisper)
```

### Authorization

- The source requirement is `REGULAR`; Zenbot's equivalent catalog role is `model.USER` in `internal/command/registry.go`.
- Leave authorization entirely at `DispatchUserCommand`. `lastonlineCommand` must not inspect trips, active users, hashes, moderator/admin status, or agent capability.
- This is a read-only historical lookup; it writes no repository records itself. Existing listener audit and command-audit behavior remain unchanged.

### Input and error semantics

- Use first token only, matching Saturn. Extra tokens are ignored.
- Trim whitespace and remove exactly one leading `@`; a blank/missing result replies exactly `"\\n Example: " + engine.GetPrefix() + "lastseen merc"` and returns `model.FAILED` with no database call.
- If the service/repository fails, return `model.FAILED` and the error; do not substitute `"user not found"`, a generic acknowledgement, or invented data.
- A successfully completed lookup with no matching messages is not an error: render Saturn's placeholder values in the normal fixed payload and return `model.SUCCESSFUL`.

### Rendering compatibility

The service owns text compatibility and must render the source field order and punctuation exactly:

```text
\n Nick|Trip: <requested target>
\n Joined: <joined RFC1123 or " - ">
\n Last seen: <last-seen RFC1123 or " - ">
\n Seen active: <duration or " - "> ago.
\n Session duration: <duration or " - "> \n Last message: <JSON-escaped message or " - ">
\n
```

Use literal `\\n` separators at the command/service boundary, as Zenbot's normal outgoing payload normalization already handles them. Preserve the source quirk that `Seen active:  -  ago.` is rendered when no last message exists. The requested target—not a resolved nick/trip—is echoed as `Nick|Trip`, matching Saturn.

## Compatibility decisions and exclusions

1. **Case behavior:** preserve Saturn's SQL equality semantics for this command. Do not reuse `NicksByTrip`'s `LOWER(trip)` behavior or introduce case-folding. Only trim and one-`@` normalization come from `IdentityUtil.normalizeNickTarget`.
2. **Message visibility/channel:** preserve source SQL: it does not filter by `visibility` or `channel`. Do not silently add room/visibility policy in this parity slice.
3. **Presence values:** only literal uppercase `LEFT` and `JOINED` are excluded from last message; only literal `JOINED` feeds session start. Do not broaden to case-insensitive or event-type semantics.
4. **Output:** add/use a dedicated source-compatible JSON-string escape helper for Apache Commons Text `StringEscapeUtils.escapeJson`; test quote, backslash, newline, control-character, and HTML-sensitive vectors before choosing a standard-library primitive. Do not quote-wrap, truncate, or add sanitization beyond the source behavior.
5. **No agent coupling:** do not expose `lastonline` as an agent tool or route it through `l`; that is a distinct live-agent delivery concern.
6. **No schema/config decision:** the target `messages` columns and indexes already satisfy the two reads. No configuration, migration, index audit, or external service is needed.

## Complexity and risk

**Complexity: small (one command vertical, read-only).** Expected change is 6–9 Go files plus focused tests, with no configuration/protocol/schema change.

| Risk | Mitigation |
|---|---|
| Full Saturn payload is more detailed than the source command test | service test fixes source-derived output byte-for-byte with a fixed clock; document the source-test limitation rather than inventing semantics |
| Clock-dependent duration makes tests flaky | inject a fixed UTC `Now` function in the service test; runtime default is current UTC time |
| SQL scan cannot distinguish missing row from nullable message | typed record uses explicit nullable/valid fields; test all-missing data |
| Accidental case normalization changes parity | repository test uses mixed-case target and proves no match where source equality would not match |
| Placeholder remains reachable | replace construction in `newCommand` and conditionally register only the concrete command; listener test verifies every alias has real output |
| Existing dirty semantic-moderation work is disturbed | file map excludes those files; inspect `git diff --check`/`git status --short` before and after; stage nothing |

## RED -> GREEN focused test plan

Perform each vertical test cycle before its associated production edit.

1. **RED: repository contract/query behavior**
   - Add `internal/repository/h2/last_online_test.go` with a real H2 fixture containing `JOINED`, `LEFT`, ordinary messages, newer/older rows, matching by name, matching by trip, and an unmatched target.
   - Assert selected message/timestamp and joined timestamp independently. Assert `LEFT`/`JOINED` cannot be selected as the last message, but the newest `JOINED` is selected for session start.
   - Run `go test ./internal/repository/h2 -run TestLastOnline -count=1`; it should fail because the repository contract/method does not exist.
   - GREEN: add the typed interface and H2 implementation, then rerun until green.

2. **RED: source-shaped rendering**
   - Add `internal/service/last_online_test.go` with a fake query repository and fixed `Now` (for example `1970-01-02T00:00:00Z`).
   - Assert complete output for populated fields, all default ` - ` fields when no rows exist, RFC-1123 UTC values, source duration text, and JSON escaping of a message containing quote, backslash, and newline.
   - Run `go test ./internal/service -run TestUserServiceLastOnline -count=1`; it should fail because the service API/rendering is absent.
   - GREEN: implement service rendering with existing date utilities; rerun.

3. **RED: command behavior and live dispatch**
   - Add `internal/command/last_online_test.go`.
   - Direct tests: all five aliases resolve to USER metadata; missing/`@`-blank input returns `FAILED`, emits exact prefix-sensitive usage, and does not call the query; `@merc ignored` queries `merc`, outputs the service text, preserves whisper; repository error returns `FAILED` with no fabricated reply.
   - Integration test: use `h2fixture.Open`, `RegisterUserUtilities`, and `listener.NewUserChatListener` to dispatch `!lastseen merc` and at least one alternate alias. Assert one addressed reply containing the populated source-shaped data. Also assert the command is not registered when `Users`/`Queries` are unavailable.
   - Run `go test ./internal/command -run 'Test(LastOnline|LastSeen)' -count=1`; it should fail because construction/registration is absent.
   - GREEN: add command, constructor switch, and conditional registration; rerun.

4. **Regression gates**
   - Run the relevant packages, then full `go test ./...`. Do not add broad test-only refactors or unrelated hardening.

## Rapid verification commands

Run from `/Users/ab/workspace/go-projects/zenbot` after implementation:

```bash
gofmt -w internal/repository/user_queries.go internal/repository/h2/user_queries.go internal/repository/h2/last_online_test.go internal/service/services.go internal/service/last_online_test.go internal/command/last_online.go internal/command/last_online_test.go internal/command/handlers.go internal/command/dispatch_adapter.go

go test ./internal/repository/h2 -run TestLastOnline -count=1
go test ./internal/service -run TestUserServiceLastOnline -count=1
go test ./internal/command -run 'Test(LastOnline|LastSeen)' -count=1
go test ./internal/command -run 'Test(RegisterUserUtilities|UsersAndNicks)' -count=1
go test ./internal/repository/h2 ./internal/service ./internal/command
go test ./...
git diff --check
git status --short
```

Expected acceptance: focused tests pass; package tests pass; full suite passes; `git diff --check` is clean; only this slice's intended files plus pre-existing dirty work appear. No commit or push is part of this handoff.

## Routing decision

**Assign to @developer.** The implementation is additive and follows established local seams: a typed H2 read, one service renderer, one concrete command, conditional registration, and focused real-H2/command tests. It does not need @senior-developer because it creates no cross-runtime protocol contract, schema migration, authorization redesign, config surface, or product-policy decision. Escalate only if a pre-existing H2 behavior prevents exact Saturn predicate/output compatibility or the full baseline exposes a regression outside this file map.

## Independence from blocked semantic MODERATION activation

This command does not read, write, invoke, register, or configure `internal/agent/**`, semantic moderation, moderation target selection, shadow-ban reversal, participation policy, or the disputed nick/hash source interpretation. Its target is explicit command text, and its sole data source is historical `messages` lookup by source-defined name-or-trip equality. The blocked MODERATION activation decision concerns a semantic agent action's identity disposition; `lastonline` is a regular-user, read-only command dispatched after ordinary listener authorization and has no dependency on that decision. Therefore it can proceed while all current semantic moderation staging/reversal artifacts remain untouched.
