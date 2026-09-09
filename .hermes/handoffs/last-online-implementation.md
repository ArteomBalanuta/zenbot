# `lastonline` / `lastseen` implementation handoff

## Scope delivered

Implemented the independent Saturn regular-user last-online vertical in Zenbot only. No Saturn source, schema, configuration, protocol, agent, moderation, reversal, migration-plan, or frozen-audit files were edited. No commit or push was made.

### Source trace

- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/command/impl/user/LastOnlineUserCommandImpl.java`
  - aliases: `lastonline`, `seen`, `last`, `online`, `lastseen`
  - regular-user authorization is retained through Zenbot's catalog/dispatcher
  - first argument, trim/one-`@` normalization, missing-target usage, caller addressing, and inbound whisper behavior.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/service/impl/UserServiceImpl.java`
  - two reads and fixed payload source.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/util/SqlUtil.java`
  - parity predicates: `(name = ? or trip = ?)`, non-`LEFT`/`JOINED` latest message, and latest literal `JOINED`.
- `/Users/ab/workspace/projects/saturn/src/main/java/org/saturn/app/model/dto/LastSeenDto.java`
  - default `" - "` values.

## Exact touched files

Implementation:

- `internal/repository/user_queries.go` — `LastOnlineRecord` and typed `LastOnline` repository contract.
- `internal/repository/h2/user_queries.go` — parameterized two-read H2 implementation; missing rows are valid empty fields.
- `internal/service/services.go` — `UserService.LastOnline`, injectable UTC `Now`, source-shaped literal `\\n` payload, UTC RFC-1123/duration formatting, and JSON escaping without quote wrapping or HTML escaping.
- `internal/command/last_online.go` — concrete command adapter.
- `internal/command/handlers.go` — canonical construction.
- `internal/command/dispatch_adapter.go` — registration only when `Users` and `Users.Queries` are available.

Tests:

- `internal/repository/h2/last_online_test.go` — real-H2 name/trip matching, case behavior, presence exclusions, latest selection, empty record.
- `internal/service/last_online_test.go` — fixed-clock populated/default payload rendering and quote/backslash/newline/control/HTML-sensitive escaping.
- `internal/command/last_online_test.go` — aliases/USER metadata, normalization, usage/no query, whisper, errors, conditional registration, real-H2 listener dispatch for `lastseen` and `seen`.
- `internal/listener/user_joined_listener_test.go` — required one-method update to its local `UserQueryRepository` fake after the interface gained `LastOnline`.

## Verification

RED evidence recorded before each production slice:

- repository test initially failed: `Database.LastOnline undefined`.
- service test initially failed: `UserService.Now` and `UserService.LastOnline` absent.
- command test initially failed: placeholder behavior/no registration.

GREEN/final gates:

- `go test ./internal/repository/h2 -run TestLastOnline -count=1` — PASS
- `go test ./internal/service -run TestUserServiceLastOnline -count=1` — PASS
- `go test ./internal/command -run 'Test(LastOnline|LastSeen)' -count=1` — PASS
- `go test ./internal/command -run 'Test(RegisterUserUtilities|UsersAndNicks)' -count=1` — PASS
- `go test ./internal/repository/h2 ./internal/service ./internal/command` — PASS
- `go test ./internal/listener -run TestUserJoinedListener -count=1` — PASS
- `go test ./...` — PASS
- `git diff --check` — PASS

The first full-suite attempt exposed the expected interface-fake compile break in `internal/listener/user_joined_listener_test.go`; adding the minimal `LastOnline` stub resolved it, and the final full suite passed.

## Parity limitation

Saturn has focused command-boundary tests but no focused persistence/rendering test for `UserServiceImpl.lastOnline`; therefore full historical payload compatibility is source-derived. Zenbot fixes that source-derived payload with its own fixed-clock renderer test.

## Tree state

Pre-existing dirty semantic-moderation/reversal and related handoff work remains untouched. This slice adds the files above and modifies only its listed production files plus the required listener test fake.
