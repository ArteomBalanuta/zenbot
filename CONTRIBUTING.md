# Contributing

Start with [local setup](docs/getting-started.md#local-development) and the
[architecture guide](docs/architecture.md). Small, focused changes are easier to
review than unrelated cleanup mixed with behavior changes.

## Workflow

1. Describe the problem and intended user-visible behavior. For a bug, include
   sanitized reproduction steps and the affected version.
2. Make a focused change on a branch. Preserve command aliases, role checks,
   cancellation, visibility, and delivery semantics unless changing those is
   explicitly part of the change.
3. Add a regression test that fails without the fix. Use isolated fixtures;
   never use a live room or production database for ordinary automated tests.
4. Run `make check`, `make compile`, and race tests for the changed packages.
   Follow [development](docs/development.md) for integration prerequisites.
5. Update the relevant `docs/` reference and sanitized configuration examples.
6. Open a pull request explaining the outcome, notable tradeoffs, and exact
   verification commands/results. Call out tests you could not run.

Do not commit generated binaries, downloaded jars, credentials, database files,
chat logs, editor metadata, or internal planning/handoff documents. Use fictional
users and secrets in fixtures. Runtime prompt resources and regression tests are
source files, not cleanup targets.

Use `gofmt`, existing package boundaries, and conventional commit subjects such
as `fix(commands): preserve private delivery on lookup failure`. Avoid introducing
new dependencies when existing code already provides the needed behavior.

## Review expectations

- Errors must distinguish unavailable data from failed or ambiguous execution.
- A sent request is not proof of a completed remote action.
- Public history and private messages must retain their visibility boundaries.
- Database upgrades need real H2 tests and a documented recovery path.
- Tool argument/result schemas and resource descriptions must agree.
- Tests should verify observable behavior, not merely repeat implementation
  constants or relax assertions until a failure disappears.

Report vulnerabilities as described in [SECURITY.md](SECURITY.md), not in a public
issue with credentials or real chat data. The repository currently declares no
project license; resolve licensing with the maintainer before contributing code
whose redistribution terms need clarification.
