# Security and privacy

## Reporting a vulnerability

Do not include secrets, private messages, database dumps, or exploitable details
in public issues. Use GitHub's private vulnerability reporting **if enabled**
for this repository, or arrange a private channel with the repository owner
before sharing sensitive material. No dedicated security mailbox, response SLA,
or supported-version policy is currently declared.

Provide the affected revision, minimal sanitized reproduction, impact, and any
proposed mitigation. Rotate exposed credentials immediately; deleting a file or
adding it to `.gitignore` does not remove it from existing Git history or clones.

## Deployment boundaries

- The bot can read room messages and perform privileged actions permitted by its
  server identity. Give it only the privileges required for the deployment.
- `adminTrips`, `userTrips`, persisted roles, room moderator status, and agent
  creator capabilities have different meanings. Review [commands](docs/commands.md)
  rather than assuming every allowlist is a low-privilege access list.
- The admin `sql` command is raw SQL, not the agent's bounded read-only SQL tool.
  Treat access to it as database-administrator access.
- Agent tools enforce caller capabilities, but the model can still reason
  incorrectly. Tool authorization is not a guarantee of appropriate decisions.
- The provider receives selected conversation/history/tool context. Choose a
  provider whose handling of that data is acceptable to room participants.
- Dynamic SQL is privileged. Application tables can contain private chat and
  sensitive application records.

## Data handling

Local `config.toml`, `.env`, SQLite files, backups, and logs must remain private.
Database records can include chat text, whispers, notes, mail, identity metadata,
and agent conversation history. The application does not provide encrypted-at-rest
storage; protect the host filesystem and backups accordingly.

Logs are **not** a redacted export format. Some listeners log incoming payloads
and chat text. Review and redact diagnostic output before sharing it. Agent-memory
TTL is not a global deletion policy for messages, mail, notes, audits, or backups.
Set an operational retention policy suitable for your room; do not assume all
stored data automatically expires.

Profiling is disabled by default. When enabled, pprof endpoints expose sensitive
runtime information without application authentication. Keep them on loopback or
behind an authenticated access boundary. The default Make mapping binds the host
port to `127.0.0.1`; changing that exposes a new attack surface.

SQLite runs inside the bot process and has no listening port. Protect database and
configuration mounts from untrusted local users.

## Before sharing a checkout or release

Inspect both the current files and Git history with a maintained secret scanner.
The ignore rules prevent common accidents but cannot detect every secret. Do not
publish local runtime directories or raw evaluation transcripts. A clean working
tree or passing test suite is not a security certification. Check dependency and
container vulnerabilities as part of your release process.
