---
title: Storage Modes
section: storage
order: 1
summary: Compare browser storage, desktop state, project-local daemon state, and cloud-backed storage.
---

Spacewave storage is tied to the session and the provider path behind it.

Browser storage is convenient for first use. It is also the easiest to lose by clearing site data, changing profiles, or relying on a browser that manages storage aggressively.

Desktop and daemon state directories are easier to inspect and back up. The CLI default state path is the shared user state path, and development workflows can intentionally use a project-local `.spacewave` directory.

Cloud-backed storage gives you a provider-managed path for recovery and device moves.

## State Paths

The CLI uses `--state-path` for daemon state. The socket file lives at:

```text
<state-path>/spacewave.sock
```

Use `--socket-path` only when connecting to a known running daemon socket. It does not choose a data directory and never starts a daemon.

## Backups

Back up the state that actually owns the session. A backup of an unused `.spacewave` directory does not protect a session running from the desktop default.

Before any migration, run `spacewave status` against the state path you intend to back up and confirm the expected session and Spaces appear.
