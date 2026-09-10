---
name: git-pr-writing
description: Use when preparing or polishing branches, commits, merge requests, and pull requests in a software repository.
---

# Git PR Writing

Use this skill when creating or polishing a branch, commit series, merge request, or pull request.

## Goal

Make the change easy to review in one pass:

- the branch name tells reviewers the stream of work
- each commit changes one concern
- the PR title states the shipped outcome
- the Jira ticket ID is visible in the branch, commit body, and PR metadata when one exists
- the PR body gives a fast technical map and a concrete verification trail

## Branch naming

Prefer:

- `feat/<capability>`
- `fix/<behavior>`
- `bugfix/<behavior>` when the repo already uses it
- `chore/<maintenance-task>`
- `refactor/<area>`
- `docs/<area>`
- `test/<area>`

Examples:

- `feat/webhook-ingest-bearer-auth`
- `bugfix/remove-duplicate-search-results`
- `bugfix/app-716-service-supplier-flow`

Keep branch names lowercase, slash-scoped, and outcome-oriented. When the work is
tracked by Jira, include the lowercased ticket ID after the branch type, e.g.
`bugfix/app-716-service-supplier-flow`.

## Commit structure

Follow conventional commit subjects with a scope. When a Jira ticket exists,
put it directly after the scope:

`type(scope): JIRA-ID concise outcome`

When a Jira ticket exists, also repeat it in the commit body, preferably the
first body line:

```md
fix(router): APP-716 route service suppliers and guard proposal fields

APP-716

<why this change is needed and what changed>
```

Good types:

- `feat`
- `fix`
- `docs`
- `test`
- `refactor`
- `chore`

Good scopes:

- subsystem or bounded area, such as `router`, `agent`, `persistence`, `commands`

Rules:

- One commit per concern when practical.
- Keep the subject imperative and specific.
- Match the scope across related commits in the same series.
- Use a body when reviewers need rationale, constraints, or behavior notes.
- Wrap the body naturally and explain why, not just what.
- Include the Jira ticket ID in the body for traceability when one exists.

Examples:

- `docs(agent): explain multi-tool execution policy`
- `feat(persistence): add moderation event history`
- `test(commands): cover kick target normalization`
- `refactor(router): split planning policy from execution`

For smaller changes, one well-formed commit is enough if it still reads cleanly.

## PR title

Use the same conventional pattern as the lead commit:

`type(scope): JIRA-ID shipped outcome (ai-generated, includes-ai-code)`

The title should describe the end-user or reviewer-visible result, not the implementation mechanic and should include postfix (ai-generated, includes-ai-code).
When there is no Jira ticket, omit the `JIRA-ID` segment.

Prefer:

- `fix(commands): normalize addressed usernames (ai-generated, includes-ai-code)`
- `fix(router): APP-716 preserve tool evidence between steps (ai-generated, includes-ai-code)`

Avoid:

- `misc fixes`
- `update files`
- `changes for duplicates`

## PR defaults

When opening a new merge request / pull request, default to:

- Draft status enabled
- `ai-generated` label applied
- `includes-ai-code` label applied

Only skip draft status when the user explicitly asks for a ready-for-review PR.
Only skip the AI labels when the target repo does not support labels or the user explicitly asks not to apply them.

If the MR/PR is created from the CLI, prefer commands or follow-up API steps that ensure the draft state and labels are applied immediately after creation.

## PR body

Use this structure:

## Summary

2-6 sentences covering:

- the problem
- the intended behavior after the change
- important constraints or non-goals

## What changed

- Flat bullets only.
- Group by area or behavior, not by file dump.
- Mention propagated variants when the same fix was applied across example families.
- Call out anything intentionally left unchanged.

## Test plan

- Prefer checkbox bullets.
- Include exact commands when useful.
- Include smoke checks or targeted verification.
- Leave an unchecked item only for an external dependency that still needs to land.

Template:

```md
## Summary

Jira: <JIRA-ID or "N/A">

<problem and outcome>

## What changed

- <change area 1>
- <change area 2>
- <important non-change or scope limit>

## Test plan

- [x] `<command>`
- [x] `<command>`
- [x] Manual spot check of <behavior>
```

## Review heuristics

Before opening the PR, check:

- The title says what ships.
- The commit subjects read like a clean changelog.
- The PR body can be skimmed in under a minute.
- The test plan proves the exact risk that changed.
- The diff contains no unrelated formatting noise.
- The Jira ticket ID appears in the branch, commit body, PR title, and PR body when one exists.
- The MR/PR is opened as draft by default unless the user asked otherwise.
- The `ai-generated` and `includes-ai-code` labels are present.

## Applying this skill to targeted behavior changes

For a focused behavior correction, prefer:

- a scope naming the affected subsystem
- type: `fix` when observable behavior changes
- PR Summary: explain the user-visible problem and intended outcome
- What changed: group the implementation by behavior rather than listing files
- Test plan: include focused tests plus a diff or smoke check proving unrelated behavior stayed unchanged
