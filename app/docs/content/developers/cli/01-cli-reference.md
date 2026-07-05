---
title: CLI Reference
section: cli
order: 1
summary: Reference the current Spacewave CLI command tree and daemon connection model.
---

The Spacewave CLI is a Go command tree under `cmd/spacewave/cli`. The repository
script `bun run build:cli` builds it through Bldr and writes `bin/spacewave`.
`bun run cli:local -- <command>` uses `./bin/spacewave --state-path ./.spacewave`.

## Global runtime flags

Common flags include `--state-path`, `--socket-path`, `--session-index`,
`--output`, `--log-level`, `--log-file`, and `--color`. State path can also come
from environment. `SPACEWAVE_SOCKET_PATH` selects an exact socket and disables
autostart.

## User and session groups

- `login`, `logout`, `whoami`, `status`
- `session list|info|logout|revoke`
- `account list|info|create local|create spacewave`
- `provider list|info`
- `auth method|passwd|lock|unlock|threshold|backup`
- `billing usage`

`login` supports username/password, PEM files, browser handoff, local login, and
browser login. `auth backup generate` writes a PEM backup key.

## Space and object groups

- `space list|create|delete|rename|info|resolve|settings|import-git|deploy`
- `space world changelog|rollback-plan`
- `space object list|info|graph|create|delete`
- `fs ls|cat|mkdir|rm|write|mv|stat`

`fs` commands accept object-only paths, object plus UnixFS path, or full web-style
URIs. `space object create` can create built-in object families such as UnixFS,
Git, Canvas, and generic object types.

## Developer and operator groups

- `plugin list|add|remove|import-manifest`
- `web`, `web list`, `web stop`
- `device setup|complete|status`
- `git show|refs|log|diff|commit|tree|clone|fetch|worktree`
- `canvas show|watch|apply|node|edge|export`
- `forge create-cluster|create-job|create-worker`
- `vm list|info|create|start|stop|watch|image|run`
- `debug trace|cpu-profile|mem-profile`

`bifrost` and `hydra` are embedded advanced command families from their owning
packages. The current tree does not register a top-level `start` or `devtool`
command in `spacewave`.
