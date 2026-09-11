# Text, nickname, and weather audit — teardown and rebuild

Date: 2026-09-11. Baseline: `1bdf508e2a7f5a663654ca14402246238d01842a` on `master`. Changes are in the worktree, not deployed. Scope is the supplied end-to-end text/identity/weather request; DBZ and automove behavior were not changed.

## Findings and implemented repairs

### 1. Split text ownership and literal escape leakage — High

Execution trace:

1. `UserService.SeenRecently` built its surrounding line breaks as literal backslash+n.
2. The join listener sent that string with an empty author.
3. `SendChatMessage` only ran its unescaping helper when the author was nonempty. Therefore the unaddressed join message retained literal escapes.
4. JSON serialization correctly escaped the backslashes, and a receiving JSON parser correctly reconstructed the wrong application text. This produced the reported `\n @merc, has been seen as...` display.
5. Addressed senders hid this producer defect by converting every literal backslash+n into LF. User text such as a Windows path or a discussion of escapes was consequently altered. Some senders also rewrote CR and CRLF.

Repair: application text and receipts are plain Unicode text. Producers insert actual LF for intended formatting. All chat senders add addressing, then call the existing `util.CommandWithValue` JSON boundary. Removed sender unescaping and redundant core escaping code. No decoding guesses remain in chat senders.

Migrated formatting sites include help, status, memory, activity, identity/history, last-seen, mail/note acknowledgments, SQL, user/nick lists, join/subscription hooks, remote snapshot lists, YouTube previews, weather, and time. Removed pre-escaping from SQL cells, history text, support relay, search responses, and note listing. Existing SQL layout, UTF-8 truncation, addressing and support-relay authorization filtering remain.

Evidence: `chat_text_contract_test.go` checks exact decoded frames and receipts for all six sender variants with actual LF, literal backslash+n, CR/CRLF, quotes, Windows paths, tabs, NUL, and Unicode. Producer regressions cover SeenRecently, history, SQL, search, and YouTube. Existing encoded-intermediate goldens were updated to the new plain-text contract; wire-format expectations remain encoded where appropriate.

### 2. Persisted mail conflated storage and transport encoding — High

Execution trace: the old Go mail writer JSON-escaped the body before storing it; delivery then passed the stored string through sender transformations and another JSON encoding. Notes were stored plain but escaped during listing. Literal escape sequences, newlines and quoted text could therefore be displayed incorrectly or mutated.

Repair: new mail and all note output are plain text. Both previous Go and Saturn `MailServiceImpl` writers demonstrably stored JSON string contents without enclosing quotes. H2 schema version 4 adds `mail.text_encoding`, defaulting to `JSON_STRING` for existing rows and writers that omit the column. New Go writes explicitly select `PLAIN`. Reads decode only tagged legacy rows, exactly once. Unknown or malformed encodings return an explicit error with the mail ID, not guessed text. There is no live data rewrite or content-based encoding detection. Existing mail delivery/receipt transitions and Saturn's trailing body-space behavior are preserved.

Evidence: real-H2 plain mail/note round trips, legacy literal-vs-LF decoding, invalid-tag rejection, and an idempotent old-schema bootstrap test that verifies message bytes remain unchanged. This is a forward upgrade, not a promise that an old binary can correctly read new plain rows after a rollback.

### 3. Inconsistent nickname ingress and opaque selector corruption — High

Execution trace: registration persisted `@merc` literally while active-user commands commonly looked up `merc`. Other paths independently stripped a marker, sometimes before a mixed nickname/trip query. Consequently aliases and direct service calls could disagree, or an opaque trip beginning with `@` could become a different credential. Normalizing already-resolved source names again would also turn a literal source nickname `@merc` into `merc`.

Repair: use the existing `util.NormalizeNickTarget` contract for raw semantic nickname operands: trim, remove exactly one ASCII `@`, trim, reject blank. Resolved `common.NickTarget` values remain literal. Registration, info, history-tool input, and the command gateway use the same helper; mute/appearance no longer normalize twice. For mail, last-online, identity deletion and shadow-ban removal, retain the raw opaque selector separately from the normalized nickname predicate. Mail retains exact-trip precedence; last-online and deletion retain their ambiguity rejection.

| Surface | Verified boundary and coverage |
|---|---|
| `reg`, `register` | Raw name normalized before lookup/write; trip unchanged; `@@merc` resolves to literal `@merc` |
| `ban`; `kick`, `k`, `out`; `mute`, `dumb`; `color`; `flair`; `overflow`, `shoot`, `love`, `hug`, `kiss` | Both nickname forms resolve through active-user lookup to the actual canonical source nickname in serialized outbound frames |
| `info`, `i`, `whois`, `who`; `shadowban`, `sban` | Both forms reach the same source record and persistence identity |
| `move`, `recover`, `heal`, `resurrect` | Both forms propagate through the mover/snapshot helper; source and destination rooms retain `@` |
| `del`, `delete`, `remove`; `unshadowban`, `shadowmercy`, `unblock` | Both forms remove the intended isolated H2 record; mixed-selector tests preserve opaque credentials |
| `mail`, `msg`, `send` | Both forms resolve to the same recipient trip; exact-trip precedence retained; arbitrary body text unchanged |
| `lastonline`, `seen`, `last`, `online`, `lastseen`; `user_message_history` | Shared command or tool ingress; mixed raw selector is resolved at its query boundary, not stripped twice |
| Hash/trip-only inputs, contains patterns, caller/source identities | Not nickname-normalized: includes unban/unmute hashes, grant/activity/nicks credentials, history hashes, incoming snapshot users and command bodies |

Evidence: new nickname ingress, persistence-alias and repository selector tests, plus existing target-boundary, alias, history-tool and moderation tests. Command catalog and raw target consumers were cross-checked, rather than treating every parameter named `target` as a nickname.

### 4. Room identifiers were treated as nicknames — High

Execution trace: `nuke room@host` used `strings.ReplaceAll(..., "@", "")`, dispatching a room-wide operation to `roomhost` instead of the requested room.

Repair: use the existing room-selector parser, which trims an optional room `?` marker and preserves all `@` characters. Regression covers `room@host`, `@room`, and `?room@host`; no live moderation was executed.

### 5. Search query escaping permitted parameter injection — Medium

Execution trace: search replaced spaces with `%20` but concatenated the remainder into a URL. A query containing `&other=value`, `+`, or `#fragment` changed the provider's parameters or lost query text.

Repair: validated provider URL plus `url.Values.Set/Encode`; preserve existing endpoint parameters while setting the intended query fields. Use a local default HTTP client without modifying shared service configuration. Regression verifies Unicode, ampersand, plus, slash and fragment characters arrive as one exact query value.

### 6. Escape conflation triggered false repetition moderation — High

Execution trace: a user sent `A` + LF + `B` and then literal `A\nB`. The repetition fingerprint converted both spellings to the same text, potentially reaching the mute threshold for distinct messages.

Repair: retain existing case/actual-whitespace folding, but never interpret literal backslash+n as whitespace. Independent review identified this remaining occurrence; a failing live-monitor regression reproduced the false mute before the fix. The complete moderation package then passed.

## Weather: conceptual comparison with Saturn

Reference: `/Users/ab/workspace/projects/saturn`, including the weather command, service, DTO, WMO enum, output service and alignment helper. Its weather service/tests have uncommitted clock and solar-time fixes; committed reference behavior was also checked. The reference worktree was not changed.

| Stage | Reference behavior and retained/repaired Go behavior |
|---|---|
| Parsing | `weather`, `w`, `today` join location arguments. Go retains the existing registered command path and explicit service/delivery errors. |
| Geocoding | Saturn uses XML GeoNames and replaces spaces. Go retains secure JSON GeoNames, full URL encoding, provider business-error detection, empty-result errors, and validated finite/ranged coordinates. Canonical geocoder city name is retained. |
| Forecast request | Same provider and displayed metric set. Go retains `timezone=auto`, `forecast_days=1`, and provider time-axis selection instead of process-date and hour-array assumptions. Unused Saturn forecast fields are not required for output. |
| Parsing/nulls | Go retains bounded strict JSON reads, explicit provider rejection, nullable metrics and `unavailable` values instead of null dereferences or invented zeroes. Malformed timestamps/timezones fail explicitly. |
| Units/conversions | Provider units and numeric spelling remain; no speculative conversions. Actual provider-local time, sunrise and sunset retain the formatted offset. |
| Icons | Restored all 28 Saturn WMO mappings, including seven omitted codes and changed icons. Unknown codes remain `unavailable`, not an exception. |
| Formatting | Preserve labels/order, thin-space alignment and 17 visible rows. Saturn's separator-free spacer rows are dropped by its own alignment helper. Preserve Go's first-colon split so colons inside values are not lost. |
| Newlines/Unicode | Actual LF now leaves the formatter; one JSON encoding at delivery. Unicode city names, emoji and units survive decoding unchanged. |
| Async/errors | Keep synchronous context-aware Go calls and existing cancellation, receipt and send-error propagation. No detached weather action or speculative retry path was added. |

`*weather chisinau` succeeded through the actual registered command, real default providers, real service formatter and core JSON sender on 2026-09-11 at approximately 18:05 Europe/Chisinau. The locally captured output contained Chișinău/Moldova, 28.6 °C, the complete 17-row forecast, provider time 18:00 +0300, sunrise 06:36 and sunset 19:24. The opt-in test's missing audit-repository warning is expected: it deliberately has no production DB audit sink. It did not send a room message.

The earlier deployed failure cannot be assigned a precise historical cause from current evidence: no matching running bot or failure log was available. Current providers and the current registered command work. Confirmed port deviations are formatting escape ownership and WMO mapping; they are fixed without inventing an API outage or reverting Zenbot's existing improvements.

## Transport and external-content safety

The lifecycle is decoded incoming/provider text → command or hook → plain formatting → one JSON string encoding → WebSocket text frame → peer JSON decoding → original application text. JSON backslash escapes necessarily exist on the wire; they are not application-visible formatting.

`TestMultilineExternalTextSurvivesWebSocketAndNextMessage` exercises the real YouTube service, core sender, Gorilla transport and a local JSON-decoding WebSocket peer. Multiline text, literal escapes, quotes, controls and Unicode survive exactly; a second message is accepted on the same connection. This demonstrates safe framing rather than removing protection blindly. It does not promise immunity to server size/rate limits or unrelated network closures.

The current YouTube implementation in the inspected source returns title and thumbnail, not a description field. The test injects arbitrary multiline metadata through that actual path and exercises the common boundary used by any description text; no unrelated description-fetching feature was invented.

## Final residual-search classification

Codebase-wide searches covered literal/actual newline handling, replacements, quoting/unquoting, JSON marshaling, source/target lookup and nickname stripping. Production has no remaining double-backslash-newline formatting strings, legacy `escapeJava`/`escapeSaturnJava`, `normalizeChatText`, or `alignLiteralLines`. The only production ASCII `@` prefix stripping implementation is in `util/identity.go`.

Intentional residual categories:

- `util/json_payloads.go`: required JSON protocol escaping. Typed `json.Marshal` calls serialize protocol envelopes, provider/agent data, observations and schemas; their nested JSON layers are distinct contracts, not pre-escaped chat strings.
- Incoming listener/model/provider JSON decoders: required transport/data parsing. Tagged legacy mail decoding: required storage compatibility only.
- URL encoding in HTTP clients: required URI component encoding, not chat escaping.
- SQL identifier quoting and SQLite-to-H2 DDL regex replacements: database grammar, not message text.
- Actual LF formatting and line splitting in help, tables, mail, history and hooks: intentional text structure. Parser tokenization and whitespace validation operate on command syntax, not global body rewriting.
- Agent response-prefix cleanup, no-reply markers, safe diagnostic quoting, and participation punctuation folding: explicit presentation/control policies, not a second transport codec. Repetition matching preserves literal escapes after the fix.
- Command/room prefix removal and URL `www.` normalization: scoped syntax. Slice `[1:]` matches in join/message pruning remove expired records, not nickname characters.
- Tests intentionally retain encoded wire goldens and adversarial literal escapes. Historical documentation is not a runtime text producer.

## Verification and requirement coverage

- `go test ./... -count=1`: passed all packages, including real-H2 migration, command, service and listener tests.
- `go test ./internal/service -run '^TestWeatherPreservesUnicodeAndReservedLocationQuery$' -count=1`: passed after the final test-only addition.
- `go vet ./...`: passed.
- `git diff --check`: passed.
- `ZENBOT_TEST_LIVE_WEATHER=1 go test ./internal/command -run '^TestWeatherChisinauLiveProviders$' -count=1 -v`: passed with live providers and locally captured delivery.
- `go test -race ./... -count=1`: passed all packages, with no race reports. The macOS linker emitted `LC_DYSYMTAB` warnings for several test binaries; linking and tests completed successfully.

Regression coverage maps the request's invariants to decoded sender frames, producer fixtures, real WebSocket framing, tagged persistence round trips/upgrades, both nickname forms across command families and aliases, nested mover propagation, opaque-selector tests, weather malformed/null/cancellation fixtures, all WMO icons and actual registered-command execution. No LLM planner, deterministic intent heuristics, production restart or live database mutation was introduced.

Final evidence logs for this local run: `/tmp/zenbot-text-identity-final-tests.log`, `/tmp/zenbot-text-identity-race-tests.log`, and `/tmp/zenbot-final-live-weather.log`. These are local diagnostic artifacts; repeatable commands and checked-in regression sources above are the durable verification contract.
