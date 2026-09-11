# Utility command audit — 2026-09-11

Historical baseline report: references to open repairs or missing reproductions
describe that audit phase. Current scope, fixes and verification are recorded in
the [consolidated Teardown & Rebuild report](2026-09-11-command-audit-coverage.md).

Scope: `afk`, `ape`, `coin`, `help`, `crashcourse`, `info`, `lastonline`, `nicks`, `list`, `mail`, `msgchannel`, `note`, `notes`, `ping`, `users`, `say`, `sub`, `time`, `unsub`, `version`, `weather`, `ws`, `wsa` on the current master checkout. This report records the pre-fix state. No production code changed during this audit. Existing unrelated documentation edits were preserved. Reproduction files contain deliberately failing regression assertions, not production fixes.

## Coverage and method

Read the catalog, typed argument contracts, concrete construction/registration, retained legacy wrappers, command handlers, service bundle composition, H2 queries, chat/whisper listeners, AFK/subscription storage, support relay, snapshot parsing/operations/coordinator, and agent adapter contracts. Generic dispatcher, output-capture receipts, shared `reply`, raw argument preservation, and common engine fixes are assigned to the root audit. Every assigned command has a concrete production handler; retained `Afk`, `Say`, `List` wrappers are not the production typed handlers. The fallback `saturnCommand` utility implementations in `registry.go` are unreachable through reviewed canonical construction and must not be mistaken for production behavior.

All provider responses used for new probes came from an in-process fake HTTP transport; no external provider calls or chat sends occurred. Mail and history probes used isolated temporary real H2 fixtures. Existing tests are evidence of expected behavior, not proof of correctness: several explicitly preserve the very legacy behavior that conflicts with current agent contracts.

## Confirmed defects

### U1 — Private mail can be claimed with an unrelated nickname or case-variant trip (P1)

`internal/service/services.go:312` checks both the supplied nickname and trip, lowercased, against a receiver field populated with resolved **trips** by `QueueResolved` (`services.go:251`, `272`, `278`). `DeliverPendingMail` passes the untrusted chat nickname and actual trip into this query (`internal/listener/message/handlers.go:77`) and whispers matching content to that nickname before marking it delivered.

Offline trace: a pending private row for trip `AbC123` containing `secret` was returned to `(nickname=AbC123, trip=attacker)` and to `(nickname=someone, trip=abc123)`. Neither is the intended exact trip. This both discloses mail and permits consuming the intended user's delivery. Fix the resolved-trip ownership boundary: require a nonblank exact trip match, never let an arbitrary nickname claim a resolved-trip row. Preserve exact trip case in queue resolution too. Regression: `TestUtilityAuditPrivateMailCannotBeClaimedByNickname` fails for both attackers; add positive exact-owner delivery and blank-trip cases.

### U2 — Public lastonline exposes privately whispered messages (P1)

`internal/repository/h2/user_queries.go:21` (`selectLastOnline`) selects the most recent message without a visibility predicate. Whisper commands are explicitly persisted with visibility `WHISPER` by `internal/listener/info/handlers.go:83`. `UserService.LastOnline` embeds that message (`internal/service/services.go:78`, `92`), and the REGULAR `lastonline` command sends it to the request's output channel.

Offline trace: public `public hello` at timestamp 1 followed by private `!note private secret` at timestamp 2 produced `Last message: !note private secret`. Fix the public query to select only public messages, including the legacy LastSeen fallback query if applicable; audit public history siblings for the same omission. Regression `TestUtilityAuditLastOnlineDoesNotRevealWhispers` fails against real H2.

### U3 — Weather and time do not request JSON from their default geocoder (P1)

Both default to `http://api.geonames.org/search`, add `q`, `maxRows`, and `username`, then invoke a JSON decoder (`internal/service/services.go:392`, `523`). No `type=json` parameter or `/searchJSON` endpoint is used. The captured default URL was `http://api.geonames.org/search?maxRows=1&q=Paris&username=dev1`. Existing tests inject a server that returns JSON regardless of request, masking the format mismatch. Use an explicitly JSON endpoint/request; propagate service error payloads without manufacturing success. Regression `TestUtilityAuditGeocoderRequestsJSON` fails for both services. The audit did not externally verify provider account availability or live endpoint behavior.

### U4 — Weather rejects ordinary numeric forecast arrays (P1)

`weatherPayload.DailyRaw.UV/Radiation` and all numeric `HourlyRaw` arrays are `[]string` (`internal/service/services.go:464`, `472`). The current scalar temperatures already use `json.Number`, but numeric arrays do not. Offline forecast with `uv_index_max:[4.2]` and `apparent_temperature:[20.1]` fails with `json: cannot unmarshal number into Go struct field .daily.uv_index_max of type string`. Existing fixtures quote every numeric array value and miss the failure. Decode numeric and null values correctly and retain their textual presentation. Regression `TestUtilityAuditWeatherNumericPayload` fails.

### U5 — Invalid weather timezone panics; malformed dates silently become year 1 (P1/P2)

`internal/service/services.go:495` discards `time.LoadLocation` errors, passes a nil location to `ParseInLocation`, and ignores parse errors. The offline payload timezone `invalid/zone` produced `panic: time: missing Location in call to Date`. Empty/malformed current, sunrise, and sunset values can format zero dates. Validate provider timezone/date values and return a useful error or explicit unavailable field. Regression `TestUtilityAuditWeatherInvalidTimezoneDoesNotPanic` catches the panic and fails.

### U6 — Ping failures become a successful zero-millisecond measurement (P2)

`internal/command/user_utilities.go:101` discards every TCP dial error, including cancellation while dialing. Offline dial to `127.0.0.1:0` returned command `SUCCESSFUL`, `err=<nil>`, chat `response time: 0 milliseconds`. An absent ping service has the same false-success result. Use the concrete service result; propagate failed/canceled measurement and avoid reporting an invented duration. Regression `TestUtilityAuditPingFailureIsNotZeroLatencySuccess` fails. The unused `pingCommand` in `services.go` already handles actual service errors more honestly, but is not what the registry selects.

### U7 — Private note/mail tool promises disagree with public delivery (P1/P2)

Catalog entries say mail queues a private message and notes are private. `mailCommand` instead stores `c.message.IsWhisper` (`internal/command/mail_notes.go:26`); a public agent request can queue public delivery. `notesCommand` lists stored notes through the ordinary visibility-following reply helper (`mail_notes.go:108`). Offline `!notes` with an existing private note produced `alice|'s notes: ...[private secret]...|false`. Fix private note listing and private mail behavior at the effect boundary, with clear direct-command semantics and retained deliberate visibility where appropriate. The private-note regression `TestUtilityAuditNotesArePrivate` fails; existing `TestNotesParityListPurgeClearAndInvalidArguments` explicitly expects the insecure public behavior and needs intentional revision. Acknowledgments need not reveal note contents.

### U8 — Time drops fractional UTC offsets (P2)

`internal/service/services.go:595` divides offset minutes by 60 using integer arithmetic. An injected India offset of 330 minutes produces `UTC offset: +5`, misreporting the timezone by 30 minutes; negative and quarter-hour offsets also lose precision. Preserve signed hours and minutes. Regression `TestUtilityAuditTimePreservesFractionalUTCOffset` fails with the captured full response showing `+5`.

## Additional traced findings and coverage limitations

- **History contract mismatch:** `nicks` promises registered names, but `selectNicksByTrip` reads distinct names from `messages`, case-folding trip IDs (`internal/repository/h2/user_queries.go:18`). A newly registered name with no message is absent; an unregistered historical name appears. Prefer an honest historical-name contract if this is the intended feature, and preserve trip identity case. `lastonline` promises last observation but only finds non-presence messages; a join/leave-only user is absent. Its session duration is `now - latest join`, even for departed users, and latest join can be later than the selected last message. These are traced contract/semantics defects, not newly executed regressions in this report.
- **Mail/notes cancellation:** their methods use non-context `DB.Query/Exec`; cancellation is checked only at command entry. A database wait can outlive the tool deadline or commit after cancellation. Introduce context-taking service methods used by commands and listeners while preserving wrappers if compatibility needs them. No blocked-database timing probe was run.
- **Mail validation:** recipient-only `!mail user` queues an empty message, although the agent schema requires a body. The no-argument help uses hardcoded `-mail`, not the live prefix. Recipient query can duplicate trip IDs for multiple registered names and blends name matching with trip matching. Unknown-recipient reporting depends on `GroupB` even when a functional mail database exists.
- **Notes validation:** `notes purge additional words` deletes all notes because only the first token is inspected; unsupported arguments fail silently. `notes` blank-trip handling is less strict than `note`. Note-save and mail-body whitespace flattening is shared parser work assigned to root.
- **Absent services:** `weather`/`time` echo the requested location as success when no service is configured; `users`/`nicks` return silent success with no query capability. These commands are registered unconditionally. Return unavailable or avoid registering unavailable capabilities. Factory-built ordinary master engines install services, so this primarily affects optional/partial composition and tool truthfulness.
- **AFK/shared engine:** command scans all active entries matching caller trip, then sends success even if no entry matched. AFK storage is an unsynchronized pointer-keyed map; live active-map reads bypass `usersMu`, and same-trip multiple sessions can leave duplicate AFK records. Root owns common engine fixes. The legacy AFK wrapper shares these semantics.
- **Subscriptions/shared engine:** `SubscribeTrip` uses exact map insertion while `IsSubscribedTrip`/`UnsubscribeTrip` use case-insensitive comparison (`internal/core/engine_impl.go:537`). This merges distinct trip identities for join-notification delivery and allows a case-variant trip to remove another subscription. Root owns common engine fixes. Joining user history is looked up before the current join is persisted, so first-time users may yield empty history; this is a traced behavior requiring desired-output judgment.
- **List/snapshot:** list includes every parsed roster member, including the temporary bot if the server's onlineSet includes it. Listing deduplicates via `IdentityKey`, which may collapse different active nicknames sharing a trip. Parent should decide whether the presence contract requires one entry per nickname. `list ?room` does not normalize the leading `?` as `msgchannel` does; the latter removes every question mark, not only a room prefix. Local list with no arguments displays a roster plus usage but returns FAILED. Snapshot coordinator declares completed before Apply, publishes success before Flush, ignores Flush/Close errors, and cannot cancel the already-completed operation. These shared findings were passed to root; no extra snapshot failure probe was added here.
- **Say contract:** output is public regardless of request visibility, strips all punctuation/non-ASCII for non-admins, flattens whitespace, and adds a trailing space. The agent description promises exact supplied content and does not disclose the restriction. Concrete/legacy handlers also ignore send errors; root owns shared receipts and error handling.
- **Help:** always whispers and propagates send errors, but includes unsupported mining/whiskey examples, describes Go memory as JVM memory, and categorizes msgchannel as admin despite public access. `alignHelp` splits literal `\\n` while help constants contain actual newline characters, preventing intended row alignment. The catalog is the correct source for actual availability/roles.
- **Support relay:** both concrete `ws`/`wsa` propagate unavailable support replica and send errors, precheck cancellation, sanitize non-admin body to ASCII alphanumerics/spaces, and use managed inventory only. Anonymous prefix is explicit; both are intentionally hidden from agent tools. Empty arguments still send a label plus blank body; no new severe independent defect found.

## Per-command completion matrix

All aliases and roles below were checked against `catalog/catalog.go`, construction in `handlers.go`/`registry.go`, and production registration in `dispatch_adapter.go`. “Entry cancel” means context is checked before work; it does not claim cancellation interrupts every operation.

| Command / aliases | Handler and execution path inspected | Validation, output, errors, contract disposition |
|---|---|---|
| afk / a | `afkCommand`, legacy `Afk`, engine AFK state, message/update/rename listeners | Trip required, optional reason, entry cancel; shared-map and absent-caller issues above; ordinary output acknowledgment. |
| ape / harambe | `apeCommand` | No arguments, static ape art, request visibility, send errors and entry cancel propagated; no independent defect. |
| coin / toss / ct | `coinCommand` | Crypto-random binary result, always public, entropy/send errors and entry cancel propagated; no independent defect beyond documenting public visibility. |
| help / h | `helpCommand`, static help sections | Always whisper; send failure/cancel propagated; stale help and newline alignment issues above. |
| crashcourse / howto / moderationcrashcourse / hcguide | Dedicated `howToCommand` definition | Static guide/link addressed to caller; visibility preserved, send errors/cancel propagated; no independent defect. External video not inspected. |
| info / i / whois / who | `infoUserCommand`, live roster | Requires target, strips one @, case-insensitive nickname match, missing target fails; shared send-error/live-map concerns only. Current identity contract correct. |
| lastonline / seen / last / online / lastseen | `lastonlineCommand`, `UserService.LastOnline`, H2 LastOnline and fallback | Validates one target; errors/missing row propagate; U2 disclosure plus last-observation/session semantics noted above. |
| nicks / t2n | `nicksCommand`, user service, H2 NicksByTrip | Requires trip; history-vs-registration, case-folding, and missing-service silent-success mismatches above. |
| list | `listCommand`, legacy `List`, credentialed snapshot, `ListRoomOperation` | Local roster or remote workflow; remote typed data; transport/timeout/cancel traces delegated to root; room/self/identity concerns above. |
| mail / msg / send | `mailCommand`, QueueResolved/Pending/MarkDelivered, chat listener | U1 ownership and U7 privacy; parameter/context issues above; DB errors propagated but acknowledgment receipt shared concern. |
| msgchannel / msgroom | `msgChannelCommand`, credentialed snapshot, `RemoteMessageOperation` | Requires room/body; blank room rejected; local send errors propagated, remote failures via workflow; anonymous source-room body/image special case; shared snapshot concerns above. |
| note / save | `noteCommand`, NoteService.Save | Requires trip/text, persists by exact trip, DB errors propagated; entry-only cancellation/raw body issues; stored-private contract requires private retrieval. |
| notes | `notesCommand`, NoteService.List/Clear | Own exact-trip list/purge; U7 public retrieval; extra purge args accepted and unknown operation silent failure. |
| ping / p | `pingUtilityCommand`, PingService.DialContext | Fixed hack.chat TCP target in production; U6 false measurement; unused alternate handler reviewed. |
| users / whitelist / blacklist / offenders / knownoffenders | `usersCommand`, GroupB/Queries registered directory | Public directory, aliases intentionally identical; DB errors propagated; missing-service silent success; no filtering promised by canonical tool. |
| say / echo | `sayCommand`, legacy `Say`, authorization check | Always public bot-authored text, non-admin sanitation, empty input accepted, ignored send failure, exact-content contract mismatch. |
| sub / subscribe | `subscribeCommand`, engine subscribers, join listener | Requires trip, subscribes caller, entry cancel, acknowledgment; shared trip-case mismatch; duplicate insert acknowledged as subscribed. |
| time / t | `timeCommand`, TimeService geocode/sun/time | Location required, request visibility, HTTP context/status propagated; U3/U8, optional-service false success, ignored timestamp validation. |
| unsub / unsubscribe | `unsubscribeCommand`, engine subscribers | Requires existing subscription; caller-scoped intent, entry cancel; engine trip-case mismatch above. |
| version / v | `versionCommand`, root release.Version | No arguments, running version, visibility/send-error/cancel preserved; no independent defect. |
| weather / w / today | `weatherCommand`, WeatherService | Required location, request visibility; U3/U4/U5, service fallback and provider date/hour alignment concerns. |
| ws / wsay | `supportRelayCommand`, SupportReplicaRelay | USER role, hidden tool, named relay, managed support required, context/send errors propagated; no independent severe defect. |
| wsa / wsayanon / anonsay | Same relay with anonymous flag | USER role, hidden tool, anonymous prefix, same validation/error findings as ws. |

## Executed evidence and remaining verification

`go test ./internal/service ./internal/command -run TestUtilityAudit -count=1 -v` failed as expected for geocoder mode (both services), numeric weather, invalid weather timezone, fractional timezone, mail ownership, ping failure, and public note delivery. The separately added `go test ./internal/service -run TestUtilityAuditLastOnline -count=1 -v` failed as expected for whisper disclosure. Reproduction files: `internal/service/utility_audit_regression_test.go` and `internal/command/utility_audit_regression_test.go`.

This audit does not claim fixes or a green suite. It does not establish current third-party availability, undocumented provider response variations, server-side relay acceptance, or production H2 contents. New tests are intentionally local and deterministic. Shared dispatch/capture/engine/snapshot findings should be consolidated with the root report before implementation to avoid duplicate or conflicting fixes.

Existing targeted tests passed with `go test ./internal/command ./internal/service ./internal/repository/h2 ./internal/listener/snapshot ./internal/core -run 'Test(PingVersionApeAndCoinParity|UserUtilityCluster|Help|Crash|NoteAndSave|NotesParity|MailGroupC|LastOnline|Info|Users|Nicks|Subscribe|Unsubscribe|SupportRelay|RemoteMessage|ListRoom)' -count=1`. This confirms that the reproduced defects were not caught by that existing coverage.
