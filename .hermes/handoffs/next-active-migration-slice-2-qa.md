# Mail Group C QA handoff

## Verdict

**PASS — bounded Mail Group C only.** This does not accept full row #324 or the overall migration.

## Independent source check and hardening

Read-only Saturn evidence:

- `MailServiceImpl.java:66-74` acknowledges with the resolved comma-separated receiver trips, not the input nickname.
- `MailServiceImpl.java:82-101` JSON-escapes writes and catches SQL write failures while retaining command-success behavior.
- `MailServiceImpl.java:151-195` selects pending mail by trip boundary and marks exactly by id.
- `MailUserCommandImpl.java:27-34` dispatches aliases `mail`, `msg`, and `send` through the mail service.

QA found and fixed two confirmed in-scope parity defects:

1. Zenbot acknowledged `@mErC` instead of Saturn's resolved `trip-a`. `MailService.QueueResolved` now returns the resolved trip list, and `mailCommand` uses it in the acknowledgement.
2. Zenbot propagated failed insert errors, while Saturn logs them and still acknowledges scheduling. The focused real-H2 test drops `mail` after recipient resolution and verifies `Queue` returns no error; `QueueResolved` now retains the resolved acknowledgement after a failed insert.

The existing `Queue` API remains available and delegates to `QueueResolved`; no schema, listener ordering, Notes, moderation, agent, transport, or protected-document behavior was changed.

## Focused evidence

```text
go test ./internal/service -run TestMailGroupC -count=1
ok      zenbot/internal/service    2.495s

go test ./internal/command -run TestMailGroupC -count=1
ok      zenbot/internal/command    2.555s

go test ./internal/repository/h2 -run TestMail -count=1
ok      zenbot/internal/repository/h2    1.111s
```

The added/updated focused tests cover recipient normalization, case-insensitive multi-trip resolution, JSON escaping plus trailing space, failed-write acknowledgement, pending-only reads, id-based delivery, aliases, blank/unknown receiver behavior, and resolved-trip acknowledgement.

## Full validation

All passed from `/Users/ab/workspace/go-projects/zenbot` after the final hardening edit:

```text
go test ./...       PASS
go test -race ./... PASS
go vet ./...        PASS
go build ./...      PASS
gofmt -l task files 0 paths
git diff --check    PASS
```

`go test -race ./...` emitted one macOS linker warning for `internal/agent/sql.test` (`malformed LC_DYSYMTAB`), but exited successfully and all packages passed.

Scoped added-line security scan found no secrets, shell execution, eval, or unsafe deserialization. The SQL `DB.Exec` is parameterized; it is not shell execution.

## Scope and integrity

The repository was already dirty. QA only changed these bounded files:

- `internal/service/services.go`
- `internal/command/mail_notes.go`
- `internal/service/mail_group_c_parity_test.go`
- `internal/command/mail_group_c_parity_test.go`
- this handoff

Protected files were not edited. Final SHA-256:

```text
a3c805cb1a49cf35e59aec790ae1182e2b52ef0f6f310004b7472c331af8f828  internal/service/services.go
7018fd498d305868a151f4982ab2f581f7efc544d78f41e51ef917d24d568a7c  internal/command/mail_notes.go
9a925cfe0cd6c0343ea571cd08f942ebd29f499895dd983b28668c207b0f41ad  internal/service/mail_group_c_parity_test.go
b6718a791461f974e9db9e2af24c8e3fd6bbb7c966226d347041915b8bac387a  internal/command/mail_group_c_parity_test.go
bd7f5070c08ccce511bdab06520655b648a7dcc3e6ca48dbbd549778d19891a0  MIGRATION_PLAN.md
75d7d23b2d4fe58bb2c2ceac04f56412b6d2f85cc69fe239a4755bd1b72f8a18  .hermes/migration-audit.md
```
