# Saturn To Zenbot Command Cross-Check

## Result

- Saturn source command definitions reviewed: 64
- Zenbot catalog definitions: 64
- In-scope concrete Zenbot commands: 60
- Approved exclusions: 4 (`mine`, `whiskey`, `ws`, `wsa`)
- In-scope alias gaps: 0
- In-scope placeholder handlers: 0

The detailed canonical/alias/access/tool matrix is maintained in
`COMMAND_TOOL_INVENTORY.md`. The executable closure assertion is
`TestEveryInScopeSaturnCommandBuildsAConcreteHandler` in
`internal/command/production_registry_parity_test.go`.

## Registration Model

`internal/command/catalog/catalog.go` is the only command identity catalog.
`internal/command/registry.go` and `internal/command/dispatch_adapter.go` build
chat handlers from it. `internal/command/agent_catalog.go` builds the agent
inventory from the same entries. This removes the previous alias drift caused
by maintaining separate static lists.

Handlers are registered only when the engine exposes their required runtime
capability. This is intentional composition, not a placeholder:

| Capability | Commands enabled |
|---|---|
| Base services | Public utilities and DBZ commands |
| Persistence services | Mail, notes, activity, user, audit, and shadow-ban commands |
| Moderation operations | Moderation command group |
| Room snapshots | `nuke` and snapshot-dependent behavior |
| Live room mover | `resurrect` |
| Prefix controller | `prefix` |
| Automove controller | `automove` |
| Replica controller | Replica lifecycle commands |
| Lifecycle controller | `restart`, `shutdown` |
| SQL command service | `sql` |
| Direct agent submitter | `l` |

## Agent Mapping

Every in-scope command except `l` has a `saturn_<canonical>` tool generated
from the shared catalog. `l` is deliberately omitted to prevent the agent from
recursively invoking itself. The compact `run_command` tool exposes a reviewed
subset and derives its contextual enum from the same source.

All command tools are actions, non-idempotent, and sequential. Public,
moderator, permanent-ban, and admin exposure is calculated from trusted room
identity rather than prompt text.

## Reproduction

```sh
go test ./internal/command -run TestEveryInScopeSaturnCommandBuildsAConcreteHandler -count=1
go test ./internal/command ./internal/agent/tool ./cmd/zenbot -count=1
```
