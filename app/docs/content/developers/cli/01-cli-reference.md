---
title: CLI Reference
section: cli
order: 1
summary: The current command tree, global flags, connection behavior, and output conventions.
---

The `spacewave` binary manages the local daemon, sessions, Spaces, and object data.

## Top-Level Commands

```text
start      start the daemon and block
login      sign up, log in, open browser handoff, or create a local account
logout     sign out a local session
whoami     show current session identity
serve      start the daemon and listen for CLI connections
stop       stop the daemon
status     show daemon, session, auth, and Space summary
web        manage localhost web listeners
auth       manage locking and auth methods
billing    inspect billing and usage
space      manage Spaces
fs         read and write UnixFS object paths
git        inspect and fetch Git objects
canvas     inspect and mutate Canvas objects
forge      manage Forge clusters, jobs, tasks, and workers
vm         manage VM and V86 image objects
plugin     manage Space plugins
debug      capture trace and profile data from a running daemon
account    lower-level account commands
session    lower-level session commands
provider   lower-level provider commands
bifrost    network-router command set
hydra      storage command set
```

## Global Flags

```text
--state-path, -s   state directory path
--log-level        debug, info, warn, error
--log-file         file logging spec
--output, -o       json, text, yaml
--color            auto, always, never
```

Environment variables include `SPACEWAVE_STATE_PATH`, `SPACEWAVE_DATA_DIR`, `BLDR_STATE_PATH`, `SPACEWAVE_OUTPUT`, `SPACEWAVE_COLOR`, `SPACEWAVE_SOCKET_PATH`, and `SPACEWAVE_SESSION_INDEX`.

## Connection Rules

Client commands use `--socket-path` when present. That is connect-only, does not join a state directory, and never autostarts a daemon.

Without `--socket-path`, commands resolve the state path and connect to `<state-path>/spacewave.sock`. If no daemon is reachable there, client commands may start a CLI-owned daemon for that state path. When `--state-path` is unset, project-local `.spacewave/spacewave.sock` discovery can win in development workflows before the shared user state path.

Use `spacewave --help` and `spacewave <command> --help` as the final source for flags while scripting.
