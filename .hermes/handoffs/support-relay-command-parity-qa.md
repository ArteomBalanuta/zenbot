# Support-relay command parity QA

## Outcome: ACCEPTED — no scoped defect found

Independent QA audited the bounded `ws`/`wsa` support-replica relay vertical in the dirty worktree. No production or test code was changed by QA.

## Source and contract audit

Read the authoritative Saturn sources:

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/WhiskeySayUserCommandImpl.java`
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/WhiskeyAnonUserCommandImpl.java`
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/UserCommandBaseImpl.java`

The target matches the bounded source contract:

- Exact USER aliases are retained: `ws`, `wsay`; `wsa`, `wsayanon`, `anonsay`.
- Normal dispatch performs the established USER authorization before executing the relay. The direct dispatch test proves allowed relay invocation and denied-user non-invocation with the existing denial only.
- `ws`/`wsa` register only when `common.SupportReplicaRelay` is present. They resolve to `supportRelayCommand`, not the generic `saturnCommand`; a plain engine remains unregistered.
- The relay snapshots only the managed engine key exactly `support`; a missing engine fails with no caller reply and no replica construction/mutation.
- It uses exact case-sensitive configured `AdminTrips` membership exclusively for the rendering branch. Source-shaped rendering preserves configured-admin punctuation, strips regular callers to ASCII letters/digits/spaces per argument, and preserves the trailing space.
- The direct source imports Apache Commons Text `StringEscapeUtils`. The Go helper was compared to its `escapeJava` composition: quote/backslash/control escapes, printable range through `U+007F`, uppercase `\\uXXXX` escapes outside that range, and UTF-16 surrogate-pair output. Existing tests cover quote, backslash, newline, tab, `U+0001`, and regular-user removal of markup/emoji; implementation inspection covers the remaining source-shaped branches.
- Route payloads are exactly `author + ": " + escaped` and `anon_from_hc: " + escaped`; sends are public and unaddressed once, and success creates no caller-room reply (including a whisper input).
- Pre-cancelled context, absent capability, missing support, and support-send failure return failure/error without a fabricated reply. The relay does not mutate the replica manager.
- Main composes `core.NewSupportReplicaRelay(manager, c.AdminTrips)` after the existing manager/controller and before utility registration. It does not introduce a replica factory, proxy, credentials, temporary session, snapshot, agent gateway/tool, H2/repository, or transport dependency into the relay vertical.
- `ws` and `wsa` are absent from the generic `msgchannel` branch; remote `msgchannel` and ADMIN `whiskey` remain unchanged/capability-blocked. `publicAgentCommandAliases` excludes all relay aliases.

## Test and verification evidence

Executed from `/Users/ab/workspace/go-projects/zenbot`:

```sh
go test ./internal/command -run 'TestSupportRelay|TestReplica' -count=1
# ok zenbot/internal/command 1.695s

go test ./internal/core -run 'TestSupportRelay|TestEngineSupportRelay|TestReplicaManager' -count=1
# ok zenbot/internal/core 0.253s

go test ./internal/factory ./cmd/zenbot -count=1
# ok zenbot/internal/factory 0.385s
# ok zenbot/cmd/zenbot 1.058s

go test -race ./internal/command ./internal/core ./internal/listener/message -count=1
# ok: command 58.939s; core 1.923s; listener/message 30.682s

go test ./...
# pass (cached results after focused/race gates)

go vet ./...
# exit 0

go build -o /tmp/zenbot-support-relay-qa ./cmd/zenbot
# exit 0; Mach-O 64-bit executable arm64

git diff --check
git diff --cached --check
# both exit 0, no output
```

A focused dependency scan found no proxy, credential/password, temporary-session/snapshot, agent, SQL/repository, transport, or replica-lifecycle references in `internal/common/support_relay.go`, `internal/core/support_relay.go`, or `internal/command/support_relay.go`.

## Evidence limitations

- The prior implementation handoff records RED→GREEN command outputs. This independent QA could rerun the green suite only: the historical RED states cannot be recreated without removing already-present implementation. Git history did not provide independently timestamped intermediate test evidence, so the claimed strict sequential RED→GREEN chronology is **not independently proven** here.
- No dedicated main composition test specifically asserts support-relay installation; direct source inspection and the focused `cmd/zenbot` package gate establish the current wiring. This is a coverage limitation, not a behavior defect found in the scoped implementation.
- The repository remains deliberately dirty with unrelated changes. QA did not stage, reset, clean, restore, checkout, or commit anything.
