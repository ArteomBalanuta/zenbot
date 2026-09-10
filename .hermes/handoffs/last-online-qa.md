# `lastonline` / `lastseen` QA

## Result: PASS

Independent QA completed against Saturn `develop` at `10a1ea3` (read-only) and the Zenbot working tree. No commit or push was made. Pre-existing semantic moderation/reversal work and its handoffs were not changed.

## Source-to-target behavior evidence

- **Aliases and authorization:** Saturn `LastOnlineUserCommandImpl` declares exactly `lastonline`, `seen`, `last`, `online`, and `lastseen`, with `Role.REGULAR`. Zenbot `RegisterAll` declares the same aliases with `model.USER`; command tests resolve and listener-dispatch all five aliases.
- **Input/failed usage:** Saturn uses its first parsed argument, trims it, removes one leading `@`, and on missing/blank emits `\\n Example: <prefix>lastseen merc` with `FAILED`. Zenbot `lastonlineCommand` follows this; direct tests prove `@merc ignored` queries `merc`, while absent and `@` inputs reply exactly with usage, return `FAILED`, and make no query.
- **Delivery/error behavior:** the command routes through `reply`, preserving inbound whisper delivery. The direct test asserts `IsWhisper=true`; a service/repository error returns `FAILED` with no fabricated reply.
- **SQL/case/H2:** target parameterized H2 reads match Saturn predicates exactly: `(name = ? or trip = ?)`, latest non-`LEFT`/`JOINED` message, and latest literal `JOINED`; no visibility/channel condition and no case folding. Real-H2 tests cover name/trip matching, case-sensitive non-match, presence exclusion, newest-row selection, and an absent target.
- **Rendering:** source uses literal-backslash-n fields, requested target echo, UTC RFC-1123 dates, source duration format, DTO default ` - `, and JSON escaping without surrounding quotes or HTML escaping. Fixed-clock service tests cover populated/default payloads plus quote, backslash, newline, control, and HTML-sensitive text.
- **Source ordering quirk fixed:** Saturn only queries/renders joined-session fields after the last-seen lookup found a row (`UserServiceImpl.lastOnline`, lines 107–123). Zenbot previously rendered a joined-only record. Added a RED regression test and changed H2/service gating so join-only state remains all session defaults, matching Saturn.
- **Conditional registration:** `lastonline` is registered only when `Bundle.Users` and `Users.Queries` exist; test proves it is absent otherwise. Existing listener ordering was not changed.

## Commands run

Focused final gates:

```text
go test ./internal/repository/h2 -run TestLastOnline -count=1                         PASS
go test ./internal/service -run TestUserServiceLastOnline -count=1                    PASS
go test ./internal/command -run 'Test(LastOnline|LastSeen)' -count=1                  PASS
go test ./internal/listener -run TestUserJoinedListener -count=1                      PASS
```

Regression/build gates:

```text
go test ./internal/repository/h2 ./internal/service ./internal/command                PASS
go test ./...                                                                           PASS
go build ./...                                                                          PASS
git diff --check                                                                        PASS
```

The added join-only source-parity test was first run RED and failed because Zenbot emitted joined/session duration; it passed after the scoped repository/service fix.

## Coverage

- `internal/repository/h2/last_online_test.go`: latest selection, presence filtering, latest `JOINED`, exact name/trip equality, missing data.
- `internal/service/last_online_test.go`: populated/default exact payloads, UTC dates/durations, escaping, and join-only source gating.
- `internal/command/last_online_test.go`: aliases/USER role, first-argument normalization, exact usage/no query, whisper reply, error propagation, conditional registration, real-H2 inbound listener dispatch.
- `internal/listener/user_joined_listener_test.go`: its repository fake gained the required `LastOnline` method; its focused test passes.

## Files fixed during QA

- `internal/repository/h2/user_queries.go` — skip the joined query when no last-seen row exists, matching Saturn call order.
- `internal/service/services.go` — render joined/session fields only when a last-seen row is present.
- `internal/service/last_online_test.go` — regression for a join-only record remaining at source DTO defaults.
- `.hermes/handoffs/last-online-qa.md` — this QA record.

## Known source-evidence limitation

Saturn has focused command-boundary tests but no focused persistence/rendering test for `UserServiceImpl.lastOnline`. Exact historical payload behavior is therefore derived from Saturn source; Zenbot now locks that derived behavior with deterministic service tests.
