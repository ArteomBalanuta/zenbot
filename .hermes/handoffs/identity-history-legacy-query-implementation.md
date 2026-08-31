# Legacy H2 history query verification

## Scope

Verified `internal/repository/h2/identity.go`, `Database.LastMessages`.

The working tree already contained the required legacy SQL behavior before this assignment made an edit:

```sql
WHERE (name=$1 OR trip=$2) AND visibility='PUBLIC' AND message NOT IN ('LEFT','JOINED') ORDER BY created_on DESC,id DESC LIMIT %d
```

It preserves the existing guarded integer limit behavior (`count <= 0` becomes `5`). No production-file change was required or made by this assignment.

## RED/GREEN evidence

A requested RED failure could not be reproduced because the focused test already passed before any edit:

```text
$ go test ./internal/repository/h2 -run '^TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies$' -count=1
ok  	zenbot/internal/repository/h2	1.076s
```

After inspection and formatting, the same focused test remained GREEN:

```text
$ go test ./internal/repository/h2 -run '^TestGroupBSaturnLastMessagesReturnsPublicRowsWithRowTripAndStableTies$' -count=1
ok  	zenbot/internal/repository/h2	0.944s
```

## Additional verification

```text
$ go test ./internal/command -run '^(TestAccessCommandUsesSaturnRawCaseSensitiveRoleParsing|TestMessagesCommandUsesReturnedRowTrip)$' -count=1
ok  	zenbot/internal/command	0.628s

$ gofmt -w internal/repository/h2/identity.go && git diff --check
(exit 0; no output)
```

## Files

- Created: `.hermes/handoffs/identity-history-legacy-query-implementation.md`
- `internal/repository/h2/identity.go`: inspected and formatted; no diff attributable to this assignment because the requested SQL correction was already present.

No commit or push was performed.
