---
title: Upgrades and Daemons
section: operations
order: 1
summary: Operate the daemon, socket, state path, and local web listener explicitly.
---

The native CLI and desktop runtime meet at a Unix socket named `spacewave.sock`.
The socket lives under the resolved state path.

## State path rules

Use `--state-path` or the supported state-path environment variables when you
need a specific runtime root. Use `--socket-path` or `SPACEWAVE_SOCKET_PATH` only
when you want to connect to an exact existing socket. An explicit socket path
does not autostart a daemon.

Without an explicit socket, client commands resolve the state path and can
autostart the current executable in `serve` mode. Stale refused sockets are
removed before autostart.

## Serve mode

```sh
spacewave serve
spacewave serve --takeover
spacewave stop
```

`serve` creates the state directory, listens on the Unix socket, applies socket
permissions, starts plugin and device runtime services, and removes the socket on
exit. `--takeover` asks an existing runtime to yield before binding the socket.
`stop` requests daemon shutdown and treats a missing socket as a no-op.

## Runtime handoff

The desktop app can show a takeover prompt when a local process asks to own the
runtime socket. During an active handoff, native runtime actions should be
disabled or reclaimed from the banner.

## Web listeners

`spacewave web` starts a localhost listener for browser access to the native
runtime. `--background` keeps it in the daemon. `web list` and `web stop` manage
background listeners.
