# Text, identity, and weather audit design

## Scope and authority

Implement the full objective in the attached text-identity-weather request: trace and repair escaping/newlines, nickname targets, and `*weather chisinau`; preserve transport safety and established Zenbot correctness fixes. The user authorizes implementation without approval pauses. No deployment, live moderation, reference-repository edits, or unrelated game/automove changes.

## Text contract

Application strings are decoded Unicode text. Actual LF and literal backslash+n are distinct. Formatting producers insert actual LF. Chat senders add addressing only, then serialize once into JSON. Transport accepts already serialized frames and never unescapes them. Prefer the existing JSON payload boundary over a new text abstraction; keep raw protocol operations separate from chat text. Quotes, backslashes, all controls, and non-ASCII content must survive JSON decoding unchanged. CR/CRLF are preserved rather than guessed to be formatting escapes.

Migrate all affected producers and their consumer tests together. Remove pre-escaping in search, history, SQL display, support relay, notes, and new mail. Both prior Go and Saturn mail writers stored JSON string contents without enclosing quotes. Schema version 4 tags existing and legacy-writer rows as `JSON_STRING`, while new Go writes explicitly set `PLAIN`. Decode only the tagged legacy format, once on read; do not sniff backslashes or rewrite stored message bytes. Bootstrap upgrades are idempotent and tested on an isolated database. Test externally sourced multiline text through real engine serialization and a local WebSocket peer, not merely a fake sender.

## Identity contract

Use `util.NormalizeNickTarget` at semantic raw nickname ingress: trim, remove one ASCII mention marker, trim, reject blank. Downstream resolved source names are literal, not normalized again. Existing `common.NickTarget` represents a resolved source name. Do not normalize generic command arguments globally. Inventory aliases and alternate direct/service/agent entry points. Mixed nickname/trip/hash selectors require explicit precedence that preserves opaque identities; investigate before editing. Tests must distinguish raw `@merc`/`merc` equivalence from canonical source `@merc` identity.

## Weather contract

Use Saturn command/service/DTO/WMO/alignment code as the reference, documenting dirty-reference provenance. Preserve labels/order/thin-space alignment, local date/time, units, and weather icons where applicable. Keep Zenbot's bounded JSON, context propagation, provider-axis matching, missing-value handling, validated coordinates/URLs, and request encoding. Do not replace providers speculatively: both current default endpoints returned valid Chisinau data during this audit. Test the registered command, including aliases, real service formatting and final decoded frame. Diagnose deployed/runtime evidence separately from source behavior; do not claim historical root cause from a passing provider probe.

## Reproduced findings

- `SeenRecently` produces literal `\\n`; join listener sends with an empty author, skipping the sender's conditional unescape. This explains the reported public join message.
- All addressed senders interpret literal `\\n` as LF, corrupting user/external text. Core's JSON encoding itself already safely escapes LF; its whitespace replacements are no-ops.
- Search, stored mail, listed notes, SQL/history and support relay pre-escape strings that are subsequently serialized again.
- Current YouTube implementation uses oEmbed title and thumbnail; it does not consume a description. Test arbitrary multiline metadata and shared sender safety without inventing a new description feature.
- Saturn's weather service and tests have uncommitted changes. The reference alignment drops separator-free spacer rows; Zenbot already mirrors the resulting 17 displayed fields.
- Registration passed raw names to persistence; it now uses the shared nickname ingress helper. Mixed nickname/trip/hash APIs preserve separate raw opaque and normalized nickname predicates. Source identities remain literal downstream.
- Search query construction previously escaped only spaces, allowing reserved characters to change provider parameters. URL query encoding now preserves the complete query.
- The repetition monitor previously conflated a literal backslash+n with real LF. It now folds actual whitespace only; the false-mute regression passes.
- Weather's incomplete WMO map omitted seven valid Saturn codes and changed several other icons. All 28 Saturn mappings are restored without weakening the existing provider validation.

## Completion evidence

Require failing reproductions followed by passing focused tests; decoded-frame and local-WebSocket safety tests; command-surface nickname/alias coverage; weather command fixtures and read-only live-provider execution; final complete production escaping/normalization search classification; full Go suite, relevant race suite, vet, and diff review. A provider HTTP 200 alone and previous audit green tests do not prove completion.
