---
title: Run the Background Service
section: operations
order: 1
summary: Run the background service, choose its directory, control when it stops, and replace it with a new copy.
---

The background service, also called the daemon, is the Spacewave process that
holds your data and serves the `spacewave` commands and the desktop app. This
page shows how to start it, point it at the right directory, stop it, and
replace a running copy, for example after you install a new version.

The commands and the desktop app reach the service through one Unix socket,
`spacewave.sock`. The socket sits inside the state directory in use. The state
directory is where Spacewave keeps its data.

## Choose the directory

Pass `--state-path`, or set `SPACEWAVE_STATE_PATH`, to use a specific state
directory. Commands then look for the socket inside it and start the service if
none is running. [Storage Modes](/docs/self-hosters/storage/storage-modes)
lists every way the directory is chosen.

Pass `--socket-path`, or set `SPACEWAVE_SOCKET_PATH`, to reach one exact socket.
In this form the command connects to what is there and never starts a service.

If a service crashed and left its socket behind, the next start removes the
stale socket. A crash does not block the next run.

## Start the service

```sh
spacewave serve
```

`serve` creates the state directory if needed and binds the socket with
restricted permissions. It removes the socket when it exits.

Only one service can write to a state directory at a time. If another process
holds the directory, `serve` fails with an error that names it.

## Control when it stops

The service stops on its own when nothing is using it. The idle timer starts
when the last command, app, or other client disconnects. A web address kept
open with `spacewave web --background` counts as a user, so the service stays
up while one is open.

```sh
spacewave serve --idle-timeout 5m
spacewave serve --idle-timeout 0
```

The default is 30 seconds. You can also set `SPACEWAVE_DAEMON_IDLE_TIMEOUT`. A
value of `0` turns off idle shutdown, so the service runs until you stop it.

```sh
spacewave stop
```

`stop` asks the running service to shut down. If no service is running, it
says so.

## Replace a running service

```sh
spacewave serve --takeover
```

`--takeover` asks whatever is using the socket to shut down, then starts this
copy in its place. Use it when a service is already running and you want this
one to replace it, for example a newly installed version.

If the desktop app is using the socket, it shows a prompt and asks you to
approve the handover. If you deny it, `serve --takeover` fails and asks you to
quit the desktop app or approve the prompt. While another process has control,
the app's own local actions are unavailable. Use the app's banner to take
control back.

## Browser access

`spacewave web` serves this machine's Spacewave at a local address for a
browser. See [Networking and Browser
Access](/docs/self-hosters/operations/networking-and-web-listeners).
