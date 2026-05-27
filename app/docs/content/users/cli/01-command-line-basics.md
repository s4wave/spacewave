---
title: Command Line Basics
section: cli
order: 1
summary: Use the Spacewave CLI for status, sessions, Spaces, files, Git, Canvas, and local web listeners.
---

The `spacewave` CLI talks to the same local runtime that the app uses. Start with it when you want repeatable inspection or automation.

```bash
spacewave status
spacewave whoami
spacewave space list
```

Most data commands need a daemon. Client commands normally attach through state-path resolution and may start a CLI-owned daemon when no matching socket is reachable. To run the daemon explicitly:

```bash
spacewave serve --state-path .spacewave
```

## State and Socket Paths

`--state-path` chooses a daemon state directory. The socket lives inside that directory as `spacewave.sock`.

`--socket-path` connects to one exact socket and skips state-path discovery. It is connect-only and never autostarts. Use it only when the app shows or logs the socket path you need.

When `--state-path` is not explicitly set, development workflows can use a live `.spacewave/spacewave.sock` in the current directory or Git root before falling back to the shared user state path.

## Common Commands

```bash
spacewave login local
spacewave space create notes
spacewave space info <space-id>
spacewave fs ls /u/1/so/<space-id>/-/<object-key>/-/
spacewave git show --space <space-id>
spacewave canvas show --space <space-id>
spacewave web
```

For the full command surface, read the [developer CLI reference](/docs/developers/cli/cli-reference).
