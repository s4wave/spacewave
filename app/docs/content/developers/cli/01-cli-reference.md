---
title: CLI Reference
section: cli
order: 1
summary: Every command group in the spacewave CLI, the flags they share, and how commands find the background service.
---

The `spacewave` CLI controls a Spacewave background service from the terminal.
Use this page to find the command group for a task and the flags that select
which service and session a command uses. For a guided start, see [Command
Line Basics](/docs/users/cli/command-line-basics).

The CLI is a Go command tree under `cmd/spacewave/cli`. From the repository,
`bun run build:cli` builds it with Bldr and writes `bin/spacewave`.
`bun run cli:local -- <command>` runs
`./bin/spacewave --state-path ./.spacewave`.

## Choosing the service and session

Commands that talk to the background service accept these flags:

- `--state-path`, or `-s`, sets the state directory. It can also come from the
  environment. [Storage Modes](/docs/self-hosters/storage/storage-modes) lists
  the order in which the path is chosen.
- `--socket-path` connects to an existing service at that exact socket path.
  `SPACEWAVE_SOCKET_PATH` does the same. With either one, the command does not
  start a service.
- `--session-index` picks the session to use. The default is `1`, and
  `SPACEWAVE_SESSION_INDEX` sets it too.

Commands that print data take their own `--output`, or `-o`, flag with `text`,
`json`, or `yaml`. There is no global `--output` flag. The binary's entrypoint
also accepts `--log-level`, `--log-file`, and `--color`.

## Running the service

- `serve` starts the background service in the foreground and listens for CLI
  connections. `--takeover` asks a service already on the socket to hand over.
  `--idle-timeout` sets how long the service waits after its last client or
  task before it stops. `0` disables the idle stop.
- `stop` stops the background service.
- `status` checks that the service is healthy and prints a summary.
- `web`, `web list`, and `web stop` open and manage browser access. See
  [Networking and Browser
  Access](/docs/self-hosters/operations/networking-and-web-listeners) for the
  flags.
- `tui <plugin-id>` opens a plugin's terminal view against the local runtime.
  It is not available on Windows.

[Run the Background Service](/docs/self-hosters/operations/upgrades-and-daemons)
explains when the service starts and stops.

## Accounts and sessions

- `login`, `logout`, `whoami`
- `login local`, `login browser`, `login file <path>`, `login p2p` (alias
  `pair`)
- `session list|info|logout|revoke`
- `account list|info|create local|create spacewave`
- `provider list|info`
- `auth method|passwd|lock|unlock|threshold|backup`
- `billing usage`

`login` accepts a username and password, a PEM key file, or a browser handoff.
`login local` creates a local account that works offline. `login file` opens an existing
`.s4wave` session file. `login p2p` adds an account using a code from another
device. `auth backup generate` writes a PEM backup key.

## Spaces and objects

- `space list|create|delete|rename|info|resolve|settings|import-git|deploy`
- `space world changelog|rollback-plan`
- `space object list|info|graph|create|delete`
- `fs ls|cat|mkdir|rm|write|mv|stat`

`space object list --watch` keeps printing the list as it changes. `fs`
commands accept a path to an object, an object plus a path inside its files, or
a full web-style URI. `space object create` can create built-in kinds such as
UnixFS, Git, and Canvas, as well as other object types.

## Features and devices

- `plugin list|add|remove|import-manifest`
- `device setup|setup docker|complete|status|approve|policy`
- `git show|refs|log|diff|commit|tree|clone|fetch|worktree`
- `canvas show|watch|apply|node|edge|export`
- `apt import-deb`
- `forge create-cluster|create-job|create-worker`
- `vm list|info|create|start|stop|watch|image|run`
- `debug trace|cpu-profile|mem-profile`

`plugin list --watch` keeps printing plugin state as it changes. `device
approve` approves a device's link ticket for a Space. `device policy` manages
this machine's device settings, such as the remote shell and checkout folders.
`apt import-deb` imports a Debian package into an Apt repository object.

`bifrost` and `hydra` are advanced command groups built into the binary from
Spacewave's networking and storage packages. There is no top-level `start` or
`devtool` command.
