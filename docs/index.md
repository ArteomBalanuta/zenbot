# Documentation

These guides describe the current source tree. Runtime behavior is defined by
the code and tests, not by historical migration or implementation plans.

| Guide | Audience and scope |
| --- | --- |
| [Getting started](getting-started.md) | Prerequisites, Docker, local installation, first commands |
| [Configuration](configuration.md) | TOML keys, environment overrides, defaults, limitations |
| [Commands](commands.md) | Syntax, aliases, roles, and action semantics |
| [Architecture](architecture.md) | Components, ownership, data flow, persistence boundaries |
| [Agent](agent.md) | Model loop, tools, context, permissions, recovery, extension points |
| [Operations](operations.md) | Lifecycle, SQLite backups and upgrades, profiling, troubleshooting |
| [Development](development.md) | Building, tests, resources, source layout, provider evaluation |
| [Contributing](../CONTRIBUTING.md) | Change and review expectations |
| [Agent guide](../AGENTS.md) | Repository map and working rules for coding agents |
| [Security](../SECURITY.md) | Sensitive data, trust boundaries, vulnerability reporting |

Keep runnable examples credential-free. Do not commit local configurations,
databases, logs, or agent working notes. The root README is the entry point;
details belong in the guide that owns the subject.
